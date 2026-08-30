//go:build integration

package model

import (
	"context"
	"os"
	"testing"
	"time"

	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"
	"esx/pkg/testutil"
	"esx/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var testEnv *testutil.TestEnv

func TestMain(m *testing.M) {
	testEnv = testutil.SetupTestEnvM("xbh_content", testutil.SchemaPath("xbh_content.sql"))
	_ = util.InitSnowflake(1, 1)
	code := m.Run()
	testEnv.Close()
	os.Exit(code)
}

func newTestConn() sqlx.SqlConn {
	return sqlx.NewSqlConnFromDB(testEnv.DB)
}

func testCacheConf() cache.CacheConf {
	return cache.CacheConf{
		cache.NodeConf{
			RedisConf: redis.RedisConf{Host: testEnv.RedisAddr, Type: "node"},
			Weight:    100,
		},
	}
}

func nextID(t *testing.T) int64 {
	t.Helper()
	id, err := util.NextID()
	require.NoError(t, err)
	return id
}

func outboxEvent(t *testing.T, topic string) outboxx.Event {
	t.Helper()
	payload := []byte(`{"probe":true}`)
	return outboxx.Event{
		ID:      nextID(t),
		Topic:   topic,
		Tag:     "test",
		Key:     "integration-test",
		Payload: payload,
	}
}

func countOutboxRows(t *testing.T) int64 {
	t.Helper()
	var n int64
	err := newTestConn().QueryRowCtx(context.Background(), &n,
		"SELECT COUNT(*) FROM `event_outbox`")
	require.NoError(t, err)
	return n
}

func seedPost(t *testing.T, id, authorID, status, revision int64, title string) {
	t.Helper()
	_, err := newTestConn().ExecCtx(context.Background(),
		"INSERT INTO `post` (`id`, `author_id`, `title`, `content`, `status`, `revision`) VALUES (?, ?, ?, ?, ?, ?)",
		id, authorID, title, "body "+title, status, revision)
	require.NoError(t, err)
}

func TestPostModelRoundtripAndQueries(t *testing.T) {
	testEnv.TruncateAll(t, "post_tag", "post")
	ctx := context.Background()

	postModel := NewPostModel(newTestConn(), testCacheConf())
	first := &Post{Id: nextID(t), AuthorId: 7, Title: "alpha", Content: "a-body", Status: 1, Revision: 1}
	require.NoError(t, postModel.InsertPost(ctx, first))
	second := &Post{Id: nextID(t), AuthorId: 7, Title: "beta", Content: "b-body", Status: 1, Revision: 3}
	require.NoError(t, postModel.InsertPost(ctx, second))
	draft := &Post{Id: nextID(t), AuthorId: 8, Title: "gamma-draft", Content: "g", Status: 0, Revision: 1}
	require.NoError(t, postModel.InsertPost(ctx, draft))

	// 查询 + 缓存命中二次读取。
	got, err := postModel.FindPostById(ctx, first.Id)
	require.NoError(t, err)
	assert.Equal(t, "alpha", got.Title)
	gotAgain, err := postModel.FindPostById(ctx, first.Id)
	require.NoError(t, err)
	assert.Equal(t, got.Title, gotAgain.Title)

	_, err = postModel.FindPostById(ctx, -1)
	require.ErrorIs(t, err, ErrNotFound)

	// 用户帖子 keyset 查询只包含已发布帖子（草稿被过滤）。
	posts, hasMore, err := postModel.FindUserPostsByCursor(ctx, 7, nil, 10)
	require.NoError(t, err)
	require.Len(t, posts, 2)
	assert.False(t, hasMore)

	// pageSize 截断时 hasMore=true，恰返回一页。
	paged, pagedHasMore, err := postModel.FindUserPostsByCursor(ctx, 7, nil, 1)
	require.NoError(t, err)
	require.Len(t, paged, 1)
	assert.True(t, pagedHasMore)

	// 全局列表查询同样只包含已发布帖子（草稿被过滤）。
	list, listHasMore, err := postModel.FindListByCursor(ctx, nil, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.False(t, listHasMore)

	byIds, err := postModel.FindByIds(ctx, []int64{first.Id, second.Id, draft.Id, -42})
	require.NoError(t, err)
	assert.Len(t, byIds, 3)

	// 字段更新与状态流转：命令写入绕过查询缓存，用裸 SQL 验证真实状态。
	require.NoError(t, postModel.UpdateFields(ctx, first.Id, map[string]interface{}{"title": "alpha2"}))
	var rawTitle string
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &rawTitle,
		"SELECT title FROM `post` WHERE `id` = ?", first.Id))
	assert.Equal(t, "alpha2", rawTitle)

	require.NoError(t, postModel.UpdateStatus(ctx, draft.Id, 1))
	var rawDraftStatus int64
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &rawDraftStatus,
		"SELECT status FROM `post` WHERE `id` = ?", draft.Id))
	assert.Equal(t, int64(1), rawDraftStatus)

	// 计数增减。
	require.NoError(t, postModel.IncrCommentCount(ctx, second.Id))
	require.NoError(t, postModel.IncrCommentCount(ctx, second.Id))
	require.NoError(t, postModel.DecrCommentCount(ctx, second.Id))
	var rawCommentCount int64
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &rawCommentCount,
		"SELECT comment_count FROM `post` WHERE `id` = ?", second.Id))
	assert.Equal(t, int64(1), rawCommentCount)
}

