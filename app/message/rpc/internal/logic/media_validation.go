package logic

import (
	"context"
	"errx"
	"esx/app/media/rpc/mediaservice"

	"github.com/zeromicro/go-zero/core/logx"
)

// validateMessageMedia 校验媒体消息引用的媒体（CORE-041）：媒体必须存在、
// 已完成上传且属于发送者。
func validateMessageMedia(ctx context.Context, logger logx.Logger, media mediaservice.MediaService, senderID, mediaID int64) error {
	if mediaID <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	if media == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	response, err := media.BatchGetMedia(ctx, &mediaservice.BatchGetMediaReq{MediaIds: []int64{mediaID}})
	if err != nil {
		logger.Errorw("message media validation BatchGetMedia failed", logx.Field("media_id", mediaID), logx.Field("err", err.Error()))
		return errx.Wrap(err, errx.ServiceUnavailable)
	}
	if response == nil || len(response.Medias) != 1 {
		return errx.NewWithCode(errx.ParamError)
	}
	info := response.Medias[0]
	if info == nil || info.Id != mediaID || info.UserId != senderID || info.Status != 1 {
		return errx.NewWithCode(errx.ParamError)
	}
	return nil
}
