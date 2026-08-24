package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"esx/pkg/visibilityx"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ PostModel = (*customPostModel)(nil)

// allowedUpdateCols UpdateFields 允许的列白名单，防止 SQL 注入
var allowedUpdateCols = map[string]struct{}{
	"title":   {},
	"content": {},
	"images":  {},
	"status":  {},
}

type (
	// PostModel is an interface to be customized, add more methods here,
	// and implement the added methods in customPostModel.
	PostModel interface {
		postModel
		FindPostById(ctx context.Context, id int64) (*Post, error)
		InsertPost(ctx context.Context, post *Post) error
		InsertPostTx(ctx context.Context, tx *sql.Tx, post *Post) error
		FindUserPostsByCursor(ctx context.Context, authorId int64, cur *PostListCursor, pageSize int) ([]*Post, bool, error)
		FindListByCursor(ctx context.Context, cur *PostListCursor, pageSize int) ([]*Post, bool, error)
		FindByIds(ctx context.Context, ids []int64) ([]*Post, error)
		UpdateStatus(ctx context.Context, id int64, status int64) error
		UpdateFields(ctx context.Context, id int64, fields map[string]any) error
		InvalidatePostCache(ctx context.Context, id int64) error
		IncrCommentCount(ctx context.Context, postId int64) error
		DecrCommentCount(ctx context.Context, postId int64) error
	}

	customPostModel struct {
		*defaultPostModel
	}
)

// PostListCursor keyset 游标：Vals 与当前排序模式的键列一一对应
// （末位恒为二级键 id），由 Logic 层从上一页边界行经 NewListCursor /
// NewUserPostsCursor 构造。nil 表示首页。
type PostListCursor struct {
	SortBy int
	Vals   []int64
}

// listKeysetColumns 全局帖子列表各排序模式的键列，与 ORDER BY 完全一致
// （CORE-060：确定性二级键 id 防跨页漂移）。
func listKeysetColumns(sortBy int) []string {
	switch sortBy {
	case SortByHot:
		return []string{"`like_count`", "`id`"}
	case SortByViewed:
		return []string{"`view_count`", "`id`"}
	default:
		return []string{"`created_at`", "`id`"}
	}
}

// userPostsKeysetColumns 用户主页列表的键列（热门带 created_at 中间键）。
func userPostsKeysetColumns(sortBy int) []string {
	if sortBy == SortByHot {
		return []string{"`like_count`", "`created_at`", "`id`"}
	}
	return []string{"`created_at`", "`id`"}
}

// keysetCondition 构造 (c1..cn) < (v1..vn) 的展开 OR 条件，
// 避免 MySQL 行构造符在旧版本下的索引利用问题。
func keysetCondition(cols []string) string {
	parts := make([]string, 0, len(cols))
	for i := range cols {
		var conds []string
		for j := range i {
			conds = append(conds, fmt.Sprintf("%s = ?", cols[j]))
		}
		conds = append(conds, fmt.Sprintf("%s < ?", cols[i]))
		parts = append(parts, "("+strings.Join(conds, " and ")+")")
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

// ErrCursorArity 游标值个数与当前排序键列数不一致（解码层漏校验时兜底）。
var ErrCursorArity = errors.New("post list cursor arity mismatch")

// ErrInvalidCursorArity 判断错误是否为游标维度不匹配。
func ErrInvalidCursorArity(err error) bool { return errors.Is(err, ErrCursorArity) }

// findByKeyset 是两类列表共用的 keyset 查询：多取一行判定 hasMore 后截断。
func (m *customPostModel) findByKeyset(
	ctx context.Context,
	cur *PostListCursor,
	pageSize int,
	cols []string,
	where string,
	baseArgs []any,
) ([]*Post, bool, error) {
	if cur != nil && len(cur.Vals) > 0 && len(cur.Vals) != len(cols) {
		return nil, false, fmt.Errorf("%w: want %d got %d", ErrCursorArity, len(cols), len(cur.Vals))
	}

	orderBy := strings.Join(cols, " desc, ") + " desc"
	conds := []string{where}
	args := append([]any{}, baseArgs...)
	if cur != nil && len(cur.Vals) > 0 {
		conds = append(conds, keysetCondition(cols))
		// 与 keysetCondition 的部件顺序严格对应：第 i 个部件消耗 Vals[0..i]。
		for i := range cols {
			for j := 0; j <= i; j++ {
				args = append(args, cur.Vals[j])
			}
		}
	}

	query := fmt.Sprintf(
		"select %s from %s where %s order by %s limit ?",
		postRows, m.table, strings.Join(conds, " and "), orderBy,
	)
	var posts []*Post
	err := m.QueryRowsNoCacheCtx(ctx, &posts, query, append(args, pageSize+1)...)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(posts) > pageSize
	if hasMore {
		posts = posts[:pageSize]
	}
	return posts, hasMore, nil
}

// NewPostModel returns a model for the database table.
func NewPostModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) PostModel {
	return &customPostModel{
		defaultPostModel: newPostModel(conn, c, opts...),
	}
}

// FindPostById 按主键查询帖子（业务专用，显式 SQL）
func (m *customPostModel) FindPostById(ctx context.Context, id int64) (*Post, error) {
	var post Post
	query := fmt.Sprintf("select %s from %s where `id`=? limit 1", postRows, m.table)
	err := m.QueryRowNoCacheCtx(ctx, &post, query, id)
	switch {
	case err == nil:
		if post.Images.Valid {
			// 处理images的Json格式 []
			var images []string
			err = json.Unmarshal([]byte(post.Images.String), &images)
			if err != nil {
				return nil, err
			}
			post.Images.String = strings.Join(images, ",")
		}
		return &post, err
	case errors.Is(err, sqlx.ErrNotFound):
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// InsertPost 插入帖子（显式字段列，避免依赖 gen 生成的通用 Insert）
func (m *customPostModel) InsertPost(ctx context.Context, post *Post) error {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, post.Id)
	// 校验post中images是否为空
	if !post.Images.Valid {
		_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
			query := fmt.Sprintf("insert into %s (`id`,`author_id`,`title`,`content`,`status`,`revision`) values (?,?,?,?,?,?)", m.table)
			return conn.ExecCtx(ctx, query, post.Id, post.AuthorId, post.Title, post.Content, post.Status, post.Revision)
		}, postIdKey)
		return err
	}
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("insert into %s (`id`,`author_id`,`title`,`content`,`images`,`status`,`revision`) values (?,?,?,?,?,?,?)", m.table)
		return conn.ExecCtx(ctx, query, post.Id, post.AuthorId, post.Title, post.Content, post.Images.String, post.Status, post.Revision)
	}, postIdKey)
	return err
}

