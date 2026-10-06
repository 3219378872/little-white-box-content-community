// Explicit SQL model. Maintained with its domain query extensions.

package model

import (
	"context"
	"database/sql"
	sqlc "esx/pkg/cachedstore"
	cache "esx/pkg/modelcache"
	sqlx "esx/pkg/sqlstore"
	"fmt"
	"time"
)

var (
	mediaRows                = "`id`,`user_id`,`file_name`,`original_name`,`file_type`,`mime_type`,`url`,`thumbnail_url`,`storage_type`,`bucket`,`object_key`,`thumbnail_object_key`,`file_size`,`width`,`height`,`duration`,`format`,`bit_rate`,`status`,`created_at`,`updated_at`"
	mediaRowsExpectAutoSet   = "`user_id`,`file_name`,`original_name`,`file_type`,`mime_type`,`url`,`thumbnail_url`,`storage_type`,`bucket`,`object_key`,`file_size`,`width`,`height`,`duration`,`format`,`bit_rate`,`status`"
	mediaRowsWithPlaceHolder = "`user_id`=?,`file_name`=?,`original_name`=?,`file_type`=?,`mime_type`=?,`url`=?,`thumbnail_url`=?,`storage_type`=?,`bucket`=?,`object_key`=?,`file_size`=?,`width`=?,`height`=?,`duration`=?,`format`=?,`bit_rate`=?,`status`=?"

	cacheMediaIdPrefix = "cache:media:id:"
)

type (
	mediaModel interface {
		Insert(ctx context.Context, data *Media) (sql.Result, error)
		FindOne(ctx context.Context, id int64) (*Media, error)
		Update(ctx context.Context, data *Media) error
		Delete(ctx context.Context, id int64) error
	}

	defaultMediaModel struct {
		sqlc.CachedConn
		table string
	}

	Media struct {
		Id                 int64          `db:"id"`
		UserId             int64          `db:"user_id"`              // 上传用户ID
		FileName           string         `db:"file_name"`            // 文件名
		OriginalName       sql.NullString `db:"original_name"`        // 原始文件名
		FileType           string         `db:"file_type"`            // 文件类型 image/video/audio
		MimeType           sql.NullString `db:"mime_type"`            // MIME类型
		Url                string         `db:"url"`                  // 访问URL
		ThumbnailUrl       sql.NullString `db:"thumbnail_url"`        // 缩略图URL
		StorageType        int64          `db:"storage_type"`         // 存储类型 1:MinIO 2:OSS
		Bucket             sql.NullString `db:"bucket"`               // 存储桶
		ObjectKey          sql.NullString `db:"object_key"`           // 对象键
		ThumbnailObjectKey sql.NullString `db:"thumbnail_object_key"` // 缩略图对象键
		FileSize           int64          `db:"file_size"`            // 文件大小(字节)
		Width              sql.NullInt64  `db:"width"`                // 宽度
		Height             sql.NullInt64  `db:"height"`               // 高度
		Duration           sql.NullInt64  `db:"duration"`             // 时长(秒)
		Format             sql.NullString `db:"format"`               // 格式
		BitRate            sql.NullInt64  `db:"bit_rate"`             // 比特率
		Status             int64          `db:"status"`               // 状态 0:删除 1:正常 2:处理中
		CreatedAt          time.Time      `db:"created_at"`           // 创建时间
		UpdatedAt          time.Time      `db:"updated_at"`           // 更新时间
	}
)

// newMediaModel 创建带缓存连接的基础模型。
func newMediaModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) *defaultMediaModel {
	return &defaultMediaModel{
		CachedConn: sqlc.NewConn(conn, c, opts...),
		table:      "`media`",
	}
}

// Delete 按主键硬删除并清理缓存；业务删除走 MediaCommandModel.SoftDelete。
func (m *defaultMediaModel) Delete(ctx context.Context, id int64) error {
	mediaIdKey := fmt.Sprintf("%s%v", cacheMediaIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("delete from %s where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, id)
	}, mediaIdKey)
	return err
}

// FindOne 按主键直查数据库，未找到返回 ErrNotFound。
func (m *defaultMediaModel) FindOne(ctx context.Context, id int64) (*Media, error) {
	var resp Media
	// Status and ownership authorize media use. Even a fenced cache cannot
	// provide authoritative state when post-commit invalidation fails.
	query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", mediaRows, m.table)
	err := m.QueryRowNoCacheCtx(ctx, &resp, query, id)
	switch err {
	case nil:
		return &resp, nil
	case sqlc.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// Insert 是旧的基础写入，不含 thumbnail_object_key；customMediaModel 覆盖了它。
func (m *defaultMediaModel) Insert(ctx context.Context, data *Media) (sql.Result, error) {
	mediaIdKey := fmt.Sprintf("%s%v", cacheMediaIdPrefix, data.Id)
	ret, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("insert into %s (%s) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", m.table, mediaRowsExpectAutoSet)
		return conn.ExecCtx(ctx, query, data.UserId, data.FileName, data.OriginalName, data.FileType, data.MimeType, data.Url, data.ThumbnailUrl, data.StorageType, data.Bucket, data.ObjectKey, data.FileSize, data.Width, data.Height, data.Duration, data.Format, data.BitRate, data.Status)
	}, mediaIdKey)
	return ret, err
}

// Update 是旧的基础更新，不含 thumbnail_object_key；customMediaModel 覆盖了它。
func (m *defaultMediaModel) Update(ctx context.Context, data *Media) error {
	mediaIdKey := fmt.Sprintf("%s%v", cacheMediaIdPrefix, data.Id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("update %s set %s where `id` = ?", m.table, mediaRowsWithPlaceHolder)
		return conn.ExecCtx(ctx, query, data.UserId, data.FileName, data.OriginalName, data.FileType, data.MimeType, data.Url, data.ThumbnailUrl, data.StorageType, data.Bucket, data.ObjectKey, data.FileSize, data.Width, data.Height, data.Duration, data.Format, data.BitRate, data.Status, data.Id)
	}, mediaIdKey)
	return err
}
