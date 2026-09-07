package logic

import (
	"context"
	"errors"
	mediautil2 "esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	pb2 "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/cleanupx"
	"esx/pkg/errx"
	"esx/pkg/util"
	"github.com/zeromicro/go-zero/core/logx"
	"os"
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
func (l *UploadImageLogic) UploadImage(stream pb2.MediaService_UploadImageServer) error {
	upload := l.svcCtx.Config.Upload
	sink, err := mediautil2.NewTempSink(upload.TempDir, upload.MaxImageSize)
	if err != nil {
		l.Errorw("create temp sink failed", logx.Field("err", err.Error()))
		return errx.NewWithCode(errx.SystemError)
	}
	defer cleanupx.Close(l.Logger, "upload image temp sink", sink)

	meta, err := receiveUploadStream(
		stream.Recv,
		func(r *pb2.UploadImageReq) *pb2.UploadMeta { return r.GetMeta() },
		func(r *pb2.UploadImageReq) []byte { return r.GetChunk() },
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
		l.Errorw("hash uploaded image failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("file_name", meta.GetFileName()),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.SystemError)
	}
	idem := mediaIdempotencyRecord(meta, contentHash)
	if !idem.Valid() {
		return errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.MediaCommandModel == nil {
		return errx.NewWithCode(errx.SystemError)
	}

	if err := l.validateImage(sink.Path(), meta.GetUserId()); err != nil {
		return err
	}

	quality := int(meta.GetQuality())
	if quality <= 0 || quality > 100 {
		quality = upload.DefaultQuality
	}

	compressedPath, width, height, err := mediautil2.CompressImage(
		l.ctx,
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
	defer cleanupx.Remove(l.Logger, compressedPath)

	thumbPath, err := mediautil2.MakeThumbnail(l.ctx, sink.Path())
	if err != nil {
		l.Errorw("make thumbnail failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("file_name", meta.GetFileName()),
			logx.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.MediaProcessFailed)
	}
	defer cleanupx.Remove(l.Logger, thumbPath)

	objKey := buildObjectKey("original", "jpg")
	thumbKey := buildObjectKey("thumb", "jpg")
	uploadedKeys := make([]string, 0, 2)
	keepUploadedObjects := false
	defer func() {
		if !keepUploadedObjects {
			compensateUploadedObjects(l.ctx, l.Logger, l.svcCtx, uploadedKeys...)
		}
	}()

	uploadedKeys, err = l.putImageVariants(meta.GetUserId(), compressedPath, thumbPath, objKey, thumbKey)
	if err != nil {
		return err
	}

	row, err := l.imageRecord(meta, compressedPath, objKey, thumbKey, width, height)
	if err != nil {
		return err
	}

	stored, created, err := persistUploadedMedia(l.ctx, l.Logger, l.svcCtx, row, idem)
	if err != nil {
		return err
	}
	if !created {
		return stream.SendAndClose(&pb2.UploadImageResp{Media: toPBMediaInfo(stored)})
	}
	keepUploadedObjects = true

	l.Infow("upload image success",
		logx.Field("media_id", row.Id),
		logx.Field("user_id", meta.GetUserId()),
		logx.Field("file_size", row.FileSize),
		logx.Field("object_key", objKey),
	)
	return stream.SendAndClose(&pb2.UploadImageResp{Media: toPBMediaInfo(row)})
}

// putFile 从本地文件读取并流式上传到 Storage。
func putFile(ctx context.Context, svcCtx *svc.ServiceContext, localPath, objectKey, contentType string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer cleanupx.Close(logx.WithContext(ctx), "upload source file", f)
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return svcCtx.Storage.Put(ctx, objectKey, f, info.Size(), contentType)
}

func (l *UploadImageLogic) imageRecord(meta *pb2.UploadMeta, compressedPath, objKey, thumbKey string, width, height int) (*model.Media, error) {
	info, err := os.Stat(compressedPath)
	if err != nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}

	mediaId, err := util.NextID()
	if err != nil {
		l.Errorw("NextID failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	row := &model.Media{
		Id:                 mediaId,
		UserId:             meta.GetUserId(),
		FileName:           meta.GetFileName(),
		OriginalName:       nullStringOr(meta.GetFileName()),
		FileType:           "image",
		MimeType:           nullStringOr("image/jpeg"),
		Url:                l.svcCtx.Storage.BuildPublicURL(objKey),
		ThumbnailUrl:       nullStringOr(l.svcCtx.Storage.BuildPublicURL(thumbKey)),
		StorageType:        storageTypeSeaweedFS,
		Bucket:             nullStringOr(l.svcCtx.Config.S3Storage.Bucket),
		ObjectKey:          nullStringOr(objKey),
		ThumbnailObjectKey: nullStringOr(thumbKey),
		FileSize:           info.Size(),
		Width:              nullInt(width),
		Height:             nullInt(height),
		Status:             1,
	}
	return row, nil
}

func (l *UploadImageLogic) validateImage(path string, userID int64) error {
	var err error
	if _, err = mediautil2.Detect(path, true, false); err != nil {
		return errx.NewWithCode(errx.FileTypeNotAllowed)
	}
	if _, _, err = mediautil2.ValidateImageDimensions(path); err != nil {
		if errors.Is(err, mediautil2.ErrImageDimensionsExceeded) {
			return errx.NewWithCode(errx.FileTooLarge)
		}
		l.Errorw("decode image dimensions failed",
			logx.Field("user_id", userID), logx.Field("err", err.Error()))
		return errx.NewWithCode(errx.MediaProcessFailed)
	}

	return nil
}

func (l *UploadImageLogic) putImageVariants(userID int64, compressedPath, thumbPath, objKey, thumbKey string) ([]string, error) {
	uploadedKeys := make([]string, 0, 2)
	var err error
	if err = putFile(l.ctx, l.svcCtx, compressedPath, objKey, "image/jpeg"); err != nil {
		l.Errorw("put original failed",
			logx.Field("user_id", userID),
			logx.Field("object_key", objKey),
			logx.Field("err", err.Error()),
		)
		return uploadedKeys, errx.NewWithCode(errx.UploadFailed)
	}
	uploadedKeys = append(uploadedKeys, objKey)
	if err = putFile(l.ctx, l.svcCtx, thumbPath, thumbKey, "image/jpeg"); err != nil {
		l.Errorw("put thumbnail failed",
			logx.Field("user_id", userID),
			logx.Field("object_key", thumbKey),
			logx.Field("err", err.Error()),
		)
		return uploadedKeys, errx.NewWithCode(errx.UploadFailed)
	}
	uploadedKeys = append(uploadedKeys, thumbKey)

	return uploadedKeys, nil
}
