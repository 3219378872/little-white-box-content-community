package logic

import (
	"context"
	"esx/app/media/rpc/mediaservice"
	"esx/pkg/errx"

	logx "esx/pkg/logging"
)

// validateMessageMedia 校验媒体消息引用的媒体（CORE-041）：媒体必须存在、
// 已完成上传且属于发送者。
func validateMessageMedia(ctx context.Context, logger logx.Logger, media mediaservice.MediaService, senderID, mediaID int64, msgType int32) (string, error) {
	if mediaID <= 0 {
		return "", errx.NewWithCode(errx.ParamError)
	}
	if media == nil {
		return "", errx.NewWithCode(errx.ServiceUnavailable)
	}
	response, err := media.BatchGetMedia(ctx, &mediaservice.BatchGetMediaReq{MediaIds: []int64{mediaID}})
	if err != nil {
		logger.Errorw("message media validation BatchGetMedia failed", logx.Field("media_id", mediaID), logx.Field("err", err.Error()))
		return "", errx.Wrap(err, errx.ServiceUnavailable)
	}
	if response == nil || len(response.Medias) != 1 {
		return "", errx.NewWithCode(errx.ParamError)
	}
	info := response.Medias[0]
	if info == nil || info.Id != mediaID || info.UserId != senderID || info.Status != 1 {
		return "", errx.NewWithCode(errx.ParamError)
	}
	expected := map[int32]string{2: "image", 3: "video", 4: "audio"}[msgType]
	if info.FileType != expected || info.Url == "" {
		return "", errx.NewWithCode(errx.ParamError)
	}
	return info.Url, nil
}
