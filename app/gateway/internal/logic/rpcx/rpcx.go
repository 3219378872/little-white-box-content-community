// Package rpcx holds gateway helpers for JWT user extraction and RPC error mapping.
package rpcx

import (
	"context"

	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

func RequireUser(ctx context.Context) (int64, error) {
	userID, err := jwtx.GetUserIdFromContext(ctx)
	if err != nil {
		return 0, err
	}
	if userID <= 0 {
		return 0, errx.NewWithCode(errx.LoginRequired)
	}
	return userID, nil
}

func Error(logger logx.Logger, op string, err error, fields ...logx.LogField) error {
	if err == nil {
		return nil
	}
	args := make([]logx.LogField, 0, len(fields)+1)
	args = append(args, fields...)
	args = append(args, logx.Field("err", err.Error()))
	logger.Errorw(op+" RPC failed", args...)
	return errx.FromRPCError(err)
}
