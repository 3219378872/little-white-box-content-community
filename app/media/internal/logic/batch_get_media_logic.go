package logic

import (
	"context"
	"errx"

	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

const maxBatchSize = 100

type BatchGetMediaLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchGetMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchGetMediaLogic {
	return &BatchGetMediaLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// BatchGetMedia 批量查询；部分 id 不存在或软删时静默跳过。
func (l *BatchGetMediaLogic) BatchGetMedia(in *pb.BatchGetMediaReq) (*pb.BatchGetMediaResp, error) {
	n := len(in.MediaIds)
	if n == 0 || n > maxBatchSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	for _, id := range in.MediaIds {
		if id <= 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}

	rows, err := l.svcCtx.MediaModel.FindByIds(l.ctx, in.MediaIds)
	if err != nil {
		l.Errorf("MediaModel.FindByIds failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	out := make([]*pb.MediaInfo, 0, len(rows))
	for _, m := range rows {
		if m.Status != 1 {
			continue
		}
		out = append(out, toPBMediaInfo(m))
	}
	return &pb.BatchGetMediaResp{Medias: out}, nil
}