func TestPostCommandModelCreatePostIsIdempotentAndWritesOutbox(t *testing.T) {
	testEnv.TruncateAll(t, "post_tag", "post", "idempotency")
	testEnv.DB.ExecContext(context.Background(), "TRUNCATE TABLE `event_outbox`")
	ctx := context.Background()

	conn := newTestConn()
	outboxStore := outboxx.NewSQLStore(conn)
	commandModel := NewPostCommandModel(conn, outboxStore)

	tagID := nextID(t)
	idem := idempotencyx.IdempotencyRecord{
		Scope:       "post:create",
		UserID:      7,
		Key:         "idem-create-1",
		CommandHash: idempotencyx.CommandHash("alpha"),
	}
	post := &Post{Id: nextID(t), AuthorId: 7, Title: "alpha", Content: "a", Status: 1, Revision: 1}
	event := outboxEvent(t, "content-post-v1")

	postID, created, err := commandModel.CreatePost(ctx, post, []string{"golang"}, []int64{tagID}, event, idem)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, post.Id, postID)

	// 帖子、标签关联、outbox、幂等绑定全部落库（命令写入绕过缓存，用裸 SQL 验证）。
	var storedTitle string
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &storedTitle,
		"SELECT title FROM `post` WHERE `id` = ?", postID))
	assert.Equal(t, "alpha", storedTitle)

	postTagModel := NewPostTagModel(newTestConn(), testCacheConf())
	names, err := postTagModel.FindTagNamesByPostId(ctx, postID)
	require.NoError(t, err)
	assert.Equal(t, []string{"golang"}, names)

	assert.Positive(t, countOutboxRows(t))

	// 同键重放：不重复创建，返回既有 ID。
	retryPost := &Post{Id: nextID(t), AuthorId: 7, Title: "should-not-insert", Status: 1, Revision: 1}
	retryID, created, err := commandModel.CreatePost(ctx, retryPost,
		[]string{"golang"}, []int64{tagID},
		outboxEvent(t, "content-post-v1"),
		idempotencyx.IdempotencyRecord{
			Scope: idem.Scope, UserID: idem.UserID, Key: idem.Key,
			CommandHash: idempotencyx.CommandHash("alpha"),
		})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, post.Id, retryID)

	// 同键不同命令：幂等冲突。
	_, _, err = commandModel.CreatePost(ctx, retryPost,
		[]string{"golang"}, []int64{tagID},
		outboxEvent(t, "content-post-v1"),
		idempotencyx.IdempotencyRecord{
			Scope: idem.Scope, UserID: idem.UserID, Key: idem.Key,
			CommandHash: idempotencyx.CommandHash("different-command"),
		})
	require.ErrorIs(t, err, idempotencyx.ErrIdempotencyConflict)
}

