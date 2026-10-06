package cleanupx

import (
	"io"
	"os"

	"esx/pkg/logging"
)

func Close(logger logging.Logger, resource string, closer io.Closer) {
	if closer == nil {
		return
	}

	if err := closer.Close(); err != nil {
		logger.Errorw("close resource failed",
			logging.Field("resource", resource),
			logging.Field("err", err.Error()),
		)
	}
}

func Remove(logger logging.Logger, path string) {
	if path == "" {
		return
	}

	if err := os.Remove(path); err != nil {
		logger.Errorw("remove path failed",
			logging.Field("path", path),
			logging.Field("err", err.Error()),
		)
	}
}

func Shutdown(logger logging.Logger, resource string, shutdown func() error) {
	if shutdown == nil {
		return
	}

	if err := shutdown(); err != nil {
		logger.Errorw("shutdown resource failed",
			logging.Field("resource", resource),
			logging.Field("err", err.Error()),
		)
	}
}
