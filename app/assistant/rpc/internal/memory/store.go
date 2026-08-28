package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

const (
	LayerProfile  = "profile"
	LayerInterest = "interest"
	LayerEpisodic = "episodic"
	LayerTask     = "task"

	SourceExplicit     = "explicit"
	SourceConversation = "conversation"
	SourceBehavior     = "behavior"

	interestHalfLifeDays = 30.0
	interestFloor        = 0.05
	confirmedConfidence  = 0.8

	layerShift = 48
)

var (
	ErrNotFound = errors.New("memory item not found")
	layerCodes  = map[string]int64{LayerProfile: 1, LayerInterest: 2, LayerEpisodic: 3, LayerTask: 4}
	codeLayers  = map[int64]string{1: LayerProfile, 2: LayerInterest, 3: LayerEpisodic, 4: LayerTask}
)

type Item struct {
	ID           int64
	UserID       int64
	Layer        string
	Dimension    string
	Value        string
	Score        float64
	Source       string
	Confidence   float64
	Suppressed   bool
	UpdatedAt    int64
	ExcludedJSON string
}

func (i Item) Confirmed() bool {
	return !i.Suppressed && i.Confidence >= confirmedConfidence
}

func PackID(layer string, recordID int64) int64 {
	code := layerCodes[layer]
	if code == 0 || recordID <= 0 {
		return 0
	}
	return (code << layerShift) | recordID
}

func UnpackID(id int64) (layer string, recordID int64, ok bool) {
	if id <= 0 {
		return "", 0, false
	}
	code := id >> layerShift
	recordID = id & ((1 << layerShift) - 1)
	layer, ok = codeLayers[code]
	return layer, recordID, ok && recordID > 0
}

type Patch struct {
	Value      *string
	Score      *float64
	Suppressed *bool
}

type Candidate struct {
	Layer      string
	Dimension  string
	Value      string
	Score      float64
	Source     string
	Confidence float64
	Excerpt    string
	Suppressed bool
}

type Store interface {
	List(ctx context.Context, userID int64, layer string, now time.Time) ([]Item, error)
	Update(ctx context.Context, userID, id int64, patch Patch, now time.Time) error
	Delete(ctx context.Context, userID, id int64) error
	Apply(ctx context.Context, userID int64, candidate Candidate, now time.Time) error
	ContextBlock(ctx context.Context, userID int64, intent string, now time.Time, skipBehavior bool) (string, error)
	RecordFeedback(ctx context.Context, userID int64, requestID string, postID int64, reason string) error
}

type SQLStore struct {
	conn            sqlx.SqlConn
	Personalization func(ctx context.Context, userID int64) (bool, error)
}

func NewSQLStore(conn sqlx.SqlConn) *SQLStore {
	return &SQLStore{conn: conn}
}

func (s *SQLStore) List(ctx context.Context, userID int64, layer string, now time.Time) ([]Item, error) {
	if userID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	layers := []string{LayerProfile, LayerInterest, LayerTask}
	if layer != "" {
		if layer == LayerEpisodic {
			return s.listEpisodic(ctx, userID)
		}
		layers = []string{layer}
	}
	items := make([]Item, 0)
	for _, name := range layers {
		part, err := s.listLayer(ctx, userID, name, now)
		if err != nil {
			return nil, err
		}
		items = append(items, part...)
	}
	return items, nil
}