func TestPostCommandModelUpdatePostRevisionGuard(t *testing.T) {
	testEnv.TruncateAll(t, "post_tag", "post", "idempotency")
	ctx := context.Background()

	conn := newTestConn()
	commandModel := NewPostCommandModel(conn, outboxx.NewSQLStore(conn))
	post := &Post{Id: nextID(t), AuthorId: 7, Title: "before", Content: "c", Status: 1, Revision: 5}
	seedPost(t, post.Id, post.AuthorId, 1, 5, "before")

	// 版本不匹配拒绝。
	err := commandModel.UpdatePost(ctx, post.Id,
		map[string]any{"title": "after"}, nil, nil,
		outboxEvent(t, "content-post-v1"), 4, true)
	require.ErrorIs(t, err, ErrVersionConflict)

	// 版本匹配：字段更新、revision 自增、标签整体替换。
	err = commandModel.UpdatePost(ctx, post.Id,
		map[string]any{"title": "after"},
		[]string{"rust"}, []int64{nextID(t)},
		outboxEvent(t, "content-post-v1"), 5, true)
	require.NoError(t, err)

	var updatedTitle string
	var updatedRevision int64
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &updatedTitle,
		"SELECT title FROM `post` WHERE `id` = ?", post.Id))
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &updatedRevision,
		"SELECT revision FROM `post` WHERE `id` = ?", post.Id))
	assert.Equal(t, "after", updatedTitle)
	assert.Equal(t, int64(6), updatedRevision)

	postTagModel := NewPostTagModel(conn, testCacheConf())
	names, err := postTagModel.FindTagNamesByPostId(ctx, post.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"rust"}, names)

	// 非法列名被白名单拦截（revision 保持不变）。
	err = commandModel.UpdatePost(ctx, post.Id,
		map[string]any{"author_id": 99}, nil, nil,
		outboxEvent(t, "content-post-v1"), updatedRevision, true)
	require.ErrorContains(t, err, "disallowed column")
}

func TestPostCommandModelDeletePostSoftDeletesOnce(t *testing.T) {
	testEnv.TruncateAll(t, "post_tag", "post", "idempotency")
	ctx := context.Background()

	conn := newTestConn()
	commandModel := NewPostCommandModel(conn, outboxx.NewSQLStore(conn))
	postID := nextID(t)
	seedPost(t, postID, 9, 1, 2, "doomed")

	before := countOutboxRows(t)
	// 版本不匹配拒绝删除。
	err := commandModel.DeletePost(ctx, postID, outboxEvent(t, "content-post-delete-v1"), 3)
	require.ErrorIs(t, err, ErrVersionConflict)

	require.NoError(t, commandModel.DeletePost(ctx, postID, outboxEvent(t, "content-post-delete-v1"), 2))
	assert.Greater(t, countOutboxRows(t), before)

	var (
		deletedStatus   int64
		deletedRevision int64
	)
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &deletedStatus,
		"SELECT status FROM `post` WHERE `id` = ?", postID))
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &deletedRevision,
		"SELECT revision FROM `post` WHERE `id` = ?", postID))
	assert.Equal(t, int64(2), deletedStatus)
	assert.Equal(t, int64(3), deletedRevision)

	// 已删除后重复删除：版本条件不再满足。
	err = commandModel.DeletePost(ctx, postID, outboxEvent(t, "content-post-delete-v1"), 3)
	require.ErrorIs(t, err, ErrVersionConflict)
}

