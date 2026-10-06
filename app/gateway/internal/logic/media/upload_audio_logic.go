package media

import (
	"context"
	"esx/app/gateway/internal/logic/mediaupload"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"io"
)

// UploadAudioLogic 承载音频上传；每个请求新建一个实例。
type UploadAudioLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUploadAudioLogic 绑定请求上下文与服务依赖。
func NewUploadAudioLogic(ctx context.Context, s *svc.ServiceContext) *UploadAudioLogic {
	return &UploadAudioLogic{ctx: ctx, svcCtx: s}
}

// UploadAudio 是生成路由的占位入口；上传必须走流式 multipart 的 UploadMultipart。
func (l *UploadAudioLogic) UploadAudio(_ *types.UploadMediaReq) (*types.UploadMediaResp, error) {
	return nil, errx.NewWithCode(errx.ParamError)
}

// UploadMultipart 把已绑定的 multipart 文件流式转发给媒体服务。
func (l *UploadAudioLogic) UploadMultipart(file io.Reader, filename, key string) (*types.UploadMediaResp, error) {
	return mediaupload.Upload(l.ctx, l.svcCtx, file, filename, key, true)
}
