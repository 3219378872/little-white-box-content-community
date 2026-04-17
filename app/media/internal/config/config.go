package config

import (
	"esx/app/media/internal/storage"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DataSource string
	Redis      RedisConf
	S3Storage  storage.Config
	Upload     UploadConf
}

// RedisConf 与 content 模块保持一致的嵌套结构。
type RedisConf struct {
	redis.RedisConf
	Key string
}

// UploadConf 上传相关阈值与路径。
type UploadConf struct {
	MaxImageSize      int64
	MaxVideoSize      int64
	DefaultQuality    int
	ThumbnailLongSide int
	TempDir           string
}
