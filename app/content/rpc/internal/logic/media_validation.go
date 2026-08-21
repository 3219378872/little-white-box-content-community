package logic

import (
	"context"
	"esx/app/media/rpc/mediaservice"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

// validatePostMedia 校验帖子引用的媒体（CORE-024）：媒体必须存在、已完成上传
// 且属于调用者。未配置媒体服务时返回不可用，避免绕过校验。
func validatePostMedia(ctx context.Context, logger logx.Logger, media mediaservice.MediaService, userID int64, mediaIDs []int64) error {
	if len(mediaIDs) == 0 {
		return nil
	}
	if media == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	response, err := media.BatchGetMedia(ctx, &mediaservice.BatchGetMediaReq{MediaIds: mediaIDs})
	if err != nil {
		logger.Errorw("media validation BatchGetMedia failed", logx.Field("media_ids", mediaIDs), logx.Field("err", err.Error()))
		return errx.Wrap(err, errx.ServiceUnavailable)
	}
	if response == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	byID := make(map[int64]bool, len(response.Medias))
	for _, mediaInfo := range response.Medias {
		if mediaInfo == nil {
			continue
		}
		byID[mediaInfo.Id] = true
		if mediaInfo.UserId != userID || mediaInfo.Status != 1 {
			return errx.NewWithCode(errx.ParamError)
		}
	}
	for _, mediaID := range mediaIDs {
		if !byID[mediaID] {
			return errx.NewWithCode(errx.ParamError)
		}
	}
	return nil
}
