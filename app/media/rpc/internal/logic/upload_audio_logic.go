package logic

import (
	"context"

	"esx/app/media/rpc/internal/svc"
	pb2 "esx/kitex_gen/media"

	logx "esx/pkg/logging"
)

type UploadAudioLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUploadAudioLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadAudioLogic {
	return &UploadAudioLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
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
