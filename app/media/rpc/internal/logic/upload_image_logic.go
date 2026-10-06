package logic

import (
	"context"
	"errors"
	mediautil2 "esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	pb2 "esx/kitex_gen/media"
	"esx/pkg/cleanupx"
	"esx/pkg/errx"
	"esx/pkg/util"
	"os"

	"esx/pkg/logging"
)

type UploadImageLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewUploadImageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadImageLogic {
	return &UploadImageLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// UploadImage 接收 client streaming → 落盘 → 嗅探 → 压缩 → 缩略图 → 上传 → 入库 → SendAndClose。
func (l *UploadImageLogic) UploadImage(stream pb2.MediaService_UploadImageServer) error {
	upload := l.svcCtx.Config.Upload
	received, err := receiveUpload(l.ctx, l.Logger, l.svcCtx, uploadSpec{
		kind:    "image",
		maxSize: upload.MaxImageSize,
	}, stream.Recv,
		func(r *pb2.UploadImageReq) *pb2.UploadMeta { return r.GetMeta() },
		func(r *pb2.UploadImageReq) []byte { return r.GetChunk() },
	)
	if err != nil {
		return err
	}
	defer received.Close(l.Logger, "image")
	sink, meta := received.sink, received.meta

	// 先确认是可解码、尺寸在限内的图片，再做任何耗时处理。
	if err := l.validateImage(sink.Path(), meta.GetUserId()); err != nil {
		return err
	}

	// 原图压缩为 JPEG（质量缺省或越界时用配置值）并另出缩略图。
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
		if errors.Is(err, mediautil2.ErrImageUndecodable) {
			return errx.NewWithCode(errx.FileTypeNotAllowed)
		}
		l.Errorw("compress image failed",
			logging.Field("user_id", meta.GetUserId()),
			logging.Field("file_name", meta.GetFileName()),
			logging.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.MediaProcessFailed)
	}
	defer cleanupx.Remove(l.Logger, compressedPath)

	thumbPath, err := mediautil2.MakeThumbnail(l.ctx, sink.Path())
	if err != nil {
		if errors.Is(err, mediautil2.ErrImageUndecodable) {
			return errx.NewWithCode(errx.FileTypeNotAllowed)
		}
		l.Errorw("make thumbnail failed",
			logging.Field("user_id", meta.GetUserId()),
			logging.Field("file_name", meta.GetFileName()),
			logging.Field("err", err.Error()),
		)
		return errx.NewWithCode(errx.MediaProcessFailed)
	}
	defer cleanupx.Remove(l.Logger, thumbPath)

	// 两个变体都写入对象存储；未最终保留时在返回前补偿删除已写入的对象。
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

	// keepObjects=false 表示幂等重放命中已有记录：返回已有媒体，删除本次对象。
	stored, keepObjects, err := persistUploadedMedia(l.ctx, l.Logger, l.svcCtx, row, received.idem)
	keepUploadedObjects = keepObjects
	if err != nil {
		return err
	}
	if !keepObjects {
		return stream.SendAndClose(&pb2.UploadImageResp{Media: toPBMediaInfo(stored)})
	}

	l.Infow("upload image success",
		logging.Field("media_id", row.Id),
		logging.Field("user_id", meta.GetUserId()),
		logging.Field("file_size", row.FileSize),
		logging.Field("object_key", objKey),
	)
	return stream.SendAndClose(&pb2.UploadImageResp{Media: toPBMediaInfo(row)})
}

// putFile 从本地文件读取并流式上传到 Storage。
func putFile(ctx context.Context, svcCtx *svc.ServiceContext, localPath, objectKey, contentType string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer cleanupx.Close(logging.WithContext(ctx), "upload source file", f)
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return svcCtx.Storage.Put(ctx, objectKey, f, info.Size(), contentType)
}

// imageRecord 构造压缩后原图与缩略图对应的媒体记录。
func (l *UploadImageLogic) imageRecord(meta *pb2.UploadMeta, compressedPath, objKey, thumbKey string, width, height int) (*model.Media, error) {
	info, err := os.Stat(compressedPath)
	if err != nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}

	mediaId, err := util.NextID()
	if err != nil {
		l.Errorw("NextID failed", logging.Field("err", err.Error()))
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

// validateImage 嗅探图片类型并在解码前校验像素尺寸，防止超大图片耗尽内存。
func (l *UploadImageLogic) validateImage(path string, userID int64) error {
	var err error
	if _, err = mediautil2.Detect(path, true, false); err != nil {
		return errx.NewWithCode(errx.FileTypeNotAllowed)
	}
	if _, _, err = mediautil2.ValidateImageDimensions(path); err != nil {
		if errors.Is(err, mediautil2.ErrImageDimensionsExceeded) {
			return errx.NewWithCode(errx.FileTooLarge)
		}
		if errors.Is(err, mediautil2.ErrImageUndecodable) {
			return errx.NewWithCode(errx.FileTypeNotAllowed)
		}
		l.Errorw("decode image dimensions failed",
			logging.Field("user_id", userID), logging.Field("err", err.Error()))
		return errx.NewWithCode(errx.MediaProcessFailed)
	}

	return nil
}

// putImageVariants 依次上传原图与缩略图，返回已成功写入、需要时可补偿的对象键。
func (l *UploadImageLogic) putImageVariants(userID int64, compressedPath, thumbPath, objKey, thumbKey string) ([]string, error) {
	uploadedKeys := make([]string, 0, 2)
	var err error
	if err = putFile(l.ctx, l.svcCtx, compressedPath, objKey, "image/jpeg"); err != nil {
		l.Errorw("put original failed",
			logging.Field("user_id", userID),
			logging.Field("object_key", objKey),
			logging.Field("err", err.Error()),
		)
		return uploadedKeys, errx.NewWithCode(errx.UploadFailed)
	}
	uploadedKeys = append(uploadedKeys, objKey)
	if err = putFile(l.ctx, l.svcCtx, thumbPath, thumbKey, "image/jpeg"); err != nil {
		l.Errorw("put thumbnail failed",
			logging.Field("user_id", userID),
			logging.Field("object_key", thumbKey),
			logging.Field("err", err.Error()),
		)
		return uploadedKeys, errx.NewWithCode(errx.UploadFailed)
	}
	uploadedKeys = append(uploadedKeys, thumbKey)

	return uploadedKeys, nil
}
