package index

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/pkg/errx"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/zeromicro/go-zero/core/logx"
)

const IndexName = "assistant-history-v1"
const retentionDays = 365

type Document struct {
	UserID      int64  `json:"userId"`
	SessionID   int64  `json:"sessionId"`
	MessageID   int64  `json:"messageId"`
	Role        string `json:"role"`
	Content     string `json:"content"`
	CreatedAtMs int64  `json:"createdAtMs"`
	Deleted     bool   `json:"deleted"`
	Compacted   bool   `json:"compacted"`
}

type Client struct {
	es    *elasticsearch.Client
	store store.Store
}

func New(addresses []string, username, password string, st store.Store) (*Client, error) {
	cleaned := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" || strings.Contains(addr, "${") {
			continue
		}
		cleaned = append(cleaned, addr)
	}
	addresses = cleaned
	if len(addresses) == 0 {
		return nil, nil
	}
	cfg := elasticsearch.Config{Addresses: addresses, Username: username, Password: password}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	c := &Client{es: es, store: st}
	if err := c.ensureIndex(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) ensureIndex(ctx context.Context) error {
	if c == nil || c.es == nil {
		return nil
	}
	mapping := `{"settings":{"index":{"analysis":{"analyzer":{"default":{"type":"cjk"}}}}},"mappings":{"properties":{"userId":{"type":"long"},"sessionId":{"type":"long"},"messageId":{"type":"long"},"role":{"type":"keyword"},"content":{"type":"text","analyzer":"cjk"},"createdAtMs":{"type":"long"},"deleted":{"type":"boolean"},"compacted":{"type":"boolean"}}}}`
	res, err := c.es.Indices.Exists([]string{IndexName}, c.es.Indices.Exists.WithContext(ctx))
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode == 200 {
		return nil
	}
	created, err := c.es.Indices.Create(IndexName, c.es.Indices.Create.WithContext(ctx), c.es.Indices.Create.WithBody(strings.NewReader(mapping)))
	if err != nil {
		return err
	}
	defer func() { _ = created.Body.Close() }()
	if created.IsError() && created.StatusCode != 400 {
		raw, _ := io.ReadAll(created.Body)
		return fmt.Errorf("create history index: %s", raw)
	}
	return nil
}

func (c *Client) Relay(ctx context.Context) error {
	if c == nil || c.store == nil {
		return nil
	}
	rows, err := c.store.ListUnpublishedOutbox(ctx, 50)
	if err != nil || len(rows) == 0 {
		return err
	}
	if c.es == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	published := make([]int64, 0, len(rows))
	for _, row := range rows {
		if err := c.apply(ctx, row); err != nil {
			logx.WithContext(ctx).Errorw("assistant history outbox failed", logx.Field("id", row.ID), logx.Field("err", err.Error()))
			continue
		}
		published = append(published, row.ID)
	}
	if len(published) > 0 {
		return c.store.MarkOutboxPublished(ctx, published)
	}
	return nil
}

func (c *Client) apply(ctx context.Context, row store.Outbox) error {
	docID := fmt.Sprintf("%d", row.MessageID)
	if row.Op == store.IndexOpDelete {
		req := esapi.DeleteRequest{Index: IndexName, DocumentID: docID}
		res, err := req.Do(ctx, c.es)
		if err != nil {
			return err
		}
		defer func() { _ = res.Body.Close() }()
		return nil
	}
	var doc Document
	if err := json.Unmarshal([]byte(row.PayloadJSON), &doc); err != nil {
		doc = Document{UserID: row.UserID, MessageID: row.MessageID}
	}
	raw, _ := json.Marshal(doc)
	req := esapi.IndexRequest{Index: IndexName, DocumentID: docID, Body: bytes.NewReader(raw), Refresh: "false"}
	res, err := req.Do(ctx, c.es)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		return fmt.Errorf("index message %s", res.Status())
	}
	return nil
}

func (c *Client) Search(ctx context.Context, sess *tool.Session, args tool.HistoryArgs) (string, error) {
	if c == nil || c.es == nil {
		return "", errx.New(errx.ServiceUnavailable, "assistant history search unavailable")
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 3
	}
	if limit > 10 {
		limit = 10
	}
	query := map[string]any{
		"size": limit,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]any{"userId": sess.UserID}},
					map[string]any{"term": map[string]any{"deleted": false}},
					map[string]any{"range": map[string]any{"createdAtMs": map[string]any{"gte": time.Now().AddDate(0, 0, -retentionDays).UnixMilli()}}},
				},
			},
		},
	}
	boolQuery := query["query"].(map[string]any)["bool"].(map[string]any)
	switch strings.ToLower(strings.TrimSpace(args.Shape)) {
	case "around":
		boolQuery["must"] = []any{map[string]any{"term": map[string]any{"messageId": args.MessageID}}}
	case "session":
		boolQuery["filter"] = append(boolQuery["filter"].([]any), map[string]any{"term": map[string]any{"sessionId": args.SessionID}})
	case "recent":
		query["sort"] = []any{map[string]any{"createdAtMs": "desc"}}
	default:
		if strings.TrimSpace(args.Query) == "" {
			return "", errx.New(errx.ParamError, "search_history query is required")
		}
		boolQuery["must"] = []any{map[string]any{"match": map[string]any{"content": args.Query}}}
	}
	raw, _ := json.Marshal(query)
	res, err := c.es.Search(c.es.Search.WithContext(ctx), c.es.Search.WithIndex(IndexName), c.es.Search.WithBody(bytes.NewReader(raw)))
	if err != nil {
		return "", errx.New(errx.ServiceUnavailable, "assistant history search unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		return "", errx.New(errx.ServiceUnavailable, "assistant history search unavailable")
	}
	var parsed struct {
		Hits struct {
			Hits []struct {
				Source Document `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return "", err
	}
	ids := make([]int64, 0, len(parsed.Hits.Hits))
	for _, hit := range parsed.Hits.Hits {
		ids = append(ids, hit.Source.MessageID)
	}
	msgs, err := c.store.GetMessagesByIDs(ctx, sess.UserID, ids)
	if err != nil {
		return "", err
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()
	byID := map[int64]store.Message{}
	for _, msg := range msgs {
		if msg.DeletedAtMs != 0 || msg.CreatedAtMs < cutoff {
			continue
		}
		if msg.Role != store.RoleUser && msg.Role != store.RoleAssistant {
			continue
		}
		if !msg.Visible || msg.Kind == store.KindMemoryChanged {
			continue
		}
		byID[msg.ID] = msg
	}
	var b strings.Builder
	rank := 0
	for _, id := range ids {
		msg, ok := byID[id]
		if !ok {
			continue
		}
		rank++
		fmt.Fprintf(&b, "%d. session=%d message=%d %s: %s\n", rank, msg.SessionID, msg.ID, msg.Role, store.Preview(msg.Content, 160))
		if rank == 1 {
			around, _ := c.store.ListMessages(ctx, sess.UserID, msg.SessionID, 0, 50)
			b.WriteString("  上下文：")
			for _, item := range around {
				if item.ID == msg.ID {
					continue
				}
				fmt.Fprintf(&b, " [%d]%s", item.ID, store.Preview(item.Content, 40))
			}
			b.WriteByte('\n')
		}
	}
	if rank == 0 {
		return "没有召回到可展示的历史消息。", nil
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