func (m *customPostModel) InsertPostTx(ctx context.Context, tx *sql.Tx, post *Post) error {
	if tx == nil {
		return fmt.Errorf("nil sql transaction")
	}
	if !post.Images.Valid {
		query := fmt.Sprintf("insert into %s (`id`,`author_id`,`title`,`content`,`status`,`revision`) values (?,?,?,?,?,?)", m.table)
		_, err := tx.ExecContext(ctx, query, post.Id, post.AuthorId, post.Title, post.Content, post.Status, post.Revision)
		return err
	}
	query := fmt.Sprintf("insert into %s (`id`,`author_id`,`title`,`content`,`images`,`status`,`revision`) values (?,?,?,?,?,?,?)", m.table)
	_, err := tx.ExecContext(ctx, query, post.Id, post.AuthorId, post.Title, post.Content, post.Images.String, post.Status, post.Revision)
	return err
}

// FindUserPostsByCursor keyset 游标分页获取用户已发布帖子。
// hasMore 表示是否还有下一页；返回行数至多 pageSize。
func (m *customPostModel) FindUserPostsByCursor(ctx context.Context, authorId int64, cur *PostListCursor, pageSize int) ([]*Post, bool, error) {
	return m.findByKeyset(
		ctx, cur, pageSize,
		userPostsKeysetColumns(curSortBy(cur)),
		"`author_id` = ? and `status` = "+fmt.Sprint(visibilityx.PublishedStatus),
		[]any{authorId},
	)
}

// FindListByCursor keyset 游标分页获取全局已发布帖子。
func (m *customPostModel) FindListByCursor(ctx context.Context, cur *PostListCursor, pageSize int) ([]*Post, bool, error) {
	return m.findByKeyset(
		ctx, cur, pageSize,
		listKeysetColumns(curSortBy(cur)),
		"`status` = "+fmt.Sprint(visibilityx.PublishedStatus),
		nil,
	)
}

// curSortBy 容忍 nil 首页游标，统一取排序模式。
func curSortBy(cur *PostListCursor) int {
	if cur == nil {
		return SortByLatest
	}
	return cur.SortBy
}

func (m *customPostModel) UpdateStatus(ctx context.Context, id int64, status int64) error {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `status` = ? where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, status, id)
	}, postIdKey)
	return err
}

// UpdateFields 动态更新帖子字段（PATCH 语义），只更新 fields 中显式传入的字段，
// 避免覆盖计数等字段（防止 Lost Update），空 fields 直接返回。
func (m *customPostModel) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	for col, val := range fields {
		if _, ok := allowedUpdateCols[col]; !ok {
			return fmt.Errorf("UpdateFields: disallowed column %q", col)
		}
		setClauses = append(setClauses, fmt.Sprintf("`%s`=?", col))
		args = append(args, val)
	}
	args = append(args, id)
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set %s where `id`=?", m.table, strings.Join(setClauses, ", "))
		return conn.ExecCtx(ctx, query, args...)
	}, postIdKey)
	return err
}

func (m *customPostModel) InvalidatePostCache(ctx context.Context, id int64) error {
	return m.DelCacheCtx(ctx, fmt.Sprintf("%s%v", cachePostIdPrefix, id))
}

// IncrCommentCount 原子递增评论数，避免并发写丢失
func (m *customPostModel) IncrCommentCount(ctx context.Context, postId int64) error {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, postId)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `comment_count`=`comment_count`+1 where `id`=?", m.table)
		return conn.ExecCtx(ctx, query, postId)
	}, postIdKey)
	return err
}

// DecrCommentCount 原子递减评论数，不低于 0
func (m *customPostModel) DecrCommentCount(ctx context.Context, postId int64) error {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, postId)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `comment_count`=GREATEST(`comment_count`-1,0) where `id`=?", m.table)
		return conn.ExecCtx(ctx, query, postId)
	}, postIdKey)
	return err
}

// FindByIds 批量查询帖子，避免 N+1 查询
func (m *customPostModel) FindByIds(ctx context.Context, ids []int64) ([]*Post, error) {
	if len(ids) == 0 {
		return []*Post{}, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	var posts []*Post
	query := fmt.Sprintf("select %s from %s where `id` IN (%s)", postRows, m.table, placeholders)
	if err := m.QueryRowsNoCacheCtx(ctx, &posts, query, args...); err != nil {
		return nil, err
	}
	return posts, nil
}
