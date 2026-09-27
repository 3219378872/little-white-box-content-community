package media

import (
	"context"
	"esx/app/gateway/internal/logic/mediaupload"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"io"
)

type UploadVideoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUploadVideoLogic(ctx context.Context, s *svc.ServiceContext) *UploadVideoLogic {
	return &UploadVideoLogic{ctx: ctx, svcCtx: s}
}
func (l *UploadVideoLogic) UploadVideo(_ *types.UploadMediaReq) (*types.UploadMediaResp, error) {
	return nil, errx.NewWithCode(errx.ParamError)
}
func (l *UploadVideoLogic) UploadMultipart(file io.Reader, filename, key string) (*types.UploadMediaResp, error) {
	return mediaupload.Upload(l.ctx, l.svcCtx, file, filename, key, false)
}
