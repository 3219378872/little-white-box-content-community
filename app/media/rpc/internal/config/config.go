package config

import (
	"esx/app/media/rpc/internal/storage"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"

	"esx/pkg/rpcx"
)

type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	DataSource     string
	S3Storage      storage.Config
	Upload         UploadConf
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
}

// UploadConf 上传相关阈值与路径。
type UploadConf struct {
	MaxImageSize      int64
	MaxVideoSize      int64
	MaxAudioSize      int64 `json:",default=10485760"`
	DefaultQuality    int
	ThumbnailLongSide int
	TempDir           string
}
