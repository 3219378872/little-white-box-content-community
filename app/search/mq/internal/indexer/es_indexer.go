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

// Index 把 PostEvent 序列化为 ES 文档 upsert。
// 输入参数 doc 中的 Body 字段在 PostEvent → IndexDoc 适配中携带原始事件。
func (e *ESIndexer) Index(ctx context.Context, doc IndexDoc) error {
	body, err := json.Marshal(doc.Body)
	if err != nil {
		return fmt.Errorf("marshal index body: %w", err)
	}
	req := esapi.IndexRequest{
		Index:      e.index,
		DocumentID: doc.DocID,
		Body:       bytes.NewReader(body),
		Refresh:    "false",
	}
	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("ES index request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES index failed status=%s body=%s", res.Status(), string(raw))
	}
	return nil
}

func (e *ESIndexer) Delete(ctx context.Context, docID string) error {
	req := esapi.DeleteRequest{
		Index:      e.index,
		DocumentID: docID,
		Refresh:    "false",
	}
	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("ES delete request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	// 404 视为已删除（幂等）
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.IsError() {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES delete failed status=%s body=%s", res.Status(), string(raw))
	}
	return nil
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

// Refresh 强制刷新索引，仅供测试场景使用，业务路径不应调用。
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

// PostIndexMapping 是帖子索引的 ES mapping。title/body 使用 standard 分词
// （生产可换 IK，由部署侧 plugin 配置），其余 keyword 字段直接精确匹配。
const PostIndexMapping = `{
  "mappings": {
    "properties": {
      "post_id":     {"type": "long"},
      "author_id":   {"type": "long"},
      "category_id": {"type": "long"},
      "title":       {"type": "text"},
      "body":        {"type": "text"},
      "tags":        {"type": "keyword"},
      "created_at":  {"type": "date", "format": "epoch_millis"}
    }
  }
}`

// PostEventToIndexDoc 把 PostEvent 转成 ESIndexer 可消费的 IndexDoc。
// 提供给消费者层使用，避免消费者直接耦合 ES 字段命名。
func PostEventToIndexDoc(e event.PostEvent) IndexDoc {
	return IndexDoc{
		DocID: strconv.FormatInt(e.PostID, 10),
		Type:  string(e.Type),
		Body: map[string]any{
			"post_id":     e.PostID,
			"author_id":   e.AuthorID,
			"category_id": e.CategoryID,
			"title":       e.Title,
			"body":        e.BodyExcerpt,
			"tags":        e.Tags,
			"created_at":  e.EventTime,
		},
	}
}
