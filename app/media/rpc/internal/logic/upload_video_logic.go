package logic

import (
	"context"
	"errx"
	mediautil2 "esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	pb2 "esx/app/media/rpc/pb/xiaobaihe/media/pb"

	"cleanupx"
	"util"

	"github.com/zeromicro/go-zero/core/logx"
)

type UploadVideoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUploadVideoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadVideoLogic {
	return &UploadVideoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// UploadVideo 接收 client streaming → 落盘 → 嗅探 → 直传（不转码/截图） → 入库 → SendAndClose。
func (l *UploadVideoLogic) UploadVideo(stream pb2.MediaService_UploadVideoServer) error {
	upload := l.svcCtx.Config.Upload
	sink, err := mediautil2.NewTempSink(upload.TempDir, upload.MaxVideoSize)
	if err != nil {
		l.Errorw("create temp sink failed", logx.Field("err", err.Error()))
		return errx.NewWithCode(errx.SystemError)
	}
	defer cleanupx.Close(l.Logger, "upload video temp sink", sink)

	meta, err := receiveUploadStream(
		stream.Recv,
		func(r *pb2.UploadVideoReq) *pb2.UploadMeta { return r.GetMeta() },
		func(r *pb2.UploadVideoReq) []byte { return r.GetChunk() },
		sink,
	)
	if err != nil {
		return err
	}
	if meta.GetUserId() <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}

	detected, err := mediautil2.Detect(sink.Path(), false, true)
	if err != nil {
		return errx.NewWithCode(errx.FileTypeNotAllowed)
	}

	objKey := buildObjectKey("original", detected.Ext)
	if err = putFile(l.ctx, l.svcCtx, sink.Path(), objKey, detected.MIME); err != nil {
		l.Errorw("put video failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("object_key", objKey),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.UploadFailed)
	}

	mediaId, err := util.NextID()
	if err != nil {
		l.Errorw("NextID failed", logx.Field("err", err.Error()))
		return errx.NewWithCode(errx.SystemError)
	}

	row := &model.Media{
		Id:           mediaId,
		UserId:       meta.GetUserId(),
		FileName:     meta.GetFileName(),
		OriginalName: nullStringOr(meta.GetFileName()),
		FileType:     "video",
		MimeType:     nullStringOr(detected.MIME),
		Url:          l.svcCtx.Storage.BuildPublicURL(objKey),
		StorageType:  storageTypeSeaweedFS,
		Bucket:       nullStringOr(l.svcCtx.Config.S3Storage.Bucket),
		ObjectKey:    nullStringOr(objKey),
		FileSize:     sink.Size(),
		Status:       1,
	}
	_, err = l.svcCtx.MediaModel.Insert(l.ctx, row)
	if err != nil {
		l.Errorw("insert media row failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("object_key", objKey),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.SystemError)
	}

	l.Infow("upload video success",
		logx.Field("media_id", mediaId),
		logx.Field("user_id", meta.GetUserId()),
		logx.Field("file_size", sink.Size()),
		logx.Field("object_key", objKey),
	)
	return stream.SendAndClose(&pb2.UploadVideoResp{Media: toPBMediaInfo(row)})
}