func (s *SQLStore) listLayer(ctx context.Context, userID int64, layer string, now time.Time) ([]Item, error) {
	type row struct {
		ID            int64   `db:"id"`
		Dimension     string  `db:"dimension"`
		Value         string  `db:"value"`
		Score         float64 `db:"score"`
		Source        string  `db:"source"`
		Confidence    float64 `db:"confidence"`
		Suppressed    int64   `db:"suppressed"`
		UpdatedAtMs   int64   `db:"updated_at_ms"`
		LastEventAtMs int64   `db:"last_event_at_ms"`
		IntentText    string  `db:"intent_text"`
		Status        string  `db:"status"`
		ExcludedJSON  string  `db:"excluded_json"`
	}
	query := ""
	switch layer {
	case LayerProfile:
		query = `SELECT id, dimension, value, score, source, confidence, suppressed, updated_at_ms, 0 AS last_event_at_ms, '' AS intent_text, '' AS status, '' AS excluded_json
			FROM user_preference WHERE user_id = ? ORDER BY updated_at_ms DESC`
	case LayerInterest:
		query = `SELECT id, dimension, value, score, source, confidence, suppressed, updated_at_ms, last_event_at_ms, '' AS intent_text, '' AS status, '' AS excluded_json
			FROM user_interest WHERE user_id = ? ORDER BY last_event_at_ms DESC`
	case LayerTask:
		query = `SELECT id, 'task' AS dimension, intent_text AS value, 1 AS score, 'conversation' AS source, 1 AS confidence, 0 AS suppressed, updated_at_ms, 0 AS last_event_at_ms, intent_text, status, COALESCE(excluded_json, '') AS excluded_json
			FROM task_memory WHERE user_id = ? AND status = 'open' ORDER BY updated_at_ms DESC`
	default:
		return nil, errx.NewWithCode(errx.ParamError)
	}
	var rows []row
	if err := s.conn.QueryRowsCtx(ctx, &rows, query, userID); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, item := range rows {
		score := item.Score
		if layer == LayerInterest {
			score = decayInterest(item.Score, item.LastEventAtMs, now)
		}
		value := item.Value
		if layer == LayerTask {
			value = item.IntentText
		}
		items = append(items, Item{
			ID: PackID(layer, item.ID), UserID: userID, Layer: layer,
			Dimension: item.Dimension, Value: value, Score: score, Source: item.Source,
			Confidence: item.Confidence, Suppressed: item.Suppressed == 1, UpdatedAt: item.UpdatedAtMs,
			ExcludedJSON: item.ExcludedJSON,
		})
	}
	return items, nil
}

func (s *SQLStore) listEpisodic(ctx context.Context, userID int64) ([]Item, error) {
	var rows []struct {
		ID           int64  `db:"id"`
		Kind         string `db:"kind"`
		Summary      string `db:"summary"`
		HappenedAtMs int64  `db:"happened_at_ms"`
	}
	if err := s.conn.QueryRowsCtx(ctx, &rows,
		`SELECT id, kind, summary, happened_at_ms FROM user_memory WHERE user_id = ? ORDER BY happened_at_ms DESC LIMIT 20`,
		userID); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, item := range rows {
		items = append(items, Item{
			ID: PackID(LayerEpisodic, item.ID), UserID: userID, Layer: LayerEpisodic,
			Dimension: item.Kind, Value: item.Summary, Score: 1, Source: SourceConversation,
			Confidence: 1, UpdatedAt: item.HappenedAtMs,
		})
	}
	return items, nil
}

