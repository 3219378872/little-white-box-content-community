package logic

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	pb "esx/kitex_gen/media"
	"esx/pkg/event"
	"esx/pkg/outboxx"
	"esx/pkg/sqlstore"

	"github.com/stretchr/testify/require"
)

// These tests run the production SQL transaction, idempotency, media command,
// media read and outbox adapters. Only the database wire and object store are
// controlled; durable writes and lost COMMIT ACKs are independent events.
type commitSafetyDB struct {
	row                        []driver.Value
	binding                    []driver.Value
	outbox                     [][]byte
	commits                    int
	lostACKAt                  int
	rejectInsert               bool
	rejectCommit               bool
	cancel                     context.CancelFunc
	reconciliation             string
	reconciliationCalls        int
	reconciliationDeadline     time.Duration
	reconciliationContextError error
	reconciliationTrace        any
}
type commitSafetyDriver struct{ state *commitSafetyDB }

func (d commitSafetyDriver) Open(string) (driver.Conn, error) {
	return &commitSafetyConn{state: d.state}, nil
}
func (d commitSafetyDriver) Connect(context.Context) (driver.Conn, error) { return d.Open("") }
func (d commitSafetyDriver) Driver() driver.Driver                        { return d }

type commitSafetyConn struct {
	state   *commitSafetyDB
	row     []driver.Value
	binding []driver.Value
	outbox  [][]byte
}

func (c *commitSafetyConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *commitSafetyConn) Close() error { return nil }
func (c *commitSafetyConn) Begin() (driver.Tx, error) {
	c.row = nil
	c.binding = nil
	c.outbox = nil
	return c, nil
}
func (c *commitSafetyConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}
func (c *commitSafetyConn) Commit() error {
	c.state.commits++
	if c.state.rejectCommit {
		return errors.New("commit failed without a known outcome")
	}
	if c.row != nil {
		c.state.row = c.row
	}
	if c.binding != nil {
		c.state.binding = c.binding
	}
	c.state.outbox = append(c.state.outbox, c.outbox...)
	if c.state.commits == c.state.lostACKAt {
		if c.state.cancel != nil {
			c.state.cancel()
		}
		return errors.New("COMMIT durable, acknowledgement lost")
	}
	return nil
}
func (c *commitSafetyConn) Rollback() error { c.row = nil; c.binding = nil; c.outbox = nil; return nil }
func (c *commitSafetyConn) ExecContext(_ context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	switch {
	case strings.Contains(q, "INSERT INTO `idempotency`"):
		c.binding = []driver.Value{a[5].Value, a[4].Value, a[1].Value, a[2].Value, a[3].Value}
	case strings.Contains(q, "INSERT INTO media"):
		if c.state.rejectInsert {
			return nil, errors.New("definite INSERT failure")
		}
		for _, v := range a {
			c.row = append(c.row, v.Value)
		}
		c.row = append(c.row, time.Now(), time.Now())
	case strings.Contains(q, "INSERT INTO event_outbox"):
		c.outbox = append(c.outbox, append([]byte(nil), a[4].Value.([]byte)...))
	default:
		return nil, fmt.Errorf("unexpected exec: %s", q)
	}
	return driver.RowsAffected(1), nil
}

type commitSafetyTraceKey struct{}

func (c *commitSafetyConn) QueryContext(ctx context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(q, "SELECT m.id, m.user_id"):
		c.state.reconciliationCalls++
		c.state.reconciliationContextError = ctx.Err()
		c.state.reconciliationTrace = ctx.Value(commitSafetyTraceKey{})
		if deadline, ok := ctx.Deadline(); ok {
			c.state.reconciliationDeadline = time.Until(deadline)
		}
		switch c.state.reconciliation {
		case "error":
			return nil, errors.New("primary unavailable")
		case "timeout":
			<-ctx.Done()
			return nil, ctx.Err()
		}
		rows := &commitSafetyRows{columns: []string{"id", "user_id", "object_key", "thumbnail_object_key", "status"}}
		if c.state.reconciliation == "missing" || len(c.state.row) == 0 {
			return rows, nil
		}
		if a[0].Value != c.state.row[0] {
			return rows, nil
		}
		if len(a) > 1 {
			b := c.state.binding
			if !strings.Contains(q, "JOIN `idempotency`") || !strings.Contains(q, "i.command_hash = ?") {
				return nil, errors.New("reconciliation did not check binding")
			}
			if len(b) == 0 || b[0] != a[0].Value || b[2] != a[1].Value || b[3] != a[2].Value || b[4] != a[3].Value || b[1] != a[4].Value {
				return rows, nil
			}
			if c.state.reconciliation == "binding_mismatch" {
				return rows, nil
			}
		}
		r := c.state.row
		rows.values = []driver.Value{r[0], r[1], r[10], r[11], r[18]}
		switch c.state.reconciliation {
		case "wrong_owner":
			rows.values[1] = int64(999)
		case "wrong_key":
			rows.values[2] = "unrelated-object"
		case "wrong_thumbnail":
			rows.values[3] = "unrelated-thumbnail"
		case "deleted":
			rows.values[4] = int64(0)
		}
		return rows, nil
	case strings.Contains(q, "FROM `idempotency`"):
		rows := &commitSafetyRows{columns: []string{"resource_id", "command_hash"}}
		if b := c.state.binding; len(b) > 0 && b[2] == a[0].Value && b[3] == a[1].Value && b[4] == a[2].Value {
			rows.values = b[:2]
		}
		return rows, nil
	case strings.Contains(q, "from `media`"):
		rows := &commitSafetyRows{columns: strings.Split("id,user_id,file_name,original_name,file_type,mime_type,url,thumbnail_url,storage_type,bucket,object_key,thumbnail_object_key,file_size,width,height,duration,format,bit_rate,status,created_at,updated_at", ",")}
		if len(c.state.row) > 0 && c.state.row[0] == a[0].Value {
			rows.values = c.state.row
		}
		return rows, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", q)
	}
}

type commitSafetyRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *commitSafetyRows) Columns() []string { return r.columns }
func (r *commitSafetyRows) Close() error      { return nil }
func (r *commitSafetyRows) Next(dst []driver.Value) error {
	if r.done || r.values == nil {
		return io.EOF
	}
	r.done = true
	copy(dst, r.values)
	return nil
}

type commitSafetyStorage struct {
	unitObjectStorage
	objects             map[string][]byte
	deleteContextErrors []error
}

func (s *commitSafetyStorage) Put(ctx context.Context, key string, r io.Reader, size int64, kind string) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if err = s.unitObjectStorage.Put(ctx, key, bytes.NewReader(body), size, kind); err != nil {
		return err
	}
	s.objects[key] = body
	return nil
}
func (s *commitSafetyStorage) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		s.deleteContextErrors = append(s.deleteContextErrors, err)
		return err
	}
	if err := s.unitObjectStorage.Delete(ctx, key); err != nil {
		return err
	}
	delete(s.objects, key)
	return nil
}
func newCommitSafetyService(t *testing.T, state *commitSafetyDB) (*svc.ServiceContext, *commitSafetyStorage) {
	t.Helper()
	unitInitSnowflake(t)
	db := sql.OpenDB(commitSafetyDriver{state: state})
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	conn := sqlstore.NewSqlConnFromDB(db)
	storage := &commitSafetyStorage{objects: map[string][]byte{}}
	cfg := unitUploadConfig()
	cfg.Upload.TempDir = t.TempDir()
	sc := unitSvcCtx(cfg, model.NewMediaModel(conn, nil), model.NewMediaCommandModel(conn, outboxx.NewSQLStore(conn)), &storage.unitObjectStorage)
	sc.Storage = storage
	return sc, storage
}
func runCommitSafetyUpload(t *testing.T, ctx context.Context, sc *svc.ServiceContext, kind, key string) (*pb.MediaInfo, error) {
	t.Helper()
	switch kind {
	case "image":
		s := unitImageStreamFromBytes(ctx, 77, "a.jpg", key, unitTestJPEG(t, 32, 24), 256)
		err := NewUploadImageLogic(ctx, sc).UploadImage(s)
		if s.resp != nil {
			return s.resp.Media, err
		}
		return nil, err
	case "video":
		s := unitVideoStreamFromBytes(ctx, 77, "a.mp4", key, unitTestMP4(), 64)
		err := NewUploadVideoLogic(ctx, sc).UploadVideo(s)
		if s.resp != nil {
			return s.resp.Media, err
		}
		return nil, err
	default:
		s := unitAudioStreamFromBytes(ctx, 77, "a.wav", key, unitTestWAV(), 64)
		err := NewUploadAudioLogic(ctx, sc).UploadAudio(s)
		if s.resp != nil {
			return s.resp.Media, err
		}
		return nil, err
	}
}
func requireCommittedObjects(t *testing.T, state *commitSafetyDB, storage *commitSafetyStorage) {
	t.Helper()
	require.NotEmpty(t, state.row)
	require.Equal(t, int64(1), state.row[18])
	for _, i := range []int{10, 11} {
		if key, ok := state.row[i].(string); ok && key != "" {
			require.NotEmpty(t, storage.objects[key])
			require.NotContains(t, storage.deleteKeys, key)
		}
	}
	require.Empty(t, storage.deleteContextErrors)
}
func TestUploadAmbiguousCommitRetainsReferencedMedia(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio"} {
		for _, fault := range []string{"reconciled", "canceled_caller", "error", "missing", "timeout", "wrong_owner", "wrong_key", "wrong_thumbnail", "binding_mismatch", "deleted"} {
			t.Run(kind+"/"+fault, func(t *testing.T) {
				state := &commitSafetyDB{lostACKAt: 1, reconciliation: fault}
				sc, storage := newCommitSafetyService(t, state)
				ctx := context.WithValue(context.Background(), commitSafetyTraceKey{}, "request-trace")
				if fault == "canceled_caller" {
					ctx, state.cancel = context.WithCancel(ctx)
					defer state.cancel()
				}
				response, err := runCommitSafetyUpload(t, ctx, sc, kind, "same-key")
				requireCommittedObjects(t, state, storage)
				require.Empty(t, storage.deleteKeys)
				require.Empty(t, state.outbox)
				if fault == "reconciled" || fault == "canceled_caller" {
					require.NoError(t, err)
					require.Equal(t, state.row[0], response.Id)
				} else {
					require.Error(t, err)
					require.Nil(t, response)
				}
				require.Equal(t, 1, state.reconciliationCalls)
				require.NoError(t, state.reconciliationContextError)
				require.Equal(t, "request-trace", state.reconciliationTrace)
				require.Greater(t, state.reconciliationDeadline, time.Duration(0))
				require.LessOrEqual(t, state.reconciliationDeadline, 2*time.Second)
				// A fresh request after recovery reads the authoritative SQL row. Only the
				// second attempt's unique objects may be discarded, including both variants.
				state.reconciliation = ""
				response, err = runCommitSafetyUpload(t, context.Background(), sc, kind, "same-key")
				require.NoError(t, err)
				require.Equal(t, state.row[0], response.Id)
				require.Equal(t, state.row[6], response.Url)
				requireCommittedObjects(t, state, storage)
				wantObjects := 1
				if kind == "image" {
					wantObjects = 2
				}
				require.Len(t, storage.objects, wantObjects)
				require.Len(t, storage.deleteKeys, wantObjects)
			})
		}
	}
}
func TestUploadDefiniteRollbackCleansObjects(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio"} {
		for _, deleteFailure := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/delete_failure_%t", kind, deleteFailure), func(t *testing.T) {
				state := &commitSafetyDB{rejectInsert: true}
				sc, storage := newCommitSafetyService(t, state)
				storage.failDeletes = deleteFailure
				_, err := runCommitSafetyUpload(t, context.Background(), sc, kind, "rollback-key")
				require.Error(t, err)
				require.Nil(t, state.row)
				require.Nil(t, state.binding)
				require.Zero(t, state.reconciliationCalls)
				n := 1
				if kind == "image" {
					n = 2
				}
				if !deleteFailure {
					require.Empty(t, storage.objects)
					require.Len(t, storage.deleteKeys, n)
					require.Empty(t, state.outbox)
					return
				}
				require.Len(t, state.outbox, n)
				for _, payload := range state.outbox {
					var deletion event.MediaDeletedEvent
					require.NoError(t, json.Unmarshal(payload, &deletion))
					require.Equal(t, "upload_compensation", deletion.Reason)
					require.Contains(t, storage.objects, deletion.S3ObjectKey)
				}
			})
		}
	}
}
func TestUploadUnprovenRollbackRetainsObjects(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			state := &commitSafetyDB{rejectCommit: true}
			sc, storage := newCommitSafetyService(t, state)
			_, err := runCommitSafetyUpload(t, context.Background(), sc, kind, "unknown-key")
			require.Error(t, err)
			require.Nil(t, state.row)
			require.Nil(t, state.binding)
			require.NotEmpty(t, storage.objects)
			require.Empty(t, storage.deleteKeys)
			require.Empty(t, state.outbox)
			require.Equal(t, 1, state.reconciliationCalls)
		})
	}
}
func TestUploadReconcilesCommitWithoutIdempotencyKey(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			state := &commitSafetyDB{lostACKAt: 1}
			sc, storage := newCommitSafetyService(t, state)
			response, err := runCommitSafetyUpload(t, context.Background(), sc, kind, "")
			require.NoError(t, err)
			require.Equal(t, state.row[0], response.Id)
			require.Nil(t, state.binding)
			requireCommittedObjects(t, state, storage)
		})
	}
}
func TestUploadDuplicateCommitACKLossCleansOnlyNewObjects(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio"} {
		for _, deleteFailure := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/delete_failure_%t", kind, deleteFailure), func(t *testing.T) {
				state := &commitSafetyDB{lostACKAt: 2}
				sc, storage := newCommitSafetyService(t, state)
				first, err := runCommitSafetyUpload(t, context.Background(), sc, kind, "duplicate-key")
				require.NoError(t, err)
				storage.failDeletes = deleteFailure
				second, err := runCommitSafetyUpload(t, context.Background(), sc, kind, "duplicate-key")
				require.NoError(t, err)
				require.Equal(t, first.Id, second.Id)
				require.Equal(t, 1, state.reconciliationCalls)
				requireCommittedObjects(t, state, storage)
				n := 1
				if kind == "image" {
					n = 2
				}
				if !deleteFailure {
					require.Len(t, storage.objects, n)
					require.Len(t, storage.deleteKeys, n)
					require.Empty(t, state.outbox)
					return
				}
				require.Len(t, state.outbox, n)
				for _, payload := range state.outbox {
					var deletion event.MediaDeletedEvent
					require.NoError(t, json.Unmarshal(payload, &deletion))
					require.NotEqual(t, state.row[10], deletion.S3ObjectKey)
					require.NotEqual(t, state.row[11], deletion.S3ObjectKey)
					require.Contains(t, storage.objects, deletion.S3ObjectKey)
				}
			})
		}
	}
}
