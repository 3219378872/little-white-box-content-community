package logic

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	pb "esx/kitex_gen/media"
	"esx/pkg/cachedstore"
	"esx/pkg/errx"
	"esx/pkg/modelcache"
	"esx/pkg/redisstore"
	"esx/pkg/sqlstore"
	"esx/pkg/testutil/redisfixture"

	"github.com/stretchr/testify/require"
)

// A database/sql driver supplies controlled authoritative snapshots. The real
// sqlstore, MediaModel and GetMedia logic execute; no SQL connection is mocked.
type authoritativeMediaDB struct {
	mu              sync.Mutex
	status          int64
	err             error
	calls           int
	loaded, release chan struct{}
}
type authoritativeMediaConnector struct{ state *authoritativeMediaDB }
type authoritativeMediaConn struct{ state *authoritativeMediaDB }
type authoritativeMediaRows struct{ values []driver.Value }

func (c authoritativeMediaConnector) Connect(context.Context) (driver.Conn, error) {
	return &authoritativeMediaConn{c.state}, nil
}
func (c authoritativeMediaConnector) Driver() driver.Driver { return c }
func (c authoritativeMediaConnector) Open(string) (driver.Conn, error) {
	return c.Connect(context.Background())
}
func (c *authoritativeMediaConn) Close() error { return nil }
func (c *authoritativeMediaConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *authoritativeMediaConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c *authoritativeMediaConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "from `media` where `id` = ?") || len(args) != 1 {
		return nil, fmt.Errorf("unexpected media query: %s", query)
	}
	c.state.mu.Lock()
	c.state.calls++
	status, err := c.state.status, c.state.err
	loaded, release := c.state.loaded, c.state.release
	c.state.loaded, c.state.release = nil, nil
	c.state.mu.Unlock()
	if loaded != nil {
		close(loaded)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return &authoritativeMediaRows{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &authoritativeMediaRows{values: []driver.Value{args[0].Value, int64(4), "image.jpg", "image", "https://invalid.example/image.jpg", status, time.Unix(100, 0)}}, nil
}
func (r *authoritativeMediaRows) Columns() []string {
	return []string{"id", "user_id", "file_name", "file_type", "url", "status", "created_at"}
}
func (r *authoritativeMediaRows) Close() error { return nil }
func (r *authoritativeMediaRows) Next(out []driver.Value) error {
	if r.values == nil {
		return io.EOF
	}
	copy(out, r.values)
	r.values = nil
	return nil
}

func authoritativeMediaFixture(t *testing.T, state *authoritativeMediaDB) (model.MediaModel, *redisfixture.Server) {
	t.Helper()
	s := redisfixture.New(t)
	db := sql.OpenDB(authoritativeMediaConnector{state})
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return model.NewMediaModel(sqlstore.NewSqlConnFromDB(db), modelcache.CacheConf{{RedisConf: redisstore.RedisConf{Host: s.Addr()}}}), s
}

func TestGetMediaRejectsStaleReadyCacheWhenInvalidationFails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int64
		dbErr  error
		code   int
	}{
		{name: "deleted", status: 0, code: errx.MediaNotFound},
		{name: "processing", status: 2, code: errx.MediaNotFound},
		{name: "missing", dbErr: sql.ErrNoRows, code: errx.MediaNotFound},
		{name: "database unavailable", dbErr: errors.New("SQL unavailable"), code: errx.SystemError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &authoritativeMediaDB{status: tc.status, err: tc.dbErr}
			m, s := authoritativeMediaFixture(t, state)
			s.Set(cachedstore.Prefix+"cache:media:id:7", `{"Missing":false,"Value":{"Id":7,"UserId":4,"Status":1}}`, 604800)
			s.Fail("DEL", false)
			require.Error(t, m.DelCache(context.Background(), 7))
			logic := NewGetMediaLogic(context.Background(), &svc.ServiceContext{MediaModel: m})
			resp, err := logic.GetMedia(&pb.GetMediaReq{MediaId: 7})
			require.Nil(t, resp)
			require.Equal(t, tc.code, errx.GetCode(err))
			require.Equal(t, 1, state.calls)
			require.Zero(t, s.Calls("GET"), "authoritative reads must not consult Redis")
		})
	}
}

func TestGetMediaAfterInFlightReadAndFailedInvalidation(t *testing.T) {
	loaded, release := make(chan struct{}), make(chan struct{})
	state := &authoritativeMediaDB{status: 1, loaded: loaded, release: release}
	m, s := authoritativeMediaFixture(t, state)
	ctx := context.Background()
	done := make(chan error, 1)
	go func() { _, err := m.FindOne(ctx, 7); done <- err }()
	select {
	case <-loaded:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("authoritative query did not start")
	}
	state.mu.Lock()
	state.status = 0
	state.mu.Unlock()
	s.Fail("DEL", false)
	err := m.DelCache(ctx, 7)
	close(release)
	require.Error(t, err)
	require.NoError(t, <-done)
	logic := NewGetMediaLogic(ctx, &svc.ServiceContext{MediaModel: m})
	resp, err := logic.GetMedia(&pb.GetMediaReq{MediaId: 7})
	require.Nil(t, resp)
	require.Equal(t, errx.MediaNotFound, errx.GetCode(err))
	state.mu.Lock()
	calls := state.calls
	state.mu.Unlock()
	require.Equal(t, 2, calls)
	require.Zero(t, s.Calls("GET"))
}
