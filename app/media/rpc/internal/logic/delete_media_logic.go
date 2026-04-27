package logic

import (
	"context"
	"encoding/json"
	"errors"
	"errx"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"time"

	"mqx"

	"github.com/zeromicro/go-zero/core/logx"
)

type mediaDeletedMessage struct {
	MediaId     int64  `json:"media_id"`
	S3ObjectKey string `json:"s3_object_key"`
	Bucket      string `json:"bucket"`
	DeletedAt   int64  `json:"deleted_at"`
}

type DeleteMediaLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeleteMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteMediaLogic {
	return &DeleteMediaLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// DeleteMedia 软删媒体；仅归属用户可删；重复删除幂等。
func (l *DeleteMediaLogic) DeleteMedia(in *pb.DeleteMediaReq) (*pb.DeleteMediaResp, error) {
	if in.MediaId <= 0 || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	m, err := l.svcCtx.MediaModel.FindOne(l.ctx, in.MediaId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.MediaNotFound)
		}
		l.Errorw("MediaModel.FindOne failed",
			logx.Field("media_id", in.MediaId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if m.Status == 0 {
		return &pb.DeleteMediaResp{}, nil
	}

	if m.UserId != in.UserId {
		return nil, errx.NewWithCode(errx.PermissionDenied)
	}

	result, err := l.svcCtx.MediaModel.UpdateStatus(l.ctx, in.MediaId, 1, 0)
	if err != nil {
		l.Errorw("MediaModel.UpdateStatus failed",
			logx.Field("media_id", in.MediaId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// 可能是并发重复删除，幂等返回成功
		l.Infow("delete media no-op (concurrent or already deleted)",
			logx.Field("media_id", in.MediaId),
		)
	}

	// 投递异步清理事件
	if l.svcCtx.MQProducer != nil {
		msg := mediaDeletedMessage{
			MediaId:     in.MediaId,
			S3ObjectKey: m.ObjectKey.String,
			Bucket:      l.svcCtx.Config.S3Storage.Bucket,
			DeletedAt:   time.Now().Unix(),
		}
		body, _ := json.Marshal(msg)
		if err := l.svcCtx.MQProducer.SendOneWay(l.ctx, mqx.TopicMediaDelete, body); err != nil {
			l.Errorw("send media_deleted event failed",
				logx.Field("media_id", in.MediaId),
				logx.Field("err", err.Error()),
			)
		}
	}

	l.Infow("delete media success",
		logx.Field("media_id", in.MediaId),
		logx.Field("user_id", in.UserId),
	)
	return &pb.DeleteMediaResp{}, nil
}
