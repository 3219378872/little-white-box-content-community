package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"esx/pkg/adpolicy"
	sqlx "esx/pkg/sqlstore"
)

// AllRoles 是可授予的审核角色。
var AllRoles = []string{RoleReviewer, RoleQA, RolePolicyAdmin, RoleQualificationReviewer}

// Grant 是一次角色授予。
type Grant struct {
	UserID    int64
	Roles     []string
	Markets   []string
	Languages []string
}

func (g Grant) validate() error {
	if g.UserID <= 0 || len(g.Roles) == 0 {
		return fmt.Errorf("review: user and roles are required")
	}
	for _, role := range g.Roles {
		if !slices.Contains(AllRoles, role) {
			return fmt.Errorf("review: unknown role %q", role)
		}
	}
	for _, market := range g.Markets {
		if !adpolicy.IsMarket(market) {
			return fmt.Errorf("review: unknown market %q", market)
		}
	}
	for _, language := range g.Languages {
		if !slices.ContainsFunc(adpolicy.Markets, func(m adpolicy.Market) bool { return m.Language == language }) {
			return fmt.Errorf("review: unknown language %q", language)
		}
	}
	return nil
}

// GrantRoles 只供运维脚本调用（RVW-050）；授予与撤销都写审计（RVW-025）。
func (s *Store) GrantRoles(ctx context.Context, actor int64, g Grant, now time.Time) error {
	if err := g.validate(); err != nil {
		return err
	}
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		before, err := lockReviewer(ctx, session, g.UserID)
		if err != nil {
			return err
		}
		after := map[string]any{"roles": g.Roles, "markets": g.Markets, "languages": g.Languages, "active": true}
		if _, err := session.ExecCtx(ctx, `INSERT INTO reviewer (user_id, roles, markets, languages, active, updated_at_ms)
			VALUES (?, ?, ?, ?, 1, ?)
			ON DUPLICATE KEY UPDATE roles = VALUES(roles), markets = VALUES(markets), languages = VALUES(languages),
				active = 1, updated_at_ms = VALUES(updated_at_ms)`,
			g.UserID, strings.Join(g.Roles, ","), strings.Join(g.Markets, ","), strings.Join(g.Languages, ","), now.UnixMilli()); err != nil {
			return err
		}
		return s.insertAudit(ctx, session, AuditEntry{ActorID: actor, Action: "role.grant", ObjectType: "reviewer",
			ObjectID: g.UserID, Before: before, After: after}, now)
	})
}

// RevokeRoles 停用审核员；下一次请求即被拒绝。
func (s *Store) RevokeRoles(ctx context.Context, actor, userID int64, now time.Time) error {
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		before, err := lockReviewer(ctx, session, userID)
		if err != nil {
			return err
		}
		if before == nil {
			return ErrReviewerUnknown
		}
		if _, err := session.ExecCtx(ctx, `UPDATE reviewer SET active = 0, updated_at_ms = ? WHERE user_id = ?`,
			now.UnixMilli(), userID); err != nil {
			return err
		}
		return s.insertAudit(ctx, session, AuditEntry{ActorID: actor, Action: "role.revoke", ObjectType: "reviewer",
			ObjectID: userID, Before: before, After: map[string]any{"active": false}}, now)
	})
}

func lockReviewer(ctx context.Context, session sqlx.Session, userID int64) (map[string]any, error) {
	var r Reviewer
	err := session.QueryRowCtx(ctx, &r, `SELECT user_id, roles, markets, languages, active, updated_at_ms
		FROM reviewer WHERE user_id = ? FOR UPDATE`, userID)
	if errors.Is(err, sqlx.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"roles": r.Roles(), "markets": r.Markets(), "languages": r.Languages(), "active": r.Active}, nil
}
