package logic

import (
	"context"
	"errors"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/util"
	"strconv"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

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

// buildMediaDeletedOutboxEvent 构造与 media_cleanup_consumer 解析一致的
// media-deleted 事件载荷，EventID 同时作为 outbox 幂等键。
func buildMediaDeletedOutboxEvent(mediaID int64, objectKey, bucket string) (outboxx.Event, error) {
	eventID, err := util.NextID()
	if err != nil {
		return outboxx.Event{}, err
	}
	e := event.MediaDeletedEvent{
		EventID:     eventID,
		EventTime:   time.Now().UnixMilli(),
		MediaID:     mediaID,
		S3ObjectKey: objectKey,
		Bucket:      bucket,
		DeletedAt:   time.Now().Unix(),
	}
	if err := e.Validate(); err != nil {
		return outboxx.Event{}, err
	}
	body, err := e.MarshalPayload()
	if err != nil {
		return outboxx.Event{}, err
	}
	return outboxx.Event{
		ID: eventID, Topic: mqx.TopicMediaDelete, Tag: mqx.TagDefault,
		Key: strconv.FormatInt(eventID, 10), Payload: body,
	}, nil
}

// DeleteMedia 软删媒体；仅归属用户可删；重复删除幂等。
// 软删与 media-deleted 事件同事务写入 outbox，由 relay 可靠投递，避免
// 提交后进程崩溃导致 S3 清理事件丢失（孤儿对象）。
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

	// 无 S3 对象（如直接写入的行）无需清理：仅软删，不投递事件。
	var event outboxx.Event
	if m.ObjectKey.Valid && strings.TrimSpace(m.ObjectKey.String) != "" {
		event, err = buildMediaDeletedOutboxEvent(in.MediaId, m.ObjectKey.String, l.svcCtx.Config.S3Storage.Bucket)
		if err != nil {
			l.Errorw("build media_deleted outbox event failed",
				logx.Field("media_id", in.MediaId),
				logx.Field("err", err.Error()),
			)
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}
	if err := l.svcCtx.MediaCommandModel.SoftDelete(l.ctx, in.MediaId, event); err != nil {
		l.Errorw("MediaCommandModel.SoftDelete failed",
			logx.Field("media_id", in.MediaId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// 事务提交后失效读缓存（原 UpdateStatus 的 ExecCtx 语义），避免陈旧状态。
	if err := l.svcCtx.MediaModel.DelCache(l.ctx, in.MediaId); err != nil {
		l.Errorw("MediaModel.DelCache failed",
			logx.Field("media_id", in.MediaId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	l.Infow("delete media success",
		logx.Field("media_id", in.MediaId),
		logx.Field("user_id", in.UserId),
	)
	return &pb.DeleteMediaResp{}, nil
}
