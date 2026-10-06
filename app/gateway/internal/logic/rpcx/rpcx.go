// Package rpcx maps RPC errors to gateway business errors with logging.
package rpcx

import (
	"esx/pkg/errx"

	logx "esx/pkg/logging"
)

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
