package logic

import (
	"context"
	"errors"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"

	logx "esx/pkg/logging"
)

// The boolean transfers object ownership even on error: true means a committed
// owner exists OR the commit remains unknown. Callers must set their cleanup
// guard before inspecting err. Only confirmed rollback/duplicate allows cleanup.
func persistUploadedMedia(ctx context.Context, logger logx.Logger, svcCtx *svc.ServiceContext, row *model.Media, idem idempotencyx.IdempotencyRecord) (*model.Media, bool, error) {
	result, err := svcCtx.MediaCommandModel.CreateMedia(ctx, row, idem)
	if err != nil {
		keepObjects := result.Outcome != model.MediaNotCommitted
		if errors.Is(err, idempotencyx.ErrIdempotencyConflict) {
			return nil, keepObjects, errx.NewWithCode(errx.IdempotencyConflict)
		}
		logger.Errorw("insert media row failed",
			logx.Field("user_id", row.UserId), logx.Field("media_id", row.Id),
			logx.Field("object_key", row.ObjectKey.String), logx.Field("thumbnail_object_key", row.ThumbnailObjectKey.String),
			logx.Field("retain_objects", keepObjects), logx.Field("err", err.Error()))
		return nil, keepObjects, errx.NewWithCode(errx.SystemError)
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
