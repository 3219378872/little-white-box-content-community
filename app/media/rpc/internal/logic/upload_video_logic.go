package logic

import (
	"context"

	"esx/app/media/rpc/internal/svc"
	pb2 "esx/kitex_gen/media"

	"esx/pkg/logging"
)

// UploadVideoLogic 承载 UploadVideo 接口的业务逻辑；每个请求新建一个实例。
type UploadVideoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewUploadVideoLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewUploadVideoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadVideoLogic {
	return &UploadVideoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// UploadVideo 接收 client streaming → 落盘 → 嗅探 → 直传（不转码/截图） → 入库 → SendAndClose。
func (l *UploadVideoLogic) UploadVideo(stream pb2.MediaService_UploadVideoServer) error {
	upload, err := receiveUpload(l.ctx, l.Logger, l.svcCtx, uploadSpec{
		kind:    "video",
		maxSize: l.svcCtx.Config.Upload.MaxVideoSize,
	}, stream.Recv,
		func(r *pb2.UploadVideoReq) *pb2.UploadMeta { return r.GetMeta() },
		func(r *pb2.UploadVideoReq) []byte { return r.GetChunk() },
	)
	if err != nil {
		return err
	}
	defer upload.Close(l.Logger, "video")

	media, err := storeAVUpload(l.ctx, l.Logger, l.svcCtx, videoUpload, upload)
	if err != nil {
		return err
	}
	return stream.SendAndClose(&pb2.UploadVideoResp{Media: toPBMediaInfo(media)})
}
