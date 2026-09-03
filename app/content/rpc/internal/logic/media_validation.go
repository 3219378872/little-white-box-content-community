package logic

import (
	"context"
	"database/sql"
	"encoding/json"

	"esx/app/media/rpc/mediaservice"
	"esx/pkg/errx"
	"esx/pkg/util"

	"github.com/zeromicro/go-zero/core/logx"
)

// validatePostMedia 校验帖子引用的媒体（CORE-024）：媒体必须存在、已完成上传
// 且属于调用者。未配置媒体服务时返回不可用，避免绕过校验。
// 成功时按请求顺序返回公开 URL，供 images 为空时回填。
func validatePostMedia(ctx context.Context, logger logx.Logger, media mediaservice.MediaService, userID int64, mediaIDs []int64) ([]string, error) {
	if len(mediaIDs) == 0 {
		return nil, nil
	}
	if media == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	response, err := media.BatchGetMedia(ctx, &mediaservice.BatchGetMediaReq{MediaIds: mediaIDs})
	if err != nil {
		logger.Errorw("media validation BatchGetMedia failed", logx.Field("media_ids", mediaIDs), logx.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.ServiceUnavailable)
	}
	if response == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	byID := make(map[int64]*mediaservice.MediaInfo, len(response.Medias))
	for _, mediaInfo := range response.Medias {
		if mediaInfo == nil {
			continue
		}
		byID[mediaInfo.Id] = mediaInfo
		if mediaInfo.UserId != userID || mediaInfo.Status != 1 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}
	urls := make([]string, 0, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		info := byID[mediaID]
		if info == nil {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		urls = append(urls, info.Url)
	}
	return urls, nil
}

func encodeInt64sJSON(ids []int64) (sql.NullString, error) {
	if len(ids) == 0 {
		return sql.NullString{}, nil
	}
	raw, err := util.ToJsonObject(ids).JsonString()
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: raw, Valid: true}, nil
}

func decodeInt64sJSON(raw sql.NullString) []int64 {
	if !raw.Valid || raw.String == "" {
		return []int64{}
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw.String), &ids); err != nil {
		return []int64{}
	}
	return ids
}
