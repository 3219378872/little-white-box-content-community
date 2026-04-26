package cleanupx

import (
	"io"
	"os"

	"github.com/zeromicro/go-zero/core/logx"
)

func Close(logger logx.Logger, resource string, closer io.Closer) {
	if closer == nil {
		return
	}

	if err := closer.Close(); err != nil {
		logger.Errorw("close resource failed",
			logx.Field("resource", resource),
			logx.Field("err", err.Error()),
		)
	}
}

func Remove(logger logx.Logger, path string) {
	if path == "" {
		return
	}

	if err := os.Remove(path); err != nil {
		logger.Errorw("remove path failed",
			logx.Field("path", path),
			logx.Field("err", err.Error()),
		)
	}
}

func Shutdown(logger logx.Logger, resource string, shutdown func() error) {
	if shutdown == nil {
		return
	}

	if err := shutdown(); err != nil {
		logger.Errorw("shutdown resource failed",
			logx.Field("resource", resource),
			logx.Field("err", err.Error()),
		)
	}
}
