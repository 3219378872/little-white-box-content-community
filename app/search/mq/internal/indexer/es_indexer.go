package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"esx/pkg/event"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// ESIndexer 把 PostEvent 同步到 Elasticsearch。
// 索引名通过 Config.Index 注入；调用方负责索引初始化（CreateIndexIfMissing）。
type ESIndexer struct {
	client *elasticsearch.Client
	index  string
}

// NewESIndexer 创建指向指定索引的写入器；不检查连通性，索引初始化由 EnsureIndex 负责。
func NewESIndexer(addresses []string, index string, opts ...ESOption) (*ESIndexer, error) {
	cfg := elasticsearch.Config{Addresses: addresses}
	for _, opt := range opts {
		opt(&cfg)
	}
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("new ES client: %w", err)
	}
	return &ESIndexer{client: client, index: index}, nil
}

// ESOption 配置底层 client，用于注入 username/password 等。
type ESOption func(*elasticsearch.Config)

// WithBasicAuth 为 ES 启用用户名/密码认证。
func WithBasicAuth(user, password string) ESOption {
	return func(c *elasticsearch.Config) {
		c.Username = user
		c.Password = password
	}
}

// WithCACert 接收 ES 自动生成的 CA 证书原文（PEM），用于 TLS 校验。
func WithCACert(pem []byte) ESOption {
	return func(c *elasticsearch.Config) {
		c.CACert = pem
	}
}

// updateRetryOnConflict 让 ES 在并发 _update 的 seq_no 冲突时就地重读重试；
// 用尽后的 409 是真实失败，交给消息重试。
const updateRetryOnConflict = 3

// Index 把 PostEvent 投影为 ES 文档，按 revision 单调覆盖。
// 不用 external version：计数补丁走 _update 会推高 _version，使其不再等于 revision，
// 下一次编辑或删除会被误判为旧快照。改为脚本比较 _source.revision，
// 重投或乱序到达的旧快照（incoming <= stored）为 noop；revision <= 0 无条件覆盖。
// 计数只由 counted 补丁按 stats_seq 推进（帖子点赞与评论计数的变更都在同一事务发出
// counted 事件）。帖子写入携带的计数是事务外读到的快照，存活文档已有不旧于它的
// stats_seq 时保留已存计数；墓碑或新文档才采用快照。
func (e *ESIndexer) Index(ctx context.Context, doc IndexDoc) error {
	source := make(map[string]any, len(doc.Body)+1)
	for k, v := range doc.Body {
		source[k] = v
	}
	if doc.Revision > 0 {
		source["revision"] = doc.Revision
	}
	return e.scriptedUpsert(ctx, doc.DocID, indexByRevisionScript, map[string]any{
		"revision": doc.Revision, "doc": source,
	})
}

const indexByRevisionScript = `
long incoming = ((Number) params.revision).longValue();
if (incoming > 0 && ctx._source.revision != null && incoming <= ((Number) ctx._source.revision).longValue()) {
  ctx.op = 'noop';
  return;
}
def kept = null;
if (ctx._source.post_id != null && ctx._source.stats_seq != null) {
  long stored = ((Number) ctx._source.stats_seq).longValue();
  long next = params.doc.stats_seq == null ? 0L : ((Number) params.doc.stats_seq).longValue();
  if (next <= stored) {
    kept = ['like_count': ctx._source.like_count, 'comment_count': ctx._source.comment_count, 'stats_seq': ctx._source.stats_seq];
  }
}
ctx._source.clear();
ctx._source.putAll(params.doc);
if (kept != null) {
  ctx._source.putAll(kept);
}
`

const patchCountsScript = `
if (ctx._source.post_id == null) {
  ctx.op = 'noop';
  return;
}
long stored = ctx._source.stats_seq == null ? 0L : (long) ctx._source.stats_seq;
if (params.stats_seq <= stored) {
  ctx.op = 'noop';
  return;
}
ctx._source.like_count = params.like_count;
ctx._source.comment_count = params.comment_count;
ctx._source.stats_seq = params.stats_seq;
`

