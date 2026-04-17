package logic

import (
	"context"

	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UploadVideoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUploadVideoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadVideoLogic {
	return &UploadVideoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 上传视频（client streaming，每包 ≤ 1MB）
func (l *UploadVideoLogic) UploadVideo(stream pb.MediaService_UploadVideoServer) error {
	// todo: add your logic here and delete this line

	return nil
}
