package logic

import (
	"context"
	"errx"

	"esx/app/media/internal/mediautil"
	"esx/app/media/internal/model"
	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

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
func (l *UploadVideoLogic) UploadVideo(stream pb.MediaService_UploadVideoServer) error {
	upload := l.svcCtx.Config.Upload
	sink, err := mediautil.NewTempSink(upload.TempDir, upload.MaxVideoSize)
	if err != nil {
		l.Errorf("create temp sink: %v", err)
		return errx.NewWithCode(errx.SystemError)
	}
	defer sink.Close()

	meta, err := receiveUploadStream(
		stream.Recv,
		func(r *pb.UploadVideoReq) *pb.UploadMeta { return r.GetMeta() },
		func(r *pb.UploadVideoReq) []byte { return r.GetChunk() },
		sink,
	)
	if err != nil {
		return err
	}
	if meta.GetUserId() <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}

	detected, err := mediautil.Detect(sink.Path(), false, true)
	if err != nil {
		return errx.NewWithCode(errx.FileTypeNotAllowed)
	}

	objKey := buildObjectKey("original", detected.Ext)
	if err = putFile(l.ctx, l.svcCtx, sink.Path(), objKey, detected.MIME); err != nil {
		l.Errorf("put video: %v", err)
		return errx.NewWithCode(errx.UploadFailed)
	}

	row := &model.Media{
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
	res, err := l.svcCtx.MediaModel.Insert(l.ctx, row)
	if err != nil {
		l.Errorf("insert media row: %v", err)
		return errx.NewWithCode(errx.SystemError)
	}
	id, _ := res.LastInsertId()
	row.Id = id

	return stream.SendAndClose(&pb.UploadVideoResp{Media: toPBMediaInfo(row)})
}