func TestPostCommandModelUpdatePostIdempotencySurvivesLostResponse(t *testing.T) {
	testEnv.TruncateAll(t, "post_tag", "post", "idempotency")
	testEnv.DB.ExecContext(context.Background(), "TRUNCATE TABLE `event_outbox`")
	ctx := context.Background()
	conn := newTestConn()
	command := NewPostCommandModel(conn, outboxx.NewSQLStore(conn)).(IdempotentPostCommandModel)
	postID := nextID(t)
	seedPost(t, postID, 7, 1, 5, "before")
	idem := idempotencyx.IdempotencyRecord{
		Scope: "post:update", UserID: 7, Key: "agent-update-lost-response",
		CommandHash: idempotencyx.CommandHash("same-command"),
	}
	beforeOutbox := countOutboxRows(t)
	applied, err := command.UpdatePostIdempotent(ctx, postID, map[string]any{"title": "after"}, nil, nil,
		outboxEvent(t, "content-post-update-v1"), 5, false, 6, idem)
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = command.UpdatePostIdempotent(ctx, postID, map[string]any{"title": "after"}, nil, nil,
		outboxEvent(t, "content-post-update-v1"), 5, false, 6, idem)
	require.NoError(t, err)
	require.False(t, applied)
	var revision int64
	require.NoError(t, conn.QueryRowCtx(ctx, &revision, "SELECT revision FROM post WHERE id=?", postID))
	assert.Equal(t, int64(6), revision)
	assert.Equal(t, beforeOutbox+1, countOutboxRows(t))
}

func TestPostCommandModelDeletePostIdempotencySurvivesLostResponse(t *testing.T) {
	testEnv.TruncateAll(t, "post_tag", "post", "idempotency")
	testEnv.DB.ExecContext(context.Background(), "TRUNCATE TABLE `event_outbox`")
	ctx := context.Background()
	conn := newTestConn()
	command := NewPostCommandModel(conn, outboxx.NewSQLStore(conn)).(IdempotentPostCommandModel)
	postID := nextID(t)
	seedPost(t, postID, 7, 1, 5, "before")
	idem := idempotencyx.IdempotencyRecord{
		Scope: "post:delete", UserID: 7, Key: "agent-delete-lost-response",
		CommandHash: idempotencyx.CommandHash("same-command"),
	}
	beforeOutbox := countOutboxRows(t)
	applied, err := command.DeletePostIdempotent(ctx, postID, outboxEvent(t, "content-post-delete-v1"), 5, 6, idem)
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = command.DeletePostIdempotent(ctx, postID, outboxEvent(t, "content-post-delete-v1"), 5, 6, idem)
	require.NoError(t, err)
	require.False(t, applied)
	var row struct {
		Status   int64 `db:"status"`
		Revision int64 `db:"revision"`
	}
	require.NoError(t, conn.QueryRowCtx(ctx, &row, "SELECT status, revision FROM post WHERE id=?", postID))
	assert.Equal(t, int64(2), row.Status)
	assert.Equal(t, int64(6), row.Revision)
	assert.Equal(t, beforeOutbox+1, countOutboxRows(t))
}

