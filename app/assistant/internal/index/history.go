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
	if c == nil || c.es == nil || c.store == nil || sess == nil || sess.UserID <= 0 {
		return "", errx.New(errx.ServiceUnavailable, "assistant history search unavailable")
	}
	limit := historyLimit(args.Limit)
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()
	shape := strings.ToLower(strings.TrimSpace(args.Shape))
	switch shape {
	case "around":
		if args.MessageID <= 0 {
			return "", errx.New(errx.ParamError, "search_history around requires message_id")
		}
		messages, err := c.store.ListHistoryAround(ctx, sess.UserID, args.MessageID, 5, 5, cutoff, sess.LiveMessageIDs)
		if err != nil {
			return "", err
		}
		if len(messages) == 0 {
			return noHistoryResult(), nil
		}
		summaries, err := c.store.ListHistorySessionSummaries(ctx, sess.UserID, messages[0].SessionID, 1, cutoff, sess.LiveMessageIDs)
		if err != nil {
			return "", err
		}
		return formatHistoryAround(args.MessageID, messages, summaries), nil
	case "session":
		if args.SessionID <= 0 {
			return "", errx.New(errx.ParamError, "search_history session requires session_id")
		}
		summaries, err := c.store.ListHistorySessionSummaries(ctx, sess.UserID, args.SessionID, 1, cutoff, sess.LiveMessageIDs)
		if err != nil {
			return "", err
		}
		return formatHistorySummaries(summaries), nil
	case "recent":
		summaries, err := c.store.ListHistorySessionSummaries(ctx, sess.UserID, 0, limit, cutoff, sess.LiveMessageIDs)
		if err != nil {
			return "", err
		}
		return formatHistorySummaries(summaries), nil
	case "keywords":
		if strings.TrimSpace(args.Query) == "" {
			return "", errx.New(errx.ParamError, "search_history keywords requires query")
		}
	default:
		return "", errx.New(errx.ParamError, "search_history shape is invalid")
	}

	ids, err := c.searchKeywordIDs(ctx, sess, strings.TrimSpace(args.Query), cutoff, limit)
	if err != nil {
		return "", err
	}
	messages, err := c.store.GetMessagesByIDs(ctx, sess.UserID, ids)
	if err != nil {
		return "", err
	}
	live := make(map[int64]struct{}, len(sess.LiveMessageIDs))
	for _, id := range sess.LiveMessageIDs {
		live[id] = struct{}{}
	}
	byID := make(map[int64]store.Message, len(messages))
	for _, message := range messages {
		if historyMessageEligible(message, cutoff, live) {
			byID[message.ID] = message
		}
	}
	ranked := make([]store.Message, 0, limit)
	for _, id := range ids {
		if message, ok := byID[id]; ok {
			ranked = append(ranked, message)
			if len(ranked) == limit {
				break
			}
		}
	}
	if len(ranked) == 0 {
		return noHistoryResult(), nil
	}
	return c.formatKeywordResults(ctx, sess, ranked, cutoff)
}

