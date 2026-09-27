// Package mediaupload bridges bounded HTTP files to the existing streaming RPCs.
package mediaupload

import (
	"context"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	pb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"esx/pkg/errx"
	"esx/pkg/jwtx"
	"io"
	"strings"
)

func Upload(ctx context.Context, svcCtx *svc.ServiceContext, file io.Reader, filename, key string, audio bool) (*types.UploadMediaResp, error) {
	userID, _ := jwtx.GetUserIdFromContext(ctx)
	if userID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	// Cancellation also closes a partially sent stream on read/send failures.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	meta := &pb.UploadMeta{UserId: userID, FileName: filename, IdempotencyKey: key}
	var send func([]byte) error
	var finish func() (*pb.MediaInfo, error)
	if audio {
		stream, err := svcCtx.MediaService.UploadAudio(ctx)
		if err != nil {
			return nil, errx.FromGRPCError(err)
		}
		if err = stream.Send(&pb.UploadAudioReq{Data: &pb.UploadAudioReq_Meta{Meta: meta}}); err != nil {
			return nil, errx.FromGRPCError(err)
		}
		send = func(b []byte) error { return stream.Send(&pb.UploadAudioReq{Data: &pb.UploadAudioReq_Chunk{Chunk: b}}) }
		finish = func() (*pb.MediaInfo, error) {
			resp, err := stream.CloseAndRecv()
			if err != nil {
				return nil, err
			}
			return resp.GetMedia(), nil
		}
	} else {
		stream, err := svcCtx.MediaService.UploadVideo(ctx)
		if err != nil {
			return nil, errx.FromGRPCError(err)
		}
		if err = stream.Send(&pb.UploadVideoReq{Data: &pb.UploadVideoReq_Meta{Meta: meta}}); err != nil {
			return nil, errx.FromGRPCError(err)
		}
		send = func(b []byte) error { return stream.Send(&pb.UploadVideoReq{Data: &pb.UploadVideoReq_Chunk{Chunk: b}}) }
		finish = func() (*pb.MediaInfo, error) {
			resp, err := stream.CloseAndRecv()
			if err != nil {
				return nil, err
			}
			return resp.GetMedia(), nil
		}
	}
	buf := make([]byte, 1<<20)
	for {
		if ctx.Err() != nil {
			return nil, errx.NewWithCode(errx.UploadFailed)
		}
		n, readErr := file.Read(buf)
		if n > 0 {
			if err := send(append([]byte(nil), buf[:n]...)); err != nil {
				// Recv carries the business error when Send reports EOF.
				if err == io.EOF {
					if _, finalErr := finish(); finalErr != nil {
						return nil, errx.FromGRPCError(finalErr)
					}
				}
				return nil, errx.FromGRPCError(err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, errx.NewWithCode(errx.UploadFailed)
		}
	}
	media, err := finish()
	if err != nil {
		return nil, errx.FromGRPCError(err)
	}
	if media == nil || media.Id <= 0 || media.Url == "" {
		return nil, errx.NewWithCode(errx.UploadFailed)
	}
	return &types.UploadMediaResp{MediaId: media.Id, Url: media.Url, FileType: media.FileType, MimeType: media.MimeType, FileSize: media.FileSize}, nil
}
