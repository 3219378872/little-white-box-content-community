// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package image

import (
	"context"
	"errx"
	mediapb "esx/app/media/pb/xiaobaihe/media/pb"
	"fmt"
	"io"
	"jwtx"
	"mime/multipart"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

const chunkSize = 1 << 20 // 1 MB per chunk

type UploadImageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUploadImageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadImageLogic {
	return &UploadImageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UploadImage 保留原签名以保证 types 匹配（实际入口走 UploadImageMultipart）。
func (l *UploadImageLogic) UploadImage(_ *types.UploadImageReq) (*types.UploadImageResp, error) {
	return nil, errx.NewWithCode(errx.SystemError)
}

// UploadImageMultipart 从 handler 接收 multipart 文件，分块 streaming 到 Media RPC。
func (l *UploadImageLogic) UploadImageMultipart(file multipart.File, header *multipart.FileHeader) (*types.UploadImageResp, error) {
	userId, _ := jwtx.GetUserIdFromContext(l.ctx)
	if userId == 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}

	stream, err := l.svcCtx.MediaService.UploadImage(l.ctx)
	if err != nil {
		return nil, fmt.Errorf("建立 media 流失败: %w", err)
	}

	if err := stream.Send(&mediapb.UploadImageReq{
		Data: &mediapb.UploadImageReq_Meta{
			Meta: &mediapb.UploadMeta{
				UserId:   userId,
				FileName: header.Filename,
				Quality:  85,
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("发送 meta 失败: %w", err)
	}

	buf := make([]byte, chunkSize)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if err := stream.Send(&mediapb.UploadImageReq{
				Data: &mediapb.UploadImageReq_Chunk{Chunk: chunk},
			}); err != nil {
				return nil, fmt.Errorf("发送 chunk 失败: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取文件失败: %w", readErr)
		}
	}

	mediaResp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, fmt.Errorf("关闭流失败: %w", err)
	}
	if mediaResp == nil || mediaResp.Media == nil {
		return nil, errx.NewWithCode(errx.UploadFailed)
	}

	return &types.UploadImageResp{
		MediaId:      mediaResp.Media.Id,
		Url:          mediaResp.Media.Url,
		ThumbnailUrl: mediaResp.Media.ThumbnailUrl,
	}, nil
}
