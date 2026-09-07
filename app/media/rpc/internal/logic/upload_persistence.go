package logic

import (
	"context"
	"errors"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
	"github.com/zeromicro/go-zero/core/logx"
)

// The caller keeps uploaded objects only after an authoritative create, before sending the response.
func persistUploadedMedia(ctx context.Context, logger logx.Logger, svcCtx *svc.ServiceContext, row *model.Media, idem idempotencyx.IdempotencyRecord) (*model.Media, bool, error) {
	result, err := svcCtx.MediaCommandModel.CreateMedia(ctx, row, idem)
	if err != nil {
		if errors.Is(err, idempotencyx.ErrIdempotencyConflict) {
			return nil, false, errx.NewWithCode(errx.IdempotencyConflict)
		}
		logger.Errorw("insert media row failed", logx.Field("user_id", row.UserId), logx.Field("object_key", row.ObjectKey.String), logx.Field("err", err.Error()))
		return nil, false, errx.NewWithCode(errx.SystemError)
	}
	if result.Created {
		return row, true, nil
	}
	existing, err := svcCtx.MediaModel.FindOne(ctx, result.MediaID)
	if err != nil {
		logger.Errorw("find existing media on idempotent retry failed", logx.Field("media_id", result.MediaID), logx.Field("err", err.Error()))
		return nil, false, errx.NewWithCode(errx.SystemError)
	}
	return existing, false, nil
}
