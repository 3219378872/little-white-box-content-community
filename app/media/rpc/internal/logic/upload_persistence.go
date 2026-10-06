package logic

import (
	"context"
	"errors"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/svc"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"

	"esx/pkg/logging"
)

// The boolean transfers object ownership even on error: true means a committed
// owner exists OR the commit remains unknown. Callers must set their cleanup
// guard before inspecting err. Only confirmed rollback/duplicate allows cleanup.
func persistUploadedMedia(ctx context.Context, logger logging.Logger, svcCtx *svc.ServiceContext, row *model.Media, idem idempotencyx.IdempotencyRecord) (*model.Media, bool, error) {
	result, err := svcCtx.MediaCommandModel.CreateMedia(ctx, row, idem)
	if err != nil {
		keepObjects := result.Outcome != model.MediaNotCommitted
		if errors.Is(err, idempotencyx.ErrIdempotencyConflict) {
			return nil, keepObjects, errx.NewWithCode(errx.IdempotencyConflict)
		}
		logger.Errorw("insert media row failed",
			logging.Field("user_id", row.UserId), logging.Field("media_id", row.Id),
			logging.Field("object_key", row.ObjectKey.String), logging.Field("thumbnail_object_key", row.ThumbnailObjectKey.String),
			logging.Field("retain_objects", keepObjects), logging.Field("err", err.Error()))
		return nil, keepObjects, errx.NewWithCode(errx.SystemError)
	}
	if result.Created {
		return row, true, nil
	}
	existing, err := svcCtx.MediaModel.FindOne(ctx, result.MediaID)
	if err != nil {
		logger.Errorw("find existing media on idempotent retry failed", logging.Field("media_id", result.MediaID), logging.Field("err", err.Error()))
		return nil, false, errx.NewWithCode(errx.SystemError)
	}
	return existing, false, nil
}
