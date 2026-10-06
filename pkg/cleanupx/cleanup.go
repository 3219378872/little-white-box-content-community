package cleanupx

import (
	"io"
	"os"

	"esx/pkg/logging"
)

// Close 关闭资源并只记录失败；用于 defer 场景，关闭错误不应覆盖主流程的返回值。
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

// Remove 删除临时文件并只记录失败，空路径直接忽略。
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

// Shutdown 执行优雅停止函数并只记录失败，供进程退出时逐个释放组件。
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
