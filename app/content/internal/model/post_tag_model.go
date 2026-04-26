package model

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ PostTagModel = (*customPostTagModel)(nil)

type (
	// PostTagModel is an interface to be customized, add more methods here,
	// and implement the added methods in customPostTagModel.
	PostTagModel interface {
		postTagModel
		FindTagNamesByPostId(ctx context.Context, postId int64) ([]string, error)
		FindTagNamesByPostIds(ctx context.Context, postIds []int64) (map[int64][]string, error)
		FindPostIdsByTagName(ctx context.Context, tagName string, page, pageSize int) ([]int64, int64, error)
		DeleteByPostId(ctx context.Context, postId int64) error
		TransactReplaceTagsByPostId(ctx context.Context, conn sqlx.SqlConn, postId int64, tags []string, ids []int64) error
		BatchInsertTagsByPostId(ctx context.Context, conn sqlx.SqlConn, postId int64, tags []string, ids []int64) error
	}

	customPostTagModel struct {
		*defaultPostTagModel
	}
)

// NewPostTagModel returns a model for the database table.
func NewPostTagModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) PostTagModel {
	return &customPostTagModel{
		defaultPostTagModel: newPostTagModel(conn, c, opts...),
	}
}

func (m *customPostTagModel) FindTagNamesByPostId(ctx context.Context, postId int64) ([]string, error) {
	var rows []struct {
		TagName string `db:"tag_name"`
	}
	query := fmt.Sprintf("select `tag_name` from %s where `post_id` = ?", m.table)
	err := m.QueryRowsNoCacheCtx(ctx, &rows, query, postId)
	if err != nil {
		return nil, err
	}

	tagNames := make([]string, 0, len(rows))
	for _, r := range rows {
		tagNames = append(tagNames, r.TagName)
	}
	return tagNames, nil
}

// FindTagNamesByPostIds 批量查询多个帖子的标签，返回 map[postId][]tagName，避免 N+1 查询
func (m *customPostTagModel) FindTagNamesByPostIds(ctx context.Context, postIds []int64) (map[int64][]string, error) {
	if len(postIds) == 0 {
		return map[int64][]string{}, nil
	}
	placeholders := make([]string, len(postIds))
	args := make([]interface{}, len(postIds))
	for i, id := range postIds {
		placeholders[i] = "?"
		args[i] = id
	}
	var rows []struct {
		PostId  int64  `db:"post_id"`
		TagName string `db:"tag_name"`
	}
	query := fmt.Sprintf("select `post_id`, `tag_name` from %s where `post_id` in (%s)",
		m.table, strings.Join(placeholders, ","))
	if err := m.QueryRowsNoCacheCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make(map[int64][]string, len(postIds))
	for _, r := range rows {
		result[r.PostId] = append(result[r.PostId], r.TagName)
	}
	return result, nil
}

// FindPostIdsByTagName 查询标签下已发布帖子（JOIN post 过滤 status=1），total 与返回数据一致
func (m *customPostTagModel) FindPostIdsByTagName(ctx context.Context, tagName string, page, pageSize int) ([]int64, int64, error) {
	offset := (page - 1) * pageSize

	var rows []struct {
		PostId int64 `db:"post_id"`
	}
	query := fmt.Sprintf("select pt.`post_id` from %s pt join `post` p on pt.`post_id`=p.`id` where pt.`tag_name`=? and p.`status`=1 limit ?,?", m.table)
	err := m.QueryRowsNoCacheCtx(ctx, &rows, query, tagName, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s pt join `post` p on pt.`post_id`=p.`id` where pt.`tag_name`=? and p.`status`=1", m.table)
	err = m.QueryRowNoCacheCtx(ctx, &total, countQuery, tagName)
	if err != nil {
		return nil, 0, err
	}

	postIds := make([]int64, 0, len(rows))
	for _, r := range rows {
		postIds = append(postIds, r.PostId)
	}
	return postIds, total, nil
}

func (m *customPostTagModel) DeleteByPostId(ctx context.Context, postId int64) error {
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("delete from %s where `post_id` = ?", m.table)
		return conn.ExecCtx(ctx, query, postId)
	})
	return err
}

// BatchInsertTagsByPostId 在事务中批量插入帖子标签，全部成功或全部回滚
func (m *customPostTagModel) BatchInsertTagsByPostId(ctx context.Context, conn sqlx.SqlConn, postId int64, tags []string, ids []int64) error {
	if len(tags) == 0 {
		return nil
	}
	return conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		for i, tag := range tags {
			if _, err := session.ExecCtx(ctx,
				fmt.Sprintf("INSERT INTO %s (`id`,`post_id`,`tag_name`) VALUES (?,?,?)", m.table),
				ids[i], postId, tag,
			); err != nil {
				return fmt.Errorf("插入标签失败 tag=%s: %w", tag, err)
			}
		}
		return nil
	})
}

// TransactReplaceTagsByPostId 在事务中原子替换帖子标签（先删再插），避免部分失败导致数据不一致
func (m *customPostTagModel) TransactReplaceTagsByPostId(ctx context.Context, conn sqlx.SqlConn, postId int64, tags []string, ids []int64) error {
	return conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if _, err := session.ExecCtx(ctx, "DELETE FROM `post_tag` WHERE `post_id`=?", postId); err != nil {
			return err
		}
		for i, tag := range tags {
			if _, err := session.ExecCtx(ctx, "INSERT INTO `post_tag` (`id`, `post_id`, `tag_name`) VALUES (?,?,?)", ids[i], postId, tag); err != nil {
				return err
			}
		}
		return nil
	})
}
