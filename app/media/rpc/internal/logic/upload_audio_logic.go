package logic

import (
	"context"
	mediautil2 "esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	pb2 "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/cleanupx"
	"esx/pkg/errx"
	"esx/pkg/util"

	"github.com/zeromicro/go-zero/core/logx"
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
	upload := l.svcCtx.Config.Upload
	sink, err := mediautil2.NewTempSink(upload.TempDir, upload.MaxAudioSize)
	if err != nil {
		l.Errorw("create temp sink failed", logx.Field("err", err.Error()))
		return errx.NewWithCode(errx.SystemError)
	}
	defer cleanupx.Close(l.Logger, "upload audio temp sink", sink)

	meta, err := receiveUploadStream(
		stream.Recv,
		func(r *pb2.UploadAudioReq) *pb2.UploadMeta { return r.GetMeta() },
		func(r *pb2.UploadAudioReq) []byte { return r.GetChunk() },
		sink,
	)
	if err != nil {
		return err
	}
	if meta.GetUserId() <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	contentHash, err := sha256File(l.ctx, sink.Path())
	if err != nil {
		l.Errorw("hash uploaded audio failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("file_name", meta.GetFileName()),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.SystemError)
	}
	idem := mediaIdempotencyRecord(meta, contentHash)
	// Preserve existing image/video fingerprints; audio uses a separate namespace.
	idem.Scope = "media:upload:audio"
	if !idem.Valid() {
		return errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.MediaCommandModel == nil {
		return errx.NewWithCode(errx.SystemError)
	}

	detected, err := mediautil2.DetectAV(sink.Path(), true)
	if err != nil {
		return errx.NewWithCode(errx.FileTypeNotAllowed)
	}

	objKey := buildObjectKey("original", detected.Ext)
	keepUploadedObject := false
	uploaded := false
	defer func() {
		if uploaded && !keepUploadedObject {
			compensateUploadedObjects(l.ctx, l.Logger, l.svcCtx, objKey)
		}
	}()
	if err = putFile(l.ctx, l.svcCtx, sink.Path(), objKey, detected.MIME); err != nil {
		l.Errorw("put audio failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("object_key", objKey),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.UploadFailed)
	}
	uploaded = true

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
		FileType:     "audio",
		MimeType:     nullStringOr(detected.MIME),
		Url:          l.svcCtx.Storage.BuildPublicURL(objKey),
		StorageType:  storageTypeSeaweedFS,
		Bucket:       nullStringOr(l.svcCtx.Config.S3Storage.Bucket),
		ObjectKey:    nullStringOr(objKey),
		FileSize:     sink.Size(),
		Status:       1,
	}
	stored, created, err := persistUploadedMedia(l.ctx, l.Logger, l.svcCtx, row, idem)
	if err != nil {
		return err
	}
	if !created {
		return stream.SendAndClose(&pb2.UploadAudioResp{Media: toPBMediaInfo(stored)})
	}
	keepUploadedObject = true

	l.Infow("upload audio success",
		logx.Field("media_id", mediaId),
		logx.Field("user_id", meta.GetUserId()),
		logx.Field("file_size", sink.Size()),
		logx.Field("object_key", objKey),
	)
	return stream.SendAndClose(&pb2.UploadAudioResp{Media: toPBMediaInfo(row)})
}
