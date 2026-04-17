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
		l.Errorf("MediaModel.FindOne(%d) failed: %v", in.MediaId, err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if m.Status == 0 {
		return &pb.DeleteMediaResp{}, nil
	}

	if m.UserId != in.UserId {
		return nil, errx.NewWithCode(errx.PermissionDenied)
	}

	m.Status = 0
	if err = l.svcCtx.MediaModel.Update(l.ctx, m); err != nil {
		l.Errorf("MediaModel.Update(%d) failed: %v", in.MediaId, err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &pb.DeleteMediaResp{}, nil
}
