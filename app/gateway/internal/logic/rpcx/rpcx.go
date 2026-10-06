// Package rpcx maps RPC errors to gateway business errors with logging.
package rpcx

import (
	"esx/pkg/errx"

	"esx/pkg/logging"
)

func Error(logger logging.Logger, op string, err error, fields ...logging.LogField) error {
	if err == nil {
		return nil
	}
	args := make([]logging.LogField, 0, len(fields)+1)
	args = append(args, fields...)
	args = append(args, logging.Field("err", err.Error()))
	logger.Errorw(op+" RPC failed", args...)
	return errx.FromRPCError(err)
}
