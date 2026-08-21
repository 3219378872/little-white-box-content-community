package logic

import (
	"context"
	"esx/app/user/rpc/internal/svc"
	"esx/pkg/jwtx"
	"fmt"

	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

// refreshKeyPrefix 刷新令牌 jti 白名单键前缀；值为所属用户 ID，
// 用于轮换时校验归属并使旧令牌一次性失效。
const refreshKeyPrefix = "auth:refresh:"

// issueTokenPair 签发访问/刷新令牌对，并把新 refresh token 的 jti
// 写入 Redis 白名单（TTL 与令牌有效期一致）。
func issueTokenPair(ctx context.Context, svcCtx *svc.ServiceContext, userId int64, username string) (access, refresh string, err error) {
	cfg := svcCtx.Config.JwtConfig
	access, err = jwtx.GenerateToken(userId, username, cfg)
	if err != nil {
		logx.WithContext(ctx).Errorw("jwtx.GenerateToken failed",
			logx.Field("userId", userId), logx.Field("err", err.Error()))
		return "", "", errx.NewWithCode(errx.SystemError)
	}
	refresh, err = jwtx.GenerateRefreshToken(userId, username, cfg)
	if err != nil {
		logx.WithContext(ctx).Errorw("jwtx.GenerateRefreshToken failed",
			logx.Field("userId", userId), logx.Field("err", err.Error()))
		return "", "", errx.NewWithCode(errx.SystemError)
	}
	if err := storeRefreshJTI(ctx, svcCtx, refresh, userId); err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// storeRefreshJTI 解析出 jti 并写入白名单。
func storeRefreshJTI(ctx context.Context, svcCtx *svc.ServiceContext, refreshToken string, userId int64) error {
	if svcCtx.RedisClient == nil {
		return nil // 测试环境无 Redis 时跳过白名单（轮换将不可用）
	}
	claims, err := jwtx.ParseRefreshToken(refreshToken, svcCtx.Config.JwtConfig)
	if err != nil {
		return errx.Wrap(err, errx.SystemError)
	}
	ttl := svcCtx.Config.JwtConfig.RefreshExpire
	if ttl <= 0 {
		ttl = 7 * 24 * 3600
	}
	if err := svcCtx.RedisClient.SetexCtx(ctx, refreshJTIKey(claims.ID), fmt.Sprintf("%d", userId), int(ttl)); err != nil {
		logx.WithContext(ctx).Errorw("store refresh jti failed",
			logx.Field("err", err.Error()))
		return errx.Wrap(err, errx.SystemError)
	}
	return nil
}

// rotateRefreshToken 校验旧刷新令牌（签名 + jti 白名单 + 归属一致），
// 一次性作废旧 jti 并签发全新令牌对。检测到重放（jti 不存在）即拒绝。
func rotateRefreshToken(ctx context.Context, svcCtx *svc.ServiceContext, oldRefreshToken string) (access, refresh string, err error) {
	claims, err := jwtx.ParseRefreshToken(oldRefreshToken, svcCtx.Config.JwtConfig)
	if err != nil {
		// 过期/伪造/格式错误统一映射为登录失效语义。
		return "", "", errx.NewWithCode(errx.LoginRequired)
	}
	if svcCtx.RedisClient == nil {
		return "", "", errx.NewWithCode(errx.SystemError)
	}
	key := refreshJTIKey(claims.ID)
	stored, err := svcCtx.RedisClient.GetCtx(ctx, key)
	if err != nil {
		logx.WithContext(ctx).Errorw("load refresh jti failed",
			logx.Field("err", err.Error()))
		return "", "", errx.Wrap(err, errx.SystemError)
	}
	if stored == "" || stored != fmt.Sprintf("%d", claims.UserId) {
		// jti 已被轮换消费或与声明归属不符：疑似重放，拒绝。
		return "", "", errx.NewWithCode(errx.LoginRequired)
	}
	if _, err := svcCtx.RedisClient.DelCtx(ctx, key); err != nil {
		logx.WithContext(ctx).Errorw("delete rotated refresh jti failed",
			logx.Field("err", err.Error()))
		return "", "", errx.Wrap(err, errx.SystemError)
	}
	return issueTokenPair(ctx, svcCtx, claims.UserId, claims.Username)
}

func refreshJTIKey(jti string) string {
	return fmt.Sprintf("%s%s", refreshKeyPrefix, jti)
}
