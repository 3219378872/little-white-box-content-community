package logic

import (
	"context"
	"database/sql"
	"errx"
	"fmt"
	"time"

	"user/internal/model"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

// personalizationOptOutRedisKey 是跨服务共享的个性化关闭标记。
// user 服务在关闭时写入，recommend-rpc 与 recommend-mq 据此停止个性化并清理特征。
// 该键是 REL-023 的运行时握手契约；DB 是权威来源，Redis 只是快速标记。
func personalizationOptOutRedisKey(userID int64) string {
	return fmt.Sprintf("personalization:optout:%d", userID)
}

type SetPersonalizationPreferenceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSetPersonalizationPreferenceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetPersonalizationPreferenceLogic {
	return &SetPersonalizationPreferenceLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 设置个性化偏好（REL-023）
func (l *SetPersonalizationPreferenceLogic) SetPersonalizationPreference(in *pb.SetPersonalizationPreferenceReq) (*pb.SetPersonalizationPreferenceResp, error) {
	if in == nil || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.Personalization == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	preference := &model.PersonalizationPreference{
		UserID:  in.UserId,
		Enabled: in.Enabled,
	}
	if in.Enabled {
		preference.OptedOutAt = sql.NullInt64{}
	} else {
		preference.OptedOutAt = sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true}
	}
	if err := l.svcCtx.Personalization.Upsert(l.ctx, preference); err != nil {
		l.Errorw("Personalization.Upsert failed", logx.Field("user_id", in.UserId), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.RedisClient != nil {
		key := personalizationOptOutRedisKey(in.UserId)
		if in.Enabled {
			if _, err := l.svcCtx.RedisClient.DelCtx(l.ctx, key); err != nil {
				l.Errorw("clear personalization opt-out marker failed", logx.Field("user_id", in.UserId), logx.Field("err", err.Error()))
			}
		} else {
			// 标记保留 7 天，与推荐在线特征 30 天 TTL 相比更短；DB 仍是权威来源。
			if err := l.svcCtx.RedisClient.SetexCtx(l.ctx, key, "1", 7*24*3600); err != nil {
				l.Errorw("set personalization opt-out marker failed", logx.Field("user_id", in.UserId), logx.Field("err", err.Error()))
			}
		}
	}

	return &pb.SetPersonalizationPreferenceResp{}, nil
}
