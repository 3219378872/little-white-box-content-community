package media

import (
	"context"
	"esx/app/gateway/internal/logic/mediaupload"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"io"
)

type UploadAudioLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUploadAudioLogic(ctx context.Context, s *svc.ServiceContext) *UploadAudioLogic {
	return &UploadAudioLogic{ctx: ctx, svcCtx: s}
}
func (l *UploadAudioLogic) UploadAudio(_ *types.UploadMediaReq) (*types.UploadMediaResp, error) {
	return nil, errx.NewWithCode(errx.ParamError)
}
func (l *UploadAudioLogic) UploadMultipart(file io.Reader, filename, key string) (*types.UploadMediaResp, error) {
	return mediaupload.Upload(l.ctx, l.svcCtx, file, filename, key, true)
}
