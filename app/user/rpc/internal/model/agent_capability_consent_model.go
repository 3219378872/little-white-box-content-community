package model

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sqlx "esx/pkg/sqlstore"
)

// ErrAgentCapabilityConsentNotFound 表示用户从未显式授权过 Agent 能力（默认未授权）。
var ErrAgentCapabilityConsentNotFound = errors.New("agent capability consent not found")

// AgentCapabilityConsent 是用户对 Assistant Agent 模式的授权记录（AGNT-004/006）。
// CurrentAgentConsentVersion 是服务端当前披露版本（AGNT-007）。
const CurrentAgentConsentVersion int32 = 3

// AgentCapabilityConsent 是用户对 Assistant 能力的授权状态及最近一次授权/撤销时间。
type AgentCapabilityConsent struct {
	UserID         int64
	Granted        bool
	GrantedAt      sql.NullInt64 // Unix 毫秒；最近一次授权时间
	RevokedAt      sql.NullInt64 // Unix 毫秒；最近一次撤销时间
	ConsentVersion int32
}

// AgentCapabilityConsentStore 提供 Agent 能力授权的读写。
type AgentCapabilityConsentStore interface {
	Get(ctx context.Context, userID int64) (*AgentCapabilityConsent, error)
	Upsert(ctx context.Context, consent *AgentCapabilityConsent) error
}

// agentCapabilityConsentModel 是基于 MySQL 的授权存储。
type agentCapabilityConsentModel struct {
	conn sqlx.SqlConn
}

// NewAgentCapabilityConsentModel 创建授权存储。
func NewAgentCapabilityConsentModel(conn sqlx.SqlConn) AgentCapabilityConsentStore {
	return &agentCapabilityConsentModel{conn: conn}
}

// Get 读取用户的授权记录；从未授权过时返回 ErrAgentCapabilityConsentNotFound。
func (m *agentCapabilityConsentModel) Get(ctx context.Context, userID int64) (*AgentCapabilityConsent, error) {
	if userID <= 0 {
		return nil, ErrAgentCapabilityConsentNotFound
	}
	var row struct {
		Granted        int64         `db:"granted"`
		GrantedAt      sql.NullInt64 `db:"granted_at"`
		RevokedAt      sql.NullInt64 `db:"revoked_at"`
		ConsentVersion int32         `db:"consent_version"`
	}
	err := m.conn.QueryRowCtx(ctx, &row,
		"SELECT `granted`, `granted_at`, `revoked_at`, `consent_version` FROM `agent_capability_consent` WHERE `user_id` = ? LIMIT 1",
		userID)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, ErrAgentCapabilityConsentNotFound
		}
		return nil, err
	}
	return &AgentCapabilityConsent{
		UserID:         userID,
		Granted:        row.Granted == 1,
		GrantedAt:      row.GrantedAt,
		RevokedAt:      row.RevokedAt,
		ConsentVersion: row.ConsentVersion,
	}, nil
}

// Upsert 写入授权状态；调用方未给出时间时，按本次授权或撤销补上当前时间。
func (m *agentCapabilityConsentModel) Upsert(ctx context.Context, consent *AgentCapabilityConsent) error {
	if consent == nil || consent.UserID <= 0 {
		return errors.New("agent capability consent: invalid record")
	}
	nowMilli := timeNowMillis()
	grantedAt := consent.GrantedAt
	revokedAt := consent.RevokedAt
	if consent.Granted && !grantedAt.Valid {
		grantedAt = sql.NullInt64{Int64: nowMilli, Valid: true}
	}
	if !consent.Granted && !revokedAt.Valid {
		revokedAt = sql.NullInt64{Int64: nowMilli, Valid: true}
	}
	_, err := m.conn.ExecCtx(ctx,
		`INSERT INTO agent_capability_consent (user_id, granted, granted_at, revoked_at, consent_version, updated_at)
		 VALUES (?, ?, ?, ?, ?, NOW())
		 ON DUPLICATE KEY UPDATE granted = VALUES(granted), granted_at = VALUES(granted_at),
		   revoked_at = VALUES(revoked_at), consent_version = VALUES(consent_version), updated_at = NOW()`,
		consent.UserID, boolToInt(consent.Granted), grantedAt, revokedAt, consent.ConsentVersion)
	return err
}

// timeNowMillis 返回当前毫秒时间戳。
func timeNowMillis() int64 {
	return time.Now().UnixMilli()
}
