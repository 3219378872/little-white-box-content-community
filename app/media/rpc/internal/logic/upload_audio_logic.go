package logic

import (
	"context"

	"esx/app/media/rpc/internal/svc"
	pb2 "esx/kitex_gen/media"

	"esx/pkg/logging"
)

// UploadAudioLogic 承载 UploadAudio 接口的业务逻辑；每个请求新建一个实例。
type UploadAudioLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewUploadAudioLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewUploadAudioLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadAudioLogic {
	return &UploadAudioLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// UploadAudio 接收 client streaming → 落盘 → 嗅探 → 直传（不转码/截图） → 入库 → SendAndClose。
func (l *UploadAudioLogic) UploadAudio(stream pb2.MediaService_UploadAudioServer) error {
	upload, err := receiveUpload(l.ctx, l.Logger, l.svcCtx, uploadSpec{
		kind:    "audio",
		maxSize: l.svcCtx.Config.Upload.MaxAudioSize,
		// Preserve existing image/video fingerprints; audio uses a separate namespace.
		idempotencyScope: "media:upload:audio",
	}, stream.Recv,
		func(r *pb2.UploadAudioReq) *pb2.UploadMeta { return r.GetMeta() },
		func(r *pb2.UploadAudioReq) []byte { return r.GetChunk() },
	)
	if err != nil {
		return err
	}
	defer upload.Close(l.Logger, "audio")

	media, err := storeAVUpload(l.ctx, l.Logger, l.svcCtx, audioUpload, upload)
	if err != nil {
		return err
	}
	return stream.SendAndClose(&pb2.UploadAudioResp{Media: toPBMediaInfo(media)})
}
