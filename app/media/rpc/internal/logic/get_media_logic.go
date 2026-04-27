package logic

import (
	"context"
	"database/sql"
	"errors"
	"errx"

	"esx/app/media/internal/model"
	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMediaLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMediaLogic {
	return &GetMediaLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetMedia 获取媒体信息
func (l *GetMediaLogic) GetMedia(in *pb.GetMediaReq) (*pb.GetMediaResp, error) {
	if in.MediaId <= 0 {
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
	if m.Status != 1 {
		return nil, errx.NewWithCode(errx.MediaNotFound)
	}

	return &pb.GetMediaResp{Media: toPBMediaInfo(m)}, nil
}

// toPBMediaInfo 把 DB 模型转 pb.MediaInfo（多处复用）。
func toPBMediaInfo(m *model.Media) *pb.MediaInfo {
	nullStr := func(s sql.NullString) string {
		if s.Valid {
			return s.String
		}
		return ""
	}
	nullInt := func(s sql.NullInt64) int32 {
		if s.Valid {
			return int32(s.Int64)
		}
		return 0
	}
	return &pb.MediaInfo{
		Id:           m.Id,
		UserId:       m.UserId,
		FileName:     m.FileName,
		FileType:     m.FileType,
		Url:          m.Url,
		ThumbnailUrl: nullStr(m.ThumbnailUrl),
		FileSize:     m.FileSize,
		Width:        nullInt(m.Width),
		Height:       nullInt(m.Height),
		Duration:     nullInt(m.Duration),
		Status:       int32(m.Status),
		CreatedAt:    m.CreatedAt.UnixMilli(),
	}
}
