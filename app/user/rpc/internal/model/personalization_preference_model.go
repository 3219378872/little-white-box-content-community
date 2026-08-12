package model

import (
	"context"
	"database/sql"
	"errors"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// ErrPersonalizationPreferenceNotFound 表示用户还没有显式设置过偏好（默认开启）。
var ErrPersonalizationPreferenceNotFound = errors.New("personalization preference not found")

// PersonalizationPreference 是用户的个性化偏好（REL-023）。
type PersonalizationPreference struct {
	UserID     int64
	Enabled    bool
	OptedOutAt sql.NullInt64 // Unix 毫秒；Enabled=false 时记录关闭时间
}

// PersonalizationPreferenceStore 提供个性化偏好的读写。
type PersonalizationPreferenceStore interface {
	Get(ctx context.Context, userID int64) (*PersonalizationPreference, error)
	Upsert(ctx context.Context, preference *PersonalizationPreference) error
}

type personalizationPreferenceModel struct {
	conn sqlx.SqlConn
}

func NewPersonalizationPreferenceModel(conn sqlx.SqlConn) PersonalizationPreferenceStore {
	return &personalizationPreferenceModel{conn: conn}
}

func (m *personalizationPreferenceModel) Get(ctx context.Context, userID int64) (*PersonalizationPreference, error) {
	if userID <= 0 {
		return nil, ErrPersonalizationPreferenceNotFound
	}
	var row struct {
		Enabled    int64         `db:"enabled"`
		OptedOutAt sql.NullInt64 `db:"opted_out_at"`
	}
	err := m.conn.QueryRowCtx(ctx, &row,
		"SELECT `enabled`, `opted_out_at` FROM `personalization_preference` WHERE `user_id` = ? LIMIT 1",
		userID)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, ErrPersonalizationPreferenceNotFound
		}
		return nil, err
	}
	return &PersonalizationPreference{UserID: userID, Enabled: row.Enabled == 1, OptedOutAt: row.OptedOutAt}, nil
}

func (m *personalizationPreferenceModel) Upsert(ctx context.Context, preference *PersonalizationPreference) error {
	if preference == nil || preference.UserID <= 0 {
		return errors.New("personalization preference: invalid record")
	}
	_, err := m.conn.ExecCtx(ctx,
		`INSERT INTO personalization_preference (user_id, enabled, opted_out_at, updated_at)
		 VALUES (?, ?, ?, NOW())
		 ON DUPLICATE KEY UPDATE enabled = VALUES(enabled), opted_out_at = VALUES(opted_out_at), updated_at = NOW()`,
		preference.UserID, boolToInt(preference.Enabled), preference.OptedOutAt)
	return err
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
