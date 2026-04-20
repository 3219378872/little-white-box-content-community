package config

import (
	"esx/app/media/internal/storage"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DataSource string
	S3Storage  storage.Config
	Upload     UploadConf
}

// UploadConf 上传相关阈值与路径。
type UploadConf struct {
	MaxImageSize      int64
	MaxVideoSize      int64
	DefaultQuality    int
	ThumbnailLongSide int
	TempDir           string
}