func (s *SQLStore) Update(ctx context.Context, userID, id int64, patch Patch, now time.Time) error {
	layer, recordID, ok := UnpackID(id)
	if !ok || userID <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	items, err := s.List(ctx, userID, layer, now)
	if err != nil {
		return err
	}
	var current *Item
	for i := range items {
		if items[i].ID == id {
			current = &items[i]
			break
		}
	}
	if current == nil {
		return ErrNotFound
	}
	value, score, suppressed := current.Value, current.Score, current.Suppressed
	if patch.Value != nil {
		value = strings.TrimSpace(*patch.Value)
	}
	if patch.Score != nil {
		score = *patch.Score
	}
	if patch.Suppressed != nil {
		suppressed = *patch.Suppressed
	}
	if !validMemoryValueScore(value, score) {
		return errx.NewWithCode(errx.ParamError)
	}
	switch layer {
	case LayerProfile:
		_, err = s.conn.ExecCtx(ctx,
			`UPDATE user_preference SET value = ?, score = ?, suppressed = ?, source = ?, updated_at_ms = ? WHERE id = ? AND user_id = ?`,
			value, score, boolToInt(suppressed), SourceExplicit, now.UnixMilli(), recordID, userID)
	case LayerInterest:
		_, err = s.conn.ExecCtx(ctx,
			`UPDATE user_interest SET value = ?, score = ?, suppressed = ?, source = ?, last_event_at_ms = ?, updated_at_ms = ? WHERE id = ? AND user_id = ?`,
			value, score, boolToInt(suppressed), SourceExplicit, now.UnixMilli(), now.UnixMilli(), recordID, userID)
	case LayerTask:
		_, err = s.conn.ExecCtx(ctx,
			`UPDATE task_memory SET intent_text = ?, updated_at_ms = ? WHERE id = ? AND user_id = ?`,
			value, now.UnixMilli(), recordID, userID)
	default:
		return errx.NewWithCode(errx.ParamError)
	}
	return err
}

func (s *SQLStore) Delete(ctx context.Context, userID, id int64) error {
	layer, recordID, ok := UnpackID(id)
	if !ok || userID <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	var res sql.Result
	var err error
	switch layer {
	case LayerProfile:
		res, err = s.conn.ExecCtx(ctx, `DELETE FROM user_preference WHERE id = ? AND user_id = ?`, recordID, userID)
	case LayerInterest:
		res, err = s.conn.ExecCtx(ctx, `DELETE FROM user_interest WHERE id = ? AND user_id = ?`, recordID, userID)
	case LayerTask:
		res, err = s.conn.ExecCtx(ctx, `DELETE FROM task_memory WHERE id = ? AND user_id = ?`, recordID, userID)
	case LayerEpisodic:
		res, err = s.conn.ExecCtx(ctx, `DELETE FROM user_memory WHERE id = ? AND user_id = ?`, recordID, userID)
	default:
		return errx.NewWithCode(errx.ParamError)
	}
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) Apply(ctx context.Context, userID int64, candidate Candidate, now time.Time) error {
	if userID <= 0 || !validMemoryValueScore(candidate.Value, candidate.Score) {
		return errx.NewWithCode(errx.ParamError)
	}
	if candidate.Source != SourceExplicit && candidate.Source != SourceConversation && candidate.Source != SourceBehavior {
		return errx.NewWithCode(errx.ParamError)
	}
	if candidate.Source == SourceBehavior && !behaviorAllowed(ctx, s.Personalization, userID) {
		return nil
	}
	ms := now.UnixMilli()
	switch candidate.Layer {
	case LayerProfile:
		_, err := s.conn.ExecCtx(ctx, `
			INSERT INTO user_preference (user_id, dimension, value, score, source, confidence, suppressed, history_json, updated_at_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?)
			ON DUPLICATE KEY UPDATE
			  history_json = JSON_ARRAY_APPEND(COALESCE(history_json, JSON_ARRAY()), '$', JSON_OBJECT('score', score, 'source', source, 'updated_at_ms', updated_at_ms)),
			  score = VALUES(score), source = VALUES(source), confidence = VALUES(confidence),
			  suppressed = IF(suppressed = 1, 1, VALUES(suppressed)), updated_at_ms = VALUES(updated_at_ms)`,
			userID, candidate.Dimension, candidate.Value, candidate.Score, candidate.Source, candidate.Confidence, boolToInt(candidate.Suppressed), ms)
		return err
	case LayerInterest:
		_, err := s.conn.ExecCtx(ctx, `
			INSERT INTO user_interest (user_id, dimension, value, score, source, confidence, suppressed, last_event_at_ms, history_json, updated_at_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)
			ON DUPLICATE KEY UPDATE
			  history_json = JSON_ARRAY_APPEND(COALESCE(history_json, JSON_ARRAY()), '$', JSON_OBJECT('score', score, 'source', source, 'updated_at_ms', updated_at_ms)),
			  score = VALUES(score), source = VALUES(source), confidence = VALUES(confidence),
			  last_event_at_ms = VALUES(last_event_at_ms), updated_at_ms = VALUES(updated_at_ms),
			  suppressed = IF(suppressed = 1, 1, VALUES(suppressed))`,
			userID, candidate.Dimension, candidate.Value, candidate.Score, candidate.Source, candidate.Confidence, boolToInt(candidate.Suppressed), ms, ms)
		return err
	case LayerTask:
		value := strings.TrimSpace(candidate.Value)
		if runes := []rune(value); len(runes) > 512 {
			value = string(runes[:512])
		}
		res, err := s.conn.ExecCtx(ctx, `
			UPDATE task_memory SET updated_at_ms = ? WHERE user_id = ? AND status = 'open' AND intent_text = ?`,
			ms, userID, value)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		_, err = s.conn.ExecCtx(ctx, `
			INSERT INTO task_memory (user_id, status, intent_text, constraints_json, excluded_json, updated_at_ms)
			VALUES (?, 'open', ?, NULL, NULL, ?)`,
			userID, value, ms)
		return err
	case LayerEpisodic:
		_, err := s.conn.ExecCtx(ctx, `
			INSERT INTO user_memory (user_id, happened_at_ms, kind, summary, payload_json)
			VALUES (?, ?, ?, ?, NULL)`,
			userID, ms, candidate.Dimension, candidate.Value)
		return err
	default:
		return errx.NewWithCode(errx.ParamError)
	}
}

