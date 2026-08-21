package config

import (
	"esx/app/media/rpc/internal/storage"
	"esx/pkg/outboxx"
	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	InternalSecret string
	DataSource string
	S3Storage  storage.Config
	Upload     UploadConf
	MQ         mqx.ProducerConfig
	Outbox     outboxx.Config
}

// UploadConf 上传相关阈值与路径。
type UploadConf struct {
	MaxImageSize      int64
	MaxVideoSize      int64
	DefaultQuality    int
	ThumbnailLongSide int
	TempDir           string
}
