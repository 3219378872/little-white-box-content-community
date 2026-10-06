package logic

import (
	"context"

	"esx/app/media/rpc/internal/mediautil"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	pb "esx/kitex_gen/media"
	"esx/pkg/cleanupx"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"esx/pkg/util"

	logx "esx/pkg/logging"
)

// receivedUpload 是已完整落盘、算好内容指纹并绑定幂等记录的上传，
// 后续只剩各媒体类型自己的处理（嗅探、压缩、直传）。
type receivedUpload struct {
	sink *mediautil.TempSink
	meta *pb.UploadMeta
	idem idempotencyx.IdempotencyRecord
}

// Close 删除临时文件；调用方拿到 receivedUpload 后负责 defer 调用。
func (u *receivedUpload) Close(logger logx.Logger, kind string) {
	cleanupx.Close(logger, "upload "+kind+" temp sink", u.sink)
}

// uploadSpec 描述一次上传的接收约束。
type uploadSpec struct {
	// kind 用于日志与临时文件说明：image / video / audio。
	kind string
	// maxSize 是落盘上限，超出即返回 FileTooLarge。
	maxSize int64
	// idempotencyScope 非空时覆盖默认的 "media:upload" 命名空间。
	idempotencyScope string
}

// receiveUpload 执行三类上传共同的前半段：
// 流式接收到限额临时文件 → 校验用户 → 计算内容指纹 → 构造并校验幂等记录（CORE-050/051）。
// 失败时已自行清理临时文件；成功时由调用方 Close。
func receiveUpload[Req any](
	ctx context.Context,
	logger logx.Logger,
	svcCtx *svc.ServiceContext,
	spec uploadSpec,
	recv func() (*Req, error),
	getMeta func(*Req) *pb.UploadMeta,
	getChunk func(*Req) []byte,
) (_ *receivedUpload, err error) {
	sink, err := mediautil.NewTempSink(svcCtx.Config.Upload.TempDir, spec.maxSize)
	if err != nil {
		logger.Errorw("create temp sink failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	upload := &receivedUpload{sink: sink}
	defer func() {
		if err != nil {
			upload.Close(logger, spec.kind)
		}
	}()

	// 首包是元数据，其余是文件分片。
	meta, err := receiveUploadStream(recv, getMeta, getChunk, sink)
	if err != nil {
		return nil, err
	}
	if meta.GetUserId() <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	upload.meta = meta

	// 同一幂等键 + 不同字节 = 不同命令，所以指纹必须基于实际收到的内容。
	contentHash, err := sha256File(ctx, sink.Path())
	if err != nil {
		logger.Errorw("hash uploaded "+spec.kind+" failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("file_name", meta.GetFileName()),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	upload.idem = mediaIdempotencyRecord(meta, contentHash)
	if spec.idempotencyScope != "" {
		upload.idem.Scope = spec.idempotencyScope
	}
	if !upload.idem.Valid() {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	// 没有命令模型就无法保证幂等落库，拒绝而不是先写对象存储。
	if svcCtx.MediaCommandModel == nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return upload, nil
}

// avUploadKind 区分音频与视频这两种「不转码、不截图、原样直传」的媒体。
type avUploadKind struct {
	fileType string // 写入 media.file_type
	audio    bool   // DetectAV 按音频还是视频容器嗅探
}

var (
	audioUpload = avUploadKind{fileType: "audio", audio: true}
	videoUpload = avUploadKind{fileType: "video", audio: false}
)

// storeAVUpload 嗅探音视频容器 → 原样写入对象存储 → 幂等入库，返回应回给客户端的媒体。
// 入库判定为重放（已有记录）时，本次上传的对象会被补偿删除，返回已有记录。
func storeAVUpload(
	ctx context.Context,
	logger logx.Logger,
	svcCtx *svc.ServiceContext,
	kind avUploadKind,
	upload *receivedUpload,
) (*model.Media, error) {
	meta := upload.meta
	detected, err := mediautil.DetectAV(upload.sink.Path(), kind.audio)
	if err != nil {
		return nil, errx.NewWithCode(errx.FileTypeNotAllowed)
	}

	// 对象一旦写入，除非最终确认保留，否则在返回前补偿删除。
	objKey := buildObjectKey("original", detected.Ext)
	keepUploadedObject := false
	uploaded := false
	defer func() {
		if uploaded && !keepUploadedObject {
			compensateUploadedObjects(ctx, logger, svcCtx, objKey)
		}
	}()
	if err = putFile(ctx, svcCtx, upload.sink.Path(), objKey, detected.MIME); err != nil {
		logger.Errorw("put "+kind.fileType+" failed",
			logx.Field("user_id", meta.GetUserId()),
			logx.Field("object_key", objKey),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.UploadFailed)
	}
	uploaded = true

	mediaId, err := util.NextID()
	if err != nil {
		logger.Errorw("NextID failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	row := &model.Media{
		Id:           mediaId,
		UserId:       meta.GetUserId(),
		FileName:     meta.GetFileName(),
		OriginalName: nullStringOr(meta.GetFileName()),
		FileType:     kind.fileType,
		MimeType:     nullStringOr(detected.MIME),
		Url:          svcCtx.Storage.BuildPublicURL(objKey),
		StorageType:  storageTypeSeaweedFS,
		Bucket:       nullStringOr(svcCtx.Config.S3Storage.Bucket),
		ObjectKey:    nullStringOr(objKey),
		FileSize:     upload.sink.Size(),
		Status:       1,
	}

	// keepObjects=false 表示幂等重放命中已有记录：返回已有媒体，删除本次对象。
	stored, keepObjects, err := persistUploadedMedia(ctx, logger, svcCtx, row, upload.idem)
	keepUploadedObject = keepObjects
	if err != nil {
		return nil, err
	}
	if !keepObjects {
		return stored, nil
	}
	logger.Infow("upload "+kind.fileType+" success",
		logx.Field("media_id", mediaId),
		logx.Field("user_id", meta.GetUserId()),
		logx.Field("file_size", upload.sink.Size()),
		logx.Field("object_key", objKey),
	)
	return row, nil
}
