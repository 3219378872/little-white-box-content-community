package logic

import (
	"context"

	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/logx"
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

// 上传图片（client streaming，每包 ≤ 1MB）
func (l *UploadImageLogic) UploadImage(stream pb.MediaService_UploadImageServer) error {
	// todo: add your logic here and delete this line

	return nil
}