// PatchCounts updates interaction counters without replacing the indexed body.
// A missing document is ErrNotIndexed so the caller can retry after the create event lands.
// 墓碑没有 post_id，删除后的计数补丁为 noop。
func (e *ESIndexer) PatchCounts(ctx context.Context, doc IndexDoc) error {
	payload, err := json.Marshal(map[string]any{
		"script": map[string]any{
			"lang":   "painless",
			"source": patchCountsScript,
			"params": map[string]any{
				"like_count":    doc.Body["like_count"],
				"comment_count": doc.Body["comment_count"],
				"stats_seq":     doc.Body["stats_seq"],
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal count patch: %w", err)
	}
	res, err := e.update(ctx, doc.DocID, payload)
	if err != nil {
		return fmt.Errorf("ES count patch: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusNotFound {
		return ErrNotIndexed
	}
	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES count patch failed status=%s body=%s", res.Status(), string(raw))
	}
	return nil
}

// Delete 把文档替换为只含 revision 的墓碑，而不是物理删除：
// 物理删除后，重试晚到的旧 create/update 会把帖子重新写回索引。
// 墓碑没有 title/body/tags，不会被查询或标签聚合命中；revision 不新于已存值时为 noop。
func (e *ESIndexer) Delete(ctx context.Context, docID string, revision int64) error {
	return e.scriptedUpsert(ctx, docID, tombstoneByRevisionScript, map[string]any{
		"revision": revision,
	})
}

const tombstoneByRevisionScript = `
long incoming = ((Number) params.revision).longValue();
if (ctx.op == 'create' && incoming <= 0) {
  ctx.op = 'noop';
  return;
}
if (incoming > 0 && ctx._source.revision != null && incoming <= ((Number) ctx._source.revision).longValue()) {
  ctx.op = 'noop';
  return;
}
ctx._source.clear();
ctx._source.revision = incoming;
ctx._source.deleted = true;
`

// scriptedUpsert 用 painless 脚本做“不存在即创建”的更新，revision 比较在 ES 端原子完成。
func (e *ESIndexer) scriptedUpsert(ctx context.Context, docID, script string, params map[string]any) error {
	payload, err := json.Marshal(map[string]any{
		"scripted_upsert": true,
		"upsert":          map[string]any{},
		"script":          map[string]any{"lang": "painless", "source": script, "params": params},
	})
	if err != nil {
		return fmt.Errorf("marshal ES upsert: %w", err)
	}
	res, err := e.update(ctx, docID, payload)
	if err != nil {
		return fmt.Errorf("ES upsert request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES upsert failed status=%s body=%s", res.Status(), string(raw))
	}
	return nil
}

// update 发送 _update 请求；不强制刷新以保持写入吞吐，并发冲突交给 ES 就地重试。
func (e *ESIndexer) update(ctx context.Context, docID string, payload []byte) (*esapi.Response, error) {
	retries := updateRetryOnConflict
	return esapi.UpdateRequest{
		Index:           e.index,
		DocumentID:      docID,
		Body:            bytes.NewReader(payload),
		Refresh:         "false",
		RetryOnConflict: &retries,
	}.Do(ctx, e.client)
}

// EnsureIndex 在启动时确保索引存在，使用与父 spec phase-3 §1.2 一致的 mapping。
func (e *ESIndexer) EnsureIndex(ctx context.Context) error {
	res, err := e.client.Indices.Exists([]string{e.index}, e.client.Indices.Exists.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("ES indices exists: %w", err)
	}
	_ = res.Body.Close()
	if res.StatusCode == http.StatusOK {
		return nil
	}
	create, err := e.client.Indices.Create(
		e.index,
		e.client.Indices.Create.WithContext(ctx),
		e.client.Indices.Create.WithBody(strings.NewReader(PostIndexMapping)),
	)
	if err != nil {
		return fmt.Errorf("ES indices create: %w", err)
	}
	defer func() { _ = create.Body.Close() }()
	if create.IsError() {
		raw, _ := io.ReadAll(create.Body)
		return fmt.Errorf("ES create index failed status=%s body=%s", create.Status(), string(raw))
	}
	return nil
}

// Refresh 强制刷新索引，仅供测试和离线重建使用，在线写入路径不应调用。
func (e *ESIndexer) Refresh(ctx context.Context) error {
	res, err := e.client.Indices.Refresh(
		e.client.Indices.Refresh.WithContext(ctx),
		e.client.Indices.Refresh.WithIndex(e.index),
	)
	if err != nil {
		return fmt.Errorf("ES refresh: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		return fmt.Errorf("ES refresh failed: %s", res.Status())
	}
	return nil
}

// DeleteIndex removes this concrete index. It is intended for cleaning up a
// failed rebuild target and must not be called with the public search alias.
func (e *ESIndexer) DeleteIndex(ctx context.Context) error {
	res, err := (esapi.IndicesDeleteRequest{Index: []string{e.index}}).Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("ES delete index: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES delete index failed status=%s body=%s", res.Status(), string(raw))
	}
	return nil
}

// PromoteToAlias atomically makes this concrete index the write target for
// alias. The first promotion can migrate a legacy physical index with the same
// name by using remove_index and add in one aliases request.
func (e *ESIndexer) PromoteToAlias(ctx context.Context, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" || alias == e.index {
		return fmt.Errorf("ES promotion requires a distinct non-empty alias")
	}

	actions := make([]map[string]any, 0, 3)
	aliasResp, err := (esapi.IndicesGetAliasRequest{Name: []string{alias}}).Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("ES get alias: %w", err)
	}
	switch aliasResp.StatusCode {
	case http.StatusOK:
		var aliases map[string]json.RawMessage
		decodeErr := json.NewDecoder(aliasResp.Body).Decode(&aliases)
		_ = aliasResp.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("ES decode alias response: %w", decodeErr)
		}
		for indexName := range aliases {
			actions = append(actions, map[string]any{"remove": map[string]any{
				"index": indexName, "alias": alias,
			}})
		}
	case http.StatusNotFound:
		_ = aliasResp.Body.Close()
		exists, existsErr := e.client.Indices.Exists(
			[]string{alias}, e.client.Indices.Exists.WithContext(ctx),
		)
		if existsErr != nil {
			return fmt.Errorf("ES inspect legacy index: %w", existsErr)
		}
		if exists.StatusCode == http.StatusOK {
			actions = append(actions, map[string]any{"remove_index": map[string]any{"index": alias}})
		} else if exists.StatusCode != http.StatusNotFound {
			raw, _ := io.ReadAll(exists.Body)
			_ = exists.Body.Close()
			return fmt.Errorf("ES inspect legacy index failed status=%s body=%s", exists.Status(), string(raw))
		}
		_ = exists.Body.Close()
	default:
		raw, _ := io.ReadAll(aliasResp.Body)
		_ = aliasResp.Body.Close()
		return fmt.Errorf("ES get alias failed status=%s body=%s", aliasResp.Status(), string(raw))
	}

	actions = append(actions, map[string]any{"add": map[string]any{
		"index": e.index, "alias": alias, "is_write_index": true,
	}})
	body, err := json.Marshal(map[string]any{"actions": actions})
	if err != nil {
		return fmt.Errorf("ES marshal alias promotion: %w", err)
	}
	res, err := (esapi.IndicesUpdateAliasesRequest{Body: bytes.NewReader(body)}).Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("ES promote alias: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES promote alias failed status=%s body=%s", res.Status(), string(raw))
	}
	return nil
}