func (s *SQLStore) ContextBlock(ctx context.Context, userID int64, intent string, now time.Time, skipBehavior bool) (string, error) {
	items, err := s.List(ctx, userID, "", now)
	if err != nil {
		return "", err
	}
	return formatContext(items, intent, skipBehavior), nil
}

func (s *SQLStore) RecordFeedback(ctx context.Context, userID int64, requestID string, postID int64, reason string) error {
	if userID <= 0 || postID <= 0 || strings.TrimSpace(reason) == "" {
		return errx.NewWithCode(errx.ParamError)
	}
	_, err := s.conn.ExecCtx(ctx,
		`INSERT INTO recommendation_feedback (user_id, request_id, post_id, reason) VALUES (?, ?, ?, ?)`,
		userID, requestID, postID, strings.TrimSpace(reason))
	return err
}

func decayInterest(score float64, lastEventAtMs int64, now time.Time) float64 {
	if lastEventAtMs <= 0 {
		return score
	}
	days := now.Sub(time.UnixMilli(lastEventAtMs)).Hours() / 24
	if days <= 0 {
		return score
	}
	lambda := math.Ln2 / interestHalfLifeDays
	return score * math.Exp(-lambda*days)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func formatContext(items []Item, intent string, skipBehavior bool) string {
	var b strings.Builder
	b.WriteString("关于你的记忆（用户可见，冲突时以更新的为准）：\n")
	wrote := false
	for _, item := range items {
		if item.Suppressed {
			continue
		}
		if skipBehavior && item.Source == SourceBehavior {
			continue
		}
		if item.Layer == LayerInterest && math.Abs(item.Score) < interestFloor {
			continue
		}
		if intent == "recommend" || intent == "community_opinion" {
			if item.Layer == LayerEpisodic {
				continue
			}
		}
		wrote = true
		b.WriteString("- [")
		b.WriteString(item.Layer)
		b.WriteString("] ")
		b.WriteString(item.Dimension)
		b.WriteString("=")
		b.WriteString(item.Value)
		if item.Score < 0 {
			b.WriteString(" (不喜欢)")
		}
		b.WriteByte('\n')
	}
	if !wrote {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

// MapStore 是测试用内存实现，覆盖冲突合并与衰减读取。
type MapStore struct {
	mu              sync.Mutex
	next            int64
	items           map[int64]Item
	Personalization func(ctx context.Context, userID int64) (bool, error)
}

func NewMapStore() *MapStore {
	return &MapStore{next: 1, items: map[int64]Item{}}
}

func (m *MapStore) List(_ context.Context, userID int64, layer string, now time.Time) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Item, 0)
	for _, item := range m.items {
		if item.UserID != userID {
			continue
		}
		if layer != "" && item.Layer != layer {
			continue
		}
		copyItem := item
		if copyItem.Layer == LayerInterest {
			copyItem.Score = decayInterest(item.Score, item.UpdatedAt, now)
		}
		out = append(out, copyItem)
	}
	return out, nil
}

func (m *MapStore) Update(_ context.Context, userID, id int64, patch Patch, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok || item.UserID != userID {
		return ErrNotFound
	}
	if patch.Value != nil {
		item.Value = strings.TrimSpace(*patch.Value)
	}
	if patch.Score != nil {
		item.Score = *patch.Score
	}
	if patch.Suppressed != nil {
		item.Suppressed = *patch.Suppressed
	}
	if !validMemoryValueScore(item.Value, item.Score) {
		return errx.NewWithCode(errx.ParamError)
	}
	item.Source = SourceExplicit
	item.UpdatedAt = now.UnixMilli()
	m.items[id] = item
	return nil
}

func (m *MapStore) Delete(_ context.Context, userID, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok || item.UserID != userID {
		return ErrNotFound
	}
	delete(m.items, id)
	return nil
}

func (m *MapStore) Apply(ctx context.Context, userID int64, candidate Candidate, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	value := strings.TrimSpace(candidate.Value)
	if userID <= 0 || !validMemoryValueScore(value, candidate.Score) {
		return errx.NewWithCode(errx.ParamError)
	}
	if candidate.Source != SourceExplicit && candidate.Source != SourceConversation && candidate.Source != SourceBehavior {
		return errx.NewWithCode(errx.ParamError)
	}
	if candidate.Source == SourceBehavior && !behaviorAllowed(ctx, m.Personalization, userID) {
		return nil
	}
	for id, item := range m.items {
		if item.UserID == userID && item.Layer == candidate.Layer && item.Dimension == candidate.Dimension && item.Value == value {
			if item.Suppressed {
				return nil
			}
			item.Score = candidate.Score
			item.Source = candidate.Source
			item.Confidence = candidate.Confidence
			item.Suppressed = item.Suppressed || candidate.Suppressed
			item.UpdatedAt = now.UnixMilli()
			m.items[id] = item
			return nil
		}
	}
	recordID := m.next
	m.next++
	id := PackID(candidate.Layer, recordID)
	m.items[id] = Item{
		ID: id, UserID: userID, Layer: candidate.Layer, Dimension: candidate.Dimension,
		Value: value, Score: candidate.Score, Source: candidate.Source,
		Confidence: candidate.Confidence, Suppressed: candidate.Suppressed, UpdatedAt: now.UnixMilli(),
	}
	return nil
}

func (m *MapStore) ContextBlock(ctx context.Context, userID int64, intent string, now time.Time, skipBehavior bool) (string, error) {
	items, err := m.List(ctx, userID, "", now)
	if err != nil {
		return "", err
	}
	if !skipBehavior {
		skipBehavior = !behaviorAllowed(ctx, m.Personalization, userID)
	}
	return formatContext(items, intent, skipBehavior), nil
}

func (m *MapStore) RecordFeedback(_ context.Context, userID int64, requestID string, postID int64, reason string) error {
	if userID <= 0 || postID <= 0 || strings.TrimSpace(reason) == "" {
		return errx.NewWithCode(errx.ParamError)
	}
	return m.Apply(context.Background(), userID, Candidate{
		Layer: LayerProfile, Dimension: "post", Value: strconv.FormatInt(postID, 10),
		Score: -0.5, Source: SourceExplicit, Confidence: 0.8, Excerpt: requestID + " " + reason,
	}, time.Now())
}

// Extract 从用户话轮抽出结构化候选；只认显式句式，不把猜测当记忆。
func Extract(message string) []Candidate {
	text := strings.TrimSpace(message)
	if text == "" {
		return nil
	}
	var out []Candidate
	if value, ok := cutPrefix(text, "我不喜欢", "不喜欢"); ok {
		out = appendPrivateFiltered(out, Candidate{Layer: LayerProfile, Dimension: "topic", Value: value, Score: -0.8, Source: SourceConversation, Confidence: 0.9, Excerpt: text})
	}
	if value, ok := cutPrefix(text, "我喜欢"); ok {
		out = appendPrivateFiltered(out, Candidate{Layer: LayerProfile, Dimension: "topic", Value: value, Score: 0.8, Source: SourceConversation, Confidence: 0.9, Excerpt: text})
	}
	if value, ok := cutPrefix(text, "帮我找", "我想找"); ok {
		out = appendPrivateFiltered(out, Candidate{Layer: LayerTask, Dimension: "task", Value: value, Score: 1, Source: SourceConversation, Confidence: 0.85, Excerpt: text})
		out = appendPrivateFiltered(out, Candidate{Layer: LayerInterest, Dimension: "topic", Value: value, Score: 0.7, Source: SourceConversation, Confidence: 0.7, Excerpt: text})
	}
	if value, ok := cutPrefix(text, "不要记住"); ok {
		out = appendPrivateFiltered(out, Candidate{Layer: LayerProfile, Dimension: "topic", Value: value, Score: 0, Source: SourceExplicit, Confidence: 1, Excerpt: text, Suppressed: true})
	}
	return out
}

func appendPrivateFiltered(out []Candidate, candidate Candidate) []Candidate {
	if ContainsPrivateValue(candidate.Value) {
		return out
	}
	return append(out, candidate)
}

func ContainsPrivateValue(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" {
		return false
	}
	if strings.Contains(text, "验证码") || strings.Contains(text, "私信") {
		return true
	}
	compact := make([]rune, 0, len(text))
	digitRunes := 0
	consecutive := 0
	for _, r := range text {
		if unicode.IsSpace(r) || r == '-' {
			continue
		}
		compact = append(compact, r)
		if unicode.IsDigit(r) {
			digitRunes++
			consecutive++
			if consecutive >= 11 {
				return true
			}
			continue
		}
		consecutive = 0
	}
	return len(compact) == 11 && digitRunes >= 9
}

func validMemoryValueScore(value string, score float64) bool {
	value = strings.TrimSpace(value)
	return value != "" && !ContainsPrivateValue(value) && !math.IsNaN(score) && !math.IsInf(score, 0) && score >= -1 && score <= 1
}

func behaviorAllowed(ctx context.Context, lookup func(context.Context, int64) (bool, error), userID int64) bool {
	if lookup == nil || userID <= 0 {
		return false
	}
	enabled, err := lookup(ctx, userID)
	if err != nil {
		return false
	}
	return enabled
}

func ParsePostIDs(value string) []int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "[") {
		var ids []int64
		if err := json.Unmarshal([]byte(value), &ids); err == nil {
			return positiveIDs(ids)
		}
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	if len(fields) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(fields))
	for _, field := range fields {
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil || id <= 0 {
			return nil
		}
		ids = append(ids, id)
	}
	return ids
}

func positiveIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func cutPrefix(text string, prefixes ...string) (string, bool) {
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			value := strings.TrimSpace(strings.Trim(text[len(prefix):], "。.!！"))
			if value != "" {
				return value, true
			}
		}
	}
	return "", false
}
