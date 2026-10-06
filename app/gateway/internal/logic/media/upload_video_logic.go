package media

import (
	"context"
	"esx/app/gateway/internal/logic/mediaupload"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"io"
)

// UploadVideoLogic 承载视频上传；每个请求新建一个实例。
type UploadVideoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUploadVideoLogic 绑定请求上下文与服务依赖。
func NewUploadVideoLogic(ctx context.Context, s *svc.ServiceContext) *UploadVideoLogic {
	return &UploadVideoLogic{ctx: ctx, svcCtx: s}
}

// UploadVideo 是生成路由的占位入口；上传必须走流式 multipart 的 UploadMultipart。
func (l *UploadVideoLogic) UploadVideo(_ *types.UploadMediaReq) (*types.UploadMediaResp, error) {
	return nil, errx.NewWithCode(errx.ParamError)
}

// UploadMultipart 把已绑定的 multipart 文件流式转发给媒体服务。
func (l *UploadVideoLogic) UploadMultipart(file io.Reader, filename, key string) (*types.UploadMediaResp, error) {
	return mediaupload.Upload(l.ctx, l.svcCtx, file, filename, key, false)
}