// PostIndexMapping 是帖子索引的 ES mapping。title/body 使用 ES 内置 cjk 分词
// （中文二元组，DISC-021：standard 分词下中文整句作为一个 token，子串查询无法匹配；
// 部署侧可换 IK 提升精度），其余 keyword 字段直接精确匹配。
const PostIndexMapping = `{
  "mappings": {
    "properties": {
      "post_id":     {"type": "long"},
      "author_id":   {"type": "long"},
      "category_id": {"type": "long"},
      "title":       {"type": "text", "analyzer": "cjk", "search_analyzer": "cjk"},
      "body":        {"type": "text", "analyzer": "cjk", "search_analyzer": "cjk"},
      "tags":        {"type": "keyword"},
	  "like_count":  {"type": "long"},
	  "comment_count":{"type": "long"},
      "created_at":  {"type": "date", "format": "epoch_millis"},
      "revision":    {"type": "long"},
      "deleted":     {"type": "boolean"}
    }
  }
}`

// PostEventToIndexDoc 把 PostEvent 转成 ESIndexer 可消费的 IndexDoc。
// 提供给消费者层使用，避免消费者直接耦合 ES 字段命名。
func PostEventToIndexDoc(e event.PostEvent) IndexDoc {
	createdAt := e.CreatedAt
	if createdAt <= 0 {
		createdAt = e.EventTime
	}
	// 只有 counted 事件的序号推进计数；created/updated 未带序号时不得用事件时间冒充，
	// 否则较晚生成的旧快照会压过更新的计数补丁。
	statsSeq := e.StatsSeq
	if statsSeq <= 0 && e.Type == event.PostEventCounted {
		statsSeq = e.EventTime
	}
	doc := IndexDoc{
		DocID:    strconv.FormatInt(e.PostID, 10),
		Type:     string(e.Type),
		Revision: e.Revision,
		Body: map[string]any{
			"post_id":       e.PostID,
			"author_id":     e.AuthorID,
			"category_id":   e.CategoryID,
			"title":         e.Title,
			"body":          e.IndexText(),
			"tags":          e.Tags,
			"like_count":    e.LikeCount,
			"comment_count": e.CommentCount,
			"created_at":    createdAt,
			"revision":      e.Revision,
		},
	}
	if statsSeq > 0 {
		doc.Body["stats_seq"] = statsSeq
	}
	return doc
}
