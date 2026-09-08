package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"

	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/app/media/rpc/mediaservice"
	"esx/pkg/errx"

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
	seen := make(map[int64]struct{}, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		info := byID[mediaID]
		_, duplicate := seen[mediaID]
		if mediaID <= 0 || info == nil || duplicate {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		seen[mediaID] = struct{}{}
		urls = append(urls, info.Url)
	}
	return urls, nil
}

func createPostImages(images, verifiedURLs []string) ([]string, error) {
	if len(images) > 0 && !slices.Equal(images, verifiedURLs) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	return verifiedURLs, nil
}

// Existing URLs are a compatibility boundary for the same author's post, not
// an alternative authorization path for adding media from arbitrary URLs.
func (l *UpdatePostLogic) mergePostMedia(in *pb.UpdatePostReq, post *model.Post, verifiedURLs []string, fields map[string]any) error {
	imagesProvided := in.ImagesProvided || in.Images != nil
	idsProvided := in.MediaIdsProvided || in.MediaIds != nil
	if !imagesProvided && !idsProvided {
		return nil
	}
	images := verifiedURLs
	ids := in.MediaIds
	if imagesProvided {
		images = in.Images
		ids = make([]int64, 0, len(images))
		if len(images) == 0 && len(in.MediaIds) > 0 {
			return errx.NewWithCode(errx.ParamError)
		}
		allowed := make(map[string]int64, len(images)+len(verifiedURLs))
		if len(images) > 0 {
			if post.Images.Valid {
				for _, url := range strings.Split(post.Images.String, ",") {
					allowed[url] = 0
				}
			}
			existingIDs := decodeInt64sJSON(post.MediaIds)
			if len(existingIDs) > 0 {
				if l.svcCtx == nil {
					return errx.NewWithCode(errx.ServiceUnavailable)
				}
				existingURLs, err := validatePostMedia(l.ctx, l.Logger, l.svcCtx.MediaService, in.AuthorId, existingIDs)
				if err != nil {
					return err
				}
				for i, url := range existingURLs {
					if _, exists := allowed[url]; exists {
						allowed[url] = existingIDs[i]
					}
				}
			}
		}
		for i, url := range verifiedURLs {
			allowed[url] = in.MediaIds[i]
		}
		selected := make(map[string]struct{}, len(images))
		for _, url := range images {
			id, permitted := allowed[url]
			_, duplicate := selected[url]
			if !permitted || url == "" || duplicate {
				return errx.NewWithCode(errx.ParamError)
			}
			selected[url] = struct{}{}
			if id > 0 {
				ids = append(ids, id)
			}
		}
		for _, url := range verifiedURLs {
			if _, included := selected[url]; !included {
				return errx.NewWithCode(errx.ParamError)
			}
		}
	}
	encodedIDs, err := encodeInt64sJSON(ids)
	if err != nil {
		return errx.NewWithCode(errx.SystemError)
	}
	if images == nil {
		images = []string{}
	}
	fields["images"] = model.ToJSONObject(images)
	fields["media_ids"] = encodedIDs
	return nil
}

func encodeInt64sJSON(ids []int64) (sql.NullString, error) {
	if len(ids) == 0 {
		return sql.NullString{}, nil
	}
	raw, err := model.ToJSONObject(ids).JSONString()
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
