package logic

import (
	"context"
	"errors"
	"errx"

	"esx/app/media/internal/model"
	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

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

	l.Infow("delete media success",
		logx.Field("media_id", in.MediaId),
		logx.Field("user_id", in.UserId),
	)
	return &pb.DeleteMediaResp{}, nil
}