func TestCommentCommandModelCreateAndDeleteAdjustCounts(t *testing.T) {
	testEnv.TruncateAll(t, "comment", "post_tag", "post", "idempotency")
	ctx := context.Background()

	conn := newTestConn()
	postID := nextID(t)
	seedPost(t, postID, 7, 1, 1, "commented")

	commentModel := NewCommentModel(conn, testCacheConf())
	commandModel := NewCommentCommandModel(conn, outboxx.NewSQLStore(conn))

	comment := &Comment{
		Id: nextID(t), PostId: postID, UserId: 8,
		Content: "nice", Status: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	commentID, created, err := commandModel.CreateComment(ctx, comment,
		outboxEvent(t, "content-comment-v1"), idempotencyx.IdempotencyRecord{})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, comment.Id, commentID)

	// 评论计数 +1，outbox 写入。
	postModel := NewPostModel(conn, testCacheConf())
	withCount, err := postModel.FindPostById(ctx, postID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), withCount.CommentCount)

	stored, err := commentModel.FindOne(ctx, comment.Id)
	require.NoError(t, err)
	assert.Equal(t, "nice", stored.Content)

	// 删除评论：状态置 0 且计数回落。命令写入绕过查询缓存，
	// 因此用裸 SQL 断言数据库真实状态（logic 层负责失效后回读）。
	require.NoError(t, commandModel.DeleteComment(ctx, comment))

	var rawCount, rawStatus int64
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &rawStatus,
		"SELECT status FROM `comment` WHERE `id` = ?", comment.Id))
	assert.Equal(t, int64(0), rawStatus)
	require.NoError(t, newTestConn().QueryRowCtx(ctx, &rawCount,
		"SELECT comment_count FROM `post` WHERE `id` = ?", postID))
	assert.Equal(t, int64(0), rawCount)

	// 重复删除同一评论：状态条件不再满足，报错而非静默成功。
	err = commandModel.DeleteComment(ctx, comment)
	require.Error(t, err)
}

func TestTagCategoryAndPostTagModels(t *testing.T) {
	testEnv.TruncateAll(t, "post_tag", "tag", "category", "post")
	ctx := context.Background()

	conn := newTestConn()
	tagModel := NewTagModel(conn, testCacheConf())

	tag := &Tag{Id: nextID(t), Name: "golang", PostCount: 3, Status: 1, CreatedAt: time.Now()}
	_, err := tagModel.Insert(ctx, tag)
	require.NoError(t, err)
	found, err := tagModel.FindOneByName(ctx, "golang")
	require.NoError(t, err)
	assert.Equal(t, tag.Id, found.Id)

	other := &Tag{Id: nextID(t), Name: "rust", PostCount: 1, Status: 1, CreatedAt: time.Now()}
	_, err = tagModel.Insert(ctx, other)
	require.NoError(t, err)
	tags, err := tagModel.FindList(ctx, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tags), 2)

	categoryModel := NewCategoryModel(conn, testCacheConf())
	category := &Category{Id: nextID(t), Name: "tech", SortOrder: 1, ParentId: 0, Status: 1, CreatedAt: time.Now()}
	_, err = categoryModel.Insert(ctx, category)
	require.NoError(t, err)
	foundCategory, err := categoryModel.FindOne(ctx, category.Id)
	require.NoError(t, err)
	assert.Equal(t, "tech", foundCategory.Name)

	// PostTag 批量写入 / 多帖查询 / 标签名反查 / 替换 / 清理。
	postTagModel := NewPostTagModel(conn, testCacheConf())
	postA, postB := nextID(t), nextID(t)
	seedPost(t, postA, 1, 1, 1, "pa")
	seedPost(t, postB, 1, 1, 1, "pb")
	require.NoError(t, postTagModel.BatchInsertTagsByPostId(ctx, conn, postA,
		[]string{"golang", "rust"}, []int64{nextID(t), nextID(t)}))
	require.NoError(t, postTagModel.BatchInsertTagsByPostId(ctx, conn, postB,
		[]string{"golang"}, []int64{nextID(t)}))

	byPosts, err := postTagModel.FindTagNamesByPostIds(ctx, []int64{postA, postB})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"golang", "rust"}, byPosts[postA])
	assert.Equal(t, []string{"golang"}, byPosts[postB])

	ids, total, err := postTagModel.FindPostIdsByTagName(ctx, "golang", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, ids, 2)

	require.NoError(t, postTagModel.TransactReplaceTagsByPostId(ctx, conn, postA,
		[]string{"python"}, []int64{nextID(t)}))
	names, err := postTagModel.FindTagNamesByPostId(ctx, postA)
	require.NoError(t, err)
	assert.Equal(t, []string{"python"}, names)

	require.NoError(t, postTagModel.DeleteByPostId(ctx, postA))
	empty, err := postTagModel.FindTagNamesByPostId(ctx, postA)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
