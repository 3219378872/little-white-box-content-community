package watch

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"esx/pkg/errx"
	"esx/pkg/event"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

const (
	AuthorNewPost   = "author_new_post"
	TagNewPost      = "tag_new_post"
	KeywordNewPost  = "keyword_new_post"
	PostRevised     = "post_revised"
	DiscussionSpike = "discussion_spike"
)

var allowedConditions = map[string]string{
	AuthorNewPost:   "author",
	TagNewPost:      "tag",
	KeywordNewPost:  "keyword",
	PostRevised:     "post",
	DiscussionSpike: "post",
}

type Task struct {
	ID            int64
	UserID        int64
	ConditionType string
	TargetType    string
	TargetID      int64
	TargetText    string
	Enabled       bool
	Version       int32
	CreatedAt     int64
}

type Hit struct {
	ID        int64
	UserID    int64
	TaskID    int64
	PostID    int64
	Title     string
	Summary   string
	CreatedAt int64
	Read      bool
}

type Store interface {
	ListTasks(ctx context.Context, userID int64) ([]Task, error)
	GetTask(ctx context.Context, userID, id int64) (Task, error)
	ListEnabled(ctx context.Context) ([]Task, error)
	Create(ctx context.Context, task Task) (Task, error)
	UpdateEnabled(ctx context.Context, userID, id int64, enabled bool, expectedVersion int32) (Task, error)
	Delete(ctx context.Context, userID, id int64, expectedVersion int32) error
	ListHits(ctx context.Context, userID int64, unreadOnly bool) ([]Hit, error)
	GetHitsByIDs(ctx context.Context, userID int64, ids []int64) ([]Hit, error)
	MarkRead(ctx context.Context, userID int64, ids []int64) error
	RecordHit(ctx context.Context, hit Hit, eventKey string) error
	RecordExecution(ctx context.Context, taskID int64, eventKey, status string, usedLLM bool) error
	CountExecutions(ctx context.Context, taskID int64, eventKeyPrefix string) (int64, error)
}

func ValidateTask(task Task) error {
	wantType, ok := allowedConditions[task.ConditionType]
	if !ok {
		return errx.New(errx.ParamError, "unknown watch condition")
	}
	if task.TargetType != wantType {
		return errx.New(errx.ParamError, "watch target_type does not match condition")
	}
	switch task.ConditionType {
	case AuthorNewPost, PostRevised, DiscussionSpike:
		if task.TargetID <= 0 {
			return errx.New(errx.ParamError, "watch target_id is required")
		}
	case TagNewPost, KeywordNewPost:
		if strings.TrimSpace(task.TargetText) == "" {
			return errx.New(errx.ParamError, "watch target_text is required")
		}
	}
	return nil
}

func Match(task Task, ev event.PostEvent) (hit bool, summary string) {
	if !task.Enabled {
		return false, ""
	}
	switch task.ConditionType {
	case AuthorNewPost:
		if ev.Type == event.PostEventCreated && ev.AuthorID == task.TargetID && ev.Status == 1 {
			return true, "关注的作者发布了新帖"
		}
	case TagNewPost:
		if ev.Type != event.PostEventCreated || ev.Status != 1 {
			return false, ""
		}
		needle := strings.ToLower(strings.TrimSpace(task.TargetText))
		for _, tag := range ev.Tags {
			if strings.ToLower(tag) == needle {
				return true, "标签下出现新帖"
			}
		}
	case KeywordNewPost:
		if ev.Type != event.PostEventCreated || ev.Status != 1 {
			return false, ""
		}
		needle := strings.ToLower(strings.TrimSpace(task.TargetText))
		blob := strings.ToLower(ev.Title + " " + ev.BodyExcerpt)
		if needle != "" && strings.Contains(blob, needle) {
			return true, "关键词匹配到新帖"
		}
	case PostRevised:
		if ev.Type == event.PostEventUpdated && ev.PostID == task.TargetID && ev.Status == 1 {
			return true, "盯着的帖子更新了"
		}
	}
	return false, ""
}

