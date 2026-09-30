//go:build integration

package poststats

import (
	"context"
	"encoding/json"
	"esx/pkg/event"
	"esx/pkg/outboxx"
	"esx/pkg/sqlstore"
	"esx/pkg/testutil"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMySQLSharedSequenceAndSnapshotsCommitWithOutbox(t *testing.T) {
	env := testutil.SetupTestEnv(t, "xbh_content", testutil.SchemaPath("xbh_content.sql"))
	defer env.Close()
	ctx := context.Background()
	_, err := env.DB.ExecContext(ctx, "INSERT INTO post (id, author_id, title, content, status, revision, stats_seq) VALUES (1,1,'t','c',1,1,9000000000000)")
	require.NoError(t, err)
	conn := sqlstore.NewSqlConnFromDB(env.DB)
	outbox := outboxx.NewSQLStore(conn)
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errors <- conn.TransactCtx(ctx, func(ctx context.Context, s sqlstore.Session) error {
				column := "like_count"
				if i%2 == 1 {
					column = "comment_count"
				}
				if _, err := s.ExecCtx(ctx, "UPDATE post SET "+column+" = "+column+" + 1 WHERE id = ?", 1); err != nil {
					return err
				}
				counts, err := Advance(ctx, s, 1)
				if err != nil {
					return err
				}
				payload, err := json.Marshal(event.PostEvent{EventID: int64(i + 1), EventTime: 100, PostID: 1, Type: event.PostEventCounted, LikeCount: counts.LikeCount, CommentCount: counts.CommentCount, StatsSeq: counts.Sequence})
				if err != nil {
					return err
				}
				return outbox.Enqueue(ctx, s, outboxx.Event{ID: int64(i + 1), Topic: "post-update", Key: "1", Payload: payload})
			})
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	var likes, comments, seq, count int64
	require.NoError(t, env.DB.QueryRow("SELECT like_count,comment_count,stats_seq FROM post WHERE id=1").Scan(&likes, &comments, &seq))
	require.Equal(t, int64(10), likes)
	require.Equal(t, int64(10), comments)
	require.Equal(t, int64(9000000000020), seq)
	require.NoError(t, env.DB.QueryRow("SELECT COUNT(*) FROM event_outbox").Scan(&count))
	require.Equal(t, int64(20), count)
	_, err = env.DB.ExecContext(ctx, "INSERT INTO post (id,author_id,title,content,status,revision) VALUES (2,1,'unseeded','c',1,1)")
	require.NoError(t, err)
	err = conn.TransactCtx(ctx, func(ctx context.Context, s sqlstore.Session) error {
		if _, err := s.ExecCtx(ctx, "UPDATE post SET like_count=1 WHERE id=2"); err != nil {
			return err
		}
		_, err := Advance(ctx, s, 2)
		return err
	})
	require.ErrorContains(t, err, "watermark migration")
	require.NoError(t, env.DB.QueryRow("SELECT like_count FROM post WHERE id=2").Scan(&likes))
	require.Zero(t, likes, "unseeded stats must roll back the count mutation")

}
