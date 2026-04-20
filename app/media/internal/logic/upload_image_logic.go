package logic

import (
	"context"
	"errx"
	"os"

	"esx/app/media/internal/mediautil"
	"esx/app/media/internal/model"
	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UploadImageLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUploadImageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadImageLogic {
	return &UploadImageLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// UploadImage 接收 client streaming → 落盘 → 嗅探 → 压缩 → 缩略图 → 上传 → 入库 → SendAndClose。
func (l *UploadImageLogic) UploadImage(stream pb.MediaService_UploadImageServer) error {
	upload := l.svcCtx.Config.Upload
	sink, err := mediautil.NewTempSink(upload.TempDir, upload.MaxImageSize)
	if err != nil {
		l.Errorw("create temp sink failed", logx.Field("err", err.Error()))
		return errx.NewWithCode(errx.SystemError)
	}
	defer sink.Close()

	meta, err := receiveUploadStream(
		stream.Recv,
		func(r *pb.UploadImageReq) *pb.UploadMeta { return r.GetMeta() },
		func(r *pb.UploadImageReq) []byte { return r.GetChunk() },
		sink,
	)
	if err != nil {
		return err
	}
	if meta.GetUserId() <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}

	if _, err = mediautil.Detect(sink.Path(), true, false); err != nil {
		return errx.NewWithCode(errx.FileTypeNotAllowed)
	}

	quality := int(meta.GetQuality())
	if quality <= 0 || quality > 100 {
		quality = upload.DefaultQuality
	}

	compressedPath, width, height, err := mediautil.CompressImage(
		sink.Path(),
		int(meta.GetMaxWidth()),
		int(meta.GetMaxHeight()),
		quality,
	)
	if err != nil {
		l.Errorw("compress image failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("file_name", meta.GetFileName()),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.MediaProcessFailed)
	}
	defer os.Remove(compressedPath)

	thumbPath, err := mediautil.MakeThumbnail(sink.Path())
	if err != nil {
		l.Errorw("make thumbnail failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("file_name", meta.GetFileName()),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.MediaProcessFailed)
	}
	defer os.Remove(thumbPath)

	objKey := buildObjectKey("original", "jpg")
	thumbKey := buildObjectKey("thumb", "jpg")

	if err = putFile(l.ctx, l.svcCtx, compressedPath, objKey, "image/jpeg"); err != nil {
		l.Errorw("put original failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("object_key", objKey),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.UploadFailed)
	}
	if err = putFile(l.ctx, l.svcCtx, thumbPath, thumbKey, "image/jpeg"); err != nil {
		l.Errorw("put thumbnail failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("object_key", thumbKey),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.UploadFailed)
	}

	info, err := os.Stat(compressedPath)
	if err != nil {
		return errx.NewWithCode(errx.SystemError)
	}

	row := &model.Media{
		UserId:       meta.GetUserId(),
		FileName:     meta.GetFileName(),
		OriginalName: nullStringOr(meta.GetFileName()),
		FileType:     "image",
		MimeType:     nullStringOr("image/jpeg"),
		Url:          l.svcCtx.Storage.BuildPublicURL(objKey),
		ThumbnailUrl: nullStringOr(l.svcCtx.Storage.BuildPublicURL(thumbKey)),
		StorageType:  storageTypeSeaweedFS,
		Bucket:       nullStringOr(l.svcCtx.Config.S3Storage.Bucket),
		ObjectKey:    nullStringOr(objKey),
		FileSize:     info.Size(),
		Width:        nullInt(width),
		Height:       nullInt(height),
		Status:       1,
	}
	res, err := l.svcCtx.MediaModel.Insert(l.ctx, row)
	if err != nil {
		l.Errorw("insert media row failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("object_key", objKey),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.SystemError)
	}
	id, err := res.LastInsertId()
	if err != nil {
		l.Errorw("LastInsertId failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("object_key", objKey),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.SystemError)
	}
	row.Id = id

	l.Infow("upload image success",
		logx.Field("media_id", id),
		logx.Field("user_id", meta.GetUserId()),
		logx.Field("file_size", info.Size()),
		logx.Field("object_key", objKey),
	)
	return stream.SendAndClose(&pb.UploadImageResp{Media: toPBMediaInfo(row)})
}

// putFile 从本地文件读取并流式上传到 Storage。
func putFile(ctx context.Context, svcCtx *svc.ServiceContext, localPath, objectKey, contentType string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return svcCtx.Storage.Put(ctx, objectKey, f, info.Size(), contentType)
}