func (c *Client) searchKeywordIDs(ctx context.Context, sess *tool.Session, text string, cutoff int64, limit int) ([]int64, error) {
	size := limit * 5
	if size < 10 {
		size = 10
	}
	if size > 50 {
		size = 50
	}
	query := map[string]any{
		"size": size,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]any{"userId": sess.UserID}},
					map[string]any{"term": map[string]any{"deleted": false}},
					map[string]any{"terms": map[string]any{"role": []string{store.RoleUser, store.RoleAssistant}}},
					map[string]any{"range": map[string]any{"createdAtMs": map[string]any{"gte": cutoff}}},
				},
				"must": []any{map[string]any{"match": map[string]any{"content": text}}},
			},
		},
	}
	boolQuery := query["query"].(map[string]any)["bool"].(map[string]any)
	if len(sess.LiveMessageIDs) > 0 {
		boolQuery["must_not"] = []any{map[string]any{"terms": map[string]any{"messageId": sess.LiveMessageIDs}}}
	}
	raw, _ := json.Marshal(query)
	res, err := c.es.Search(c.es.Search.WithContext(ctx), c.es.Search.WithIndex(IndexName), c.es.Search.WithBody(bytes.NewReader(raw)))
	if err != nil {
		return nil, errx.New(errx.ServiceUnavailable, "assistant history search unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		return nil, errx.New(errx.ServiceUnavailable, "assistant history search unavailable")
	}
	var parsed struct {
		Hits struct {
			Hits []struct {
				Source Document `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(parsed.Hits.Hits))
	seen := map[int64]struct{}{}
	for _, hit := range parsed.Hits.Hits {
		id := hit.Source.MessageID
		if id <= 0 {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c *Client) formatKeywordResults(ctx context.Context, sess *tool.Session, ranked []store.Message, cutoff int64) (string, error) {
	var b strings.Builder
	for index, message := range ranked {
		fmt.Fprintf(&b, "%d. session=%d message=%d %s: %s\n", index+1, message.SessionID, message.ID, message.Role, store.Preview(message.Content, 160))
		if index != 0 {
			continue
		}
		around, err := c.store.ListHistoryAround(ctx, sess.UserID, message.ID, 5, 5, cutoff, sess.LiveMessageIDs)
		if err != nil {
			return "", err
		}
		b.WriteString("  上下文：\n")
		for _, item := range around {
			fmt.Fprintf(&b, "  - [%d] %s: %s\n", item.ID, item.Role, store.Preview(item.Content, 80))
		}
		summaries, err := c.store.ListHistorySessionSummaries(ctx, sess.UserID, message.SessionID, 1, cutoff, sess.LiveMessageIDs)
		if err != nil {
			return "", err
		}
		appendHistorySummary(&b, summaries)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func formatHistoryAround(anchorID int64, messages []store.Message, summaries []store.HistorySessionSummary) string {
	if len(messages) == 0 {
		return noHistoryResult()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "锚点 message=%d 的上下文：\n", anchorID)
	for _, message := range messages {
		marker := ""
		if message.ID == anchorID {
			marker = " [anchor]"
		}
		fmt.Fprintf(&b, "- [%d]%s %s: %s\n", message.ID, marker, message.Role, store.Preview(message.Content, 160))
	}
	appendHistorySummary(&b, summaries)
	return strings.TrimRight(b.String(), "\n")
}

func formatHistorySummaries(summaries []store.HistorySessionSummary) string {
	if len(summaries) == 0 {
		return noHistoryResult()
	}
	var b strings.Builder
	appendHistorySummary(&b, summaries)
	return strings.TrimRight(b.String(), "\n")
}

func appendHistorySummary(b *strings.Builder, summaries []store.HistorySessionSummary) {
	for index, summary := range summaries {
		fmt.Fprintf(b, "  会话摘要 %d: session=%d first=[%d] %s: %s", index+1, summary.SessionID,
			summary.First.ID, summary.First.Role, store.Preview(summary.First.Content, 100))
		if summary.Last.ID != summary.First.ID {
			fmt.Fprintf(b, " | last=[%d] %s: %s", summary.Last.ID, summary.Last.Role, store.Preview(summary.Last.Content, 100))
		}
		b.WriteByte('\n')
	}
}

func historyMessageEligible(message store.Message, cutoff int64, live map[int64]struct{}) bool {
	if message.DeletedAtMs != 0 || !message.Visible || message.CreatedAtMs < cutoff {
		return false
	}
	if message.Role != store.RoleUser && message.Role != store.RoleAssistant {
		return false
	}
	if message.Kind != store.KindMessage && message.Kind != store.KindWatch {
		return false
	}
	_, excluded := live[message.ID]
	return !excluded
}

func historyLimit(limit int) int {
	if limit <= 0 {
		return 3
	}
	if limit > 10 {
		return 10
	}
	return limit
}

func noHistoryResult() string {
	return "没有召回到可展示的历史消息。"
}