type SQLStore struct {
	conn sqlx.SqlConn
}

func NewSQLStore(conn sqlx.SqlConn) *SQLStore { return &SQLStore{conn: conn} }

func (s *SQLStore) getTask(ctx context.Context, userID, id int64) (Task, error) {
	var row struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		ConditionType string `db:"condition_type"`
		TargetType    string `db:"target_type"`
		TargetID      int64  `db:"target_id"`
		TargetText    string `db:"target_text"`
		Enabled       int64  `db:"enabled"`
		Version       int32  `db:"version"`
		CreatedAt     int64  `db:"created_at_ms"`
	}
	err := s.conn.QueryRowCtx(ctx, &row, `SELECT id, user_id, condition_type, target_type, target_id,
		target_text, enabled, version, UNIX_TIMESTAMP(created_at)*1000 AS created_at_ms
		FROM watch_task WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return Task{}, err
	}
	return Task{
		ID: row.ID, UserID: row.UserID, ConditionType: row.ConditionType, TargetType: row.TargetType,
		TargetID: row.TargetID, TargetText: row.TargetText, Enabled: row.Enabled == 1,
		Version: row.Version, CreatedAt: row.CreatedAt,
	}, nil
}

func (s *SQLStore) ListTasks(ctx context.Context, userID int64) ([]Task, error) {
	var rows []struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		ConditionType string `db:"condition_type"`
		TargetType    string `db:"target_type"`
		TargetID      int64  `db:"target_id"`
		TargetText    string `db:"target_text"`
		Enabled       int64  `db:"enabled"`
		Version       int64  `db:"version"`
		CreatedAt     int64  `db:"created_at_ms"`
	}
	err := s.conn.QueryRowsCtx(ctx, &rows,
		`SELECT id, user_id, condition_type, target_type, target_id, target_text, enabled, version, UNIX_TIMESTAMP(created_at)*1000 AS created_at_ms
		 FROM watch_task WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(rows))
	for _, row := range rows {
		out = append(out, Task{ID: row.ID, UserID: row.UserID, ConditionType: row.ConditionType, TargetType: row.TargetType,
			TargetID: row.TargetID, TargetText: row.TargetText, Enabled: row.Enabled == 1, Version: int32(row.Version), CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (s *SQLStore) GetTask(ctx context.Context, userID, id int64) (Task, error) {
	return s.getTask(ctx, userID, id)
}

func (s *SQLStore) ListEnabled(ctx context.Context) ([]Task, error) {
	var rows []struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		ConditionType string `db:"condition_type"`
		TargetType    string `db:"target_type"`
		TargetID      int64  `db:"target_id"`
		TargetText    string `db:"target_text"`
		Enabled       int64  `db:"enabled"`
		Version       int64  `db:"version"`
		CreatedAt     int64  `db:"created_at_ms"`
	}
	err := s.conn.QueryRowsCtx(ctx, &rows,
		`SELECT id, user_id, condition_type, target_type, target_id, target_text, enabled, version, UNIX_TIMESTAMP(created_at)*1000 AS created_at_ms
		 FROM watch_task WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(rows))
	for _, row := range rows {
		out = append(out, Task{ID: row.ID, UserID: row.UserID, ConditionType: row.ConditionType, TargetType: row.TargetType,
			TargetID: row.TargetID, TargetText: row.TargetText, Enabled: row.Enabled == 1, Version: int32(row.Version), CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (s *SQLStore) Create(ctx context.Context, task Task) (Task, error) {
	if err := ValidateTask(task); err != nil {
		return Task{}, err
	}
	res, err := s.conn.ExecCtx(ctx, `
		INSERT INTO watch_task (user_id, condition_type, target_type, target_id, target_text, enabled)
		VALUES (?, ?, ?, ?, ?, 1)`,
		task.UserID, task.ConditionType, task.TargetType, task.TargetID, strings.TrimSpace(task.TargetText))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return Task{}, errx.New(errx.IdempotencyConflict, "duplicate watch task")
		}
		return Task{}, err
	}
	id, _ := res.LastInsertId()
	task.ID = id
	task.Enabled = true
	task.Version = 1
	task.CreatedAt = time.Now().UnixMilli()
	return task, nil
}

func (s *SQLStore) UpdateEnabled(ctx context.Context, userID, id int64, enabled bool, expectedVersion int32) (Task, error) {
	if expectedVersion <= 0 {
		return Task{}, errx.NewWithCode(errx.ParamError)
	}
	res, err := s.conn.ExecCtx(ctx, `UPDATE watch_task SET enabled = ?, version = version + 1
		WHERE id = ? AND user_id = ? AND version = ?`, boolToInt(enabled), id, userID, expectedVersion)
	if err != nil {
		return Task{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Task{}, err
	}
	if affected > 0 {
		return s.getTask(ctx, userID, id)
	}
	if _, err := s.getTask(ctx, userID, id); err != nil {
		if err == sqlx.ErrNotFound {
			return Task{}, errx.NewWithCode(errx.NotFound)
		}
		return Task{}, err
	}
	return Task{}, errx.New(errx.ContentVersionConflict, "watch version conflict")
}

func (s *SQLStore) Delete(ctx context.Context, userID, id int64, expectedVersion int32) error {
	if expectedVersion <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	res, err := s.conn.ExecCtx(ctx, `DELETE FROM watch_task WHERE id = ? AND user_id = ? AND version = ?`, id, userID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	if _, err := s.getTask(ctx, userID, id); err != nil {
		if err == sqlx.ErrNotFound {
			return errx.NewWithCode(errx.NotFound)
		}
		return err
	}
	return errx.New(errx.ContentVersionConflict, "watch version conflict")
}

func (s *SQLStore) ListHits(ctx context.Context, userID int64, unreadOnly bool) ([]Hit, error) {
	query := `SELECT id, user_id, task_id, post_id, title, summary, created_at_ms, IF(read_at_ms IS NULL, 0, 1) AS read_flag
		FROM watch_hit WHERE user_id = ?`
	if unreadOnly {
		query += ` AND read_at_ms IS NULL`
	}
	query += ` ORDER BY id DESC LIMIT 50`
	var rows []struct {
		ID          int64  `db:"id"`
		UserID      int64  `db:"user_id"`
		TaskID      int64  `db:"task_id"`
		PostID      int64  `db:"post_id"`
		Title       string `db:"title"`
		Summary     string `db:"summary"`
		CreatedAtMs int64  `db:"created_at_ms"`
		ReadFlag    int64  `db:"read_flag"`
	}
	if err := s.conn.QueryRowsCtx(ctx, &rows, query, userID); err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, Hit{ID: row.ID, UserID: row.UserID, TaskID: row.TaskID, PostID: row.PostID,
			Title: row.Title, Summary: row.Summary, CreatedAt: row.CreatedAtMs, Read: row.ReadFlag == 1})
	}
	return hits, nil
}

func (s *SQLStore) GetHitsByIDs(ctx context.Context, userID int64, ids []int64) ([]Hit, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := `SELECT id, user_id, task_id, post_id, title, summary, created_at_ms, IF(read_at_ms IS NULL, 0, 1) AS read_flag
		FROM watch_hit WHERE user_id = ? AND id IN (` + placeholders(len(ids)) + `)`
	var rows []struct {
		ID          int64  `db:"id"`
		UserID      int64  `db:"user_id"`
		TaskID      int64  `db:"task_id"`
		PostID      int64  `db:"post_id"`
		Title       string `db:"title"`
		Summary     string `db:"summary"`
		CreatedAtMs int64  `db:"created_at_ms"`
		ReadFlag    int64  `db:"read_flag"`
	}
	args := append([]any{userID}, intsToAny(ids)...)
	if err := s.conn.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, Hit{ID: row.ID, UserID: row.UserID, TaskID: row.TaskID, PostID: row.PostID,
			Title: row.Title, Summary: row.Summary, CreatedAt: row.CreatedAtMs, Read: row.ReadFlag == 1})
	}
	return hits, nil
}

func (s *SQLStore) MarkRead(ctx context.Context, userID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.conn.ExecCtx(ctx, `UPDATE watch_hit SET read_at_ms = ? WHERE user_id = ? AND id IN (`+placeholders(len(ids))+`)`,
		append([]any{time.Now().UnixMilli(), userID}, intsToAny(ids)...)...)
	return err
}

func (s *SQLStore) RecordExecution(ctx context.Context, taskID int64, eventKey, status string, usedLLM bool) error {
	_, err := s.conn.ExecCtx(ctx, `
		INSERT IGNORE INTO watch_execution (task_id, event_key, hit, used_llm, status)
		VALUES (?, ?, 0, ?, ?)`, taskID, eventKey, boolToInt(usedLLM), status)
	return err
}

func (s *SQLStore) CountExecutions(ctx context.Context, taskID int64, eventKeyPrefix string) (int64, error) {
	var total int64
	err := s.conn.QueryRowCtx(ctx, &total,
		`SELECT COUNT(*) FROM watch_execution WHERE task_id = ? AND event_key LIKE ?`,
		taskID, eventKeyPrefix+"%")
	return total, err
}

func (s *SQLStore) RecordHit(ctx context.Context, hit Hit, eventKey string) error {
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		res, err := session.ExecCtx(ctx, `
			INSERT IGNORE INTO watch_execution (task_id, event_key, hit, used_llm, status)
			VALUES (?, ?, 1, 0, 'matched')`, hit.TaskID, eventKey)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		now := time.Now().UnixMilli()
		resHit, err := session.ExecCtx(ctx, `
			INSERT INTO watch_hit (user_id, task_id, post_id, title, summary, created_at_ms)
			VALUES (?, ?, ?, ?, ?, ?)`,
			hit.UserID, hit.TaskID, hit.PostID, hit.Title, hit.Summary, now)
		if err != nil {
			return err
		}
		hitID, _ := resHit.LastInsertId()
		window := now / 120000 * 120000
		payload, _ := json.Marshal([]int64{hitID})
		_, err = session.ExecCtx(ctx, `INSERT INTO watch_delivery_bucket (user_id, window_start_ms, status, hit_ids, run_id, created_at_ms)
			VALUES (?, ?, 'pending', ?, 0, ?)
			ON DUPLICATE KEY UPDATE hit_ids = JSON_ARRAY_APPEND(IFNULL(hit_ids, JSON_ARRAY()), '$', ?)`,
			hit.UserID, window, string(payload), now, hitID)
		return err
	})
}

type execRow struct {
	TaskID   int64
	EventKey string
	Hit      bool
	UsedLLM  bool
	Status   string
}

type MapStore struct {
	mu    sync.Mutex
	next  int64
	tasks map[int64]Task
	hits  map[int64]Hit
	keys  map[string]struct{}
	execs []execRow
}

func NewMapStore() *MapStore {
	return &MapStore{next: 1, tasks: map[int64]Task{}, hits: map[int64]Hit{}, keys: map[string]struct{}{}}
}

func (m *MapStore) ListTasks(_ context.Context, userID int64) ([]Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Task{}
	for _, task := range m.tasks {
		if task.UserID == userID {
			out = append(out, task)
		}
	}
	return out, nil
}

func (m *MapStore) GetTask(_ context.Context, userID, id int64) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok || task.UserID != userID {
		return Task{}, sqlx.ErrNotFound
	}
	return task, nil
}

func (m *MapStore) ListEnabled(_ context.Context) ([]Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Task{}
	for _, task := range m.tasks {
		if task.Enabled {
			out = append(out, task)
		}
	}
	return out, nil
}

func (m *MapStore) Create(_ context.Context, task Task) (Task, error) {
	if err := ValidateTask(task); err != nil {
		return Task{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.tasks {
		if existing.UserID == task.UserID && existing.ConditionType == task.ConditionType &&
			existing.TargetType == task.TargetType && existing.TargetID == task.TargetID &&
			existing.TargetText == task.TargetText {
			return Task{}, errx.New(errx.IdempotencyConflict, "duplicate watch task")
		}
	}
	id := m.next
	m.next++
	task.ID = id
	task.Enabled = true
	if task.Version == 0 {
		task.Version = 1
	}
	task.CreatedAt = time.Now().UnixMilli()
	m.tasks[id] = task
	return task, nil
}

func (m *MapStore) UpdateEnabled(_ context.Context, userID, id int64, enabled bool, expectedVersion int32) (Task, error) {
	if expectedVersion <= 0 {
		return Task{}, errx.NewWithCode(errx.ParamError)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok || task.UserID != userID {
		return Task{}, errx.NewWithCode(errx.NotFound)
	}
	if task.Version != expectedVersion {
		return Task{}, errx.New(errx.ContentVersionConflict, "watch version conflict")
	}
	task.Enabled = enabled
	task.Version++
	m.tasks[id] = task
	return task, nil
}

func (m *MapStore) Delete(_ context.Context, userID, id int64, expectedVersion int32) error {
	if expectedVersion <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok || task.UserID != userID {
		return errx.NewWithCode(errx.NotFound)
	}
	if task.Version != expectedVersion {
		return errx.New(errx.ContentVersionConflict, "watch version conflict")
	}
	delete(m.tasks, id)
	return nil
}

func (m *MapStore) ListHits(_ context.Context, userID int64, unreadOnly bool) ([]Hit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Hit{}
	for _, hit := range m.hits {
		if hit.UserID != userID || (unreadOnly && hit.Read) {
			continue
		}
		out = append(out, hit)
	}
	return out, nil
}

func (m *MapStore) GetHitsByIDs(_ context.Context, userID int64, ids []int64) ([]Hit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Hit, 0, len(ids))
	for _, id := range ids {
		hit, ok := m.hits[id]
		if ok && hit.UserID == userID {
			out = append(out, hit)
		}
	}
	return out, nil
}

func (m *MapStore) MarkRead(_ context.Context, userID int64, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int64]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	for id, hit := range m.hits {
		if hit.UserID == userID {
			if len(want) == 0 {
				hit.Read = true
				m.hits[id] = hit
				continue
			}
			if _, ok := want[id]; ok {
				hit.Read = true
				m.hits[id] = hit
			}
		}
	}
	return nil
}

func executionKey(taskID int64, eventKey string) string {
	return strconv.FormatInt(taskID, 10) + "\x00" + eventKey
}

func (m *MapStore) RecordHit(_ context.Context, hit Hit, eventKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.recordExecLocked(hit.TaskID, eventKey, "matched", true, false) {
		return nil
	}
	id := m.next
	m.next++
	hit.ID = id
	hit.CreatedAt = time.Now().UnixMilli()
	m.hits[id] = hit
	return nil
}

func (m *MapStore) RecordExecution(_ context.Context, taskID int64, eventKey, status string, usedLLM bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordExecLocked(taskID, eventKey, status, false, usedLLM)
	return nil
}

func (m *MapStore) CountExecutions(_ context.Context, taskID int64, eventKeyPrefix string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, row := range m.execs {
		if row.TaskID == taskID && strings.HasPrefix(row.EventKey, eventKeyPrefix) {
			total++
		}
	}
	return total, nil
}

func (m *MapStore) recordExecLocked(taskID int64, eventKey, status string, hit, usedLLM bool) bool {
	key := executionKey(taskID, eventKey)
	if _, ok := m.keys[key]; ok {
		return false
	}
	m.keys[key] = struct{}{}
	m.execs = append(m.execs, execRow{TaskID: taskID, EventKey: eventKey, Hit: hit, UsedLLM: usedLLM, Status: status})
	return true
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.Repeat("?,", n-1) + "?"
}

func intsToAny(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
