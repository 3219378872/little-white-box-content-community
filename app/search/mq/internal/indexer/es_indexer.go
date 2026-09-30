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

// Content revision and stats sequence are application watermarks. ES internal
// _version changes on every count patch, so it must never gate lifecycle writes.
const indexSnapshotScript = `
long storedRevision = ctx._source.revision == null ? 0L : (long) ctx._source.revision;
long incomingRevision = params.revision;
boolean deleted = ctx._source.projection_deleted == true;
boolean replaceBody = incomingRevision > storedRevision || (incomingRevision <= 0L && storedRevision <= 0L && !deleted);
boolean changed = false;
if (replaceBody) {
  for (def entry : params.doc.entrySet()) {
    if (entry.getKey() != 'like_count' && entry.getKey() != 'comment_count' && entry.getKey() != 'stats_seq') {
      ctx._source[entry.getKey()] = entry.getValue();
    }
  }
  ctx._source.revision = incomingRevision;
  ctx._source.projection_deleted = false;
  changed = true;
}
long storedStats = ctx._source.stats_seq == null ? 0L : (long) ctx._source.stats_seq;
long incomingStats = params.doc.stats_seq == null ? 0L : (long) params.doc.stats_seq;
if (ctx._source.post_id != null && ctx._source.projection_deleted != true && (ctx._source.like_count == null || incomingStats > storedStats)) {
  ctx._source.like_count = params.doc.like_count;
  ctx._source.comment_count = params.doc.comment_count;
  ctx._source.stats_seq = incomingStats;
  changed = true;
}
if (!changed) { ctx.op = 'noop'; }
`

// Index atomically applies independent content and count snapshot versions.
func (e *ESIndexer) Index(ctx context.Context, doc IndexDoc) error {
	return e.applySnapshot(ctx, doc.DocID, indexSnapshotScript, map[string]any{"doc": doc.Body, "revision": doc.Revision})
}
func (e *ESIndexer) applySnapshot(ctx context.Context, id, script string, params map[string]any) error {
	payload, err := json.Marshal(map[string]any{"scripted_upsert": true, "upsert": map[string]any{}, "script": map[string]any{"lang": "painless", "source": script, "params": params}})
	if err != nil {
		return fmt.Errorf("marshal projection snapshot: %w", err)
	}
	retries := 8
	response, err := (esapi.UpdateRequest{Index: e.index, DocumentID: id, Body: bytes.NewReader(payload), Refresh: "false", RetryOnConflict: &retries}).Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("ES projection update: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.IsError() {
		raw, _ := io.ReadAll(response.Body)
		return fmt.Errorf("ES projection update failed status=%s body=%s", response.Status(), string(raw))
	}
	return nil
}

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
	retries := 8
	res, err := (esapi.UpdateRequest{
		RetryOnConflict: &retries,
		Index:           e.index,
		DocumentID:      doc.DocID,
		Body:            bytes.NewReader(payload),
		Refresh:         "false",
	}).Do(ctx, e.client)
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

// Keep a durable content-revision tombstone. Physical deletion would lose the
// application watermark and allow a delayed old full snapshot to resurrect it.
const deleteSnapshotScript = `
long stored = ctx._source.revision == null ? 0L : (long) ctx._source.revision;
long incoming = params.revision;
if (incoming > stored || (incoming <= 0L && stored <= 0L)) {
  def stats = ctx._source.stats_seq;
  def likes = ctx._source.like_count;
  def comments = ctx._source.comment_count;
  ctx._source.clear();
  ctx._source.revision = incoming;
  ctx._source.projection_deleted = true;
  if (stats != null) { ctx._source.stats_seq = stats; ctx._source.like_count = likes; ctx._source.comment_count = comments; }
} else { ctx.op = 'noop'; }
`

func (e *ESIndexer) Delete(ctx context.Context, docID string, revision int64) error {
	return e.applySnapshot(ctx, docID, deleteSnapshotScript, map[string]any{"revision": revision})
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
      "stats_seq": {"type": "long"},
      "projection_deleted": {"type": "boolean"}
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
	statsSeq := e.StatsSeq
	if statsSeq <= 0 {
		statsSeq = e.EventTime
	}
	return IndexDoc{
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
			"stats_seq":     statsSeq,
		},
	}
}
