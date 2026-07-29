package logic

import (
	"context"
	"errors"

	"esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	"esx/pkg/outboxx"
)

type legacyTestInteractionCommands struct {
	service *svc.ServiceContext
}

func legacyInteractionCommandsFor(service *svc.ServiceContext) model.InteractionCommandModel {
	return &legacyTestInteractionCommands{service: service}
}

func (c *legacyTestInteractionCommands) Like(
	ctx context.Context,
	userID, targetID, targetType int64,
	_ outboxx.Event,
) (int64, error) {
	result, id, err := c.service.LikeRecordModel.UpsertLikeStatusTx(
		ctx, c.service.Conn, userID, targetID, targetType, model.StatusActive,
	)
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, errors.New("nil like result")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if changed == 0 {
		return 0, model.ErrNoStateChange
	}
	if c.service.ActionCountModel == nil {
		return 0, errors.New("nil action count model")
	}
	if err := c.service.ActionCountModel.IncrLikeCountTx(ctx, c.service.Conn, targetID, targetType); err != nil {
		return 0, err
	}
	return id, nil
}

func (c *legacyTestInteractionCommands) Unlike(
	ctx context.Context,
	recordID, targetID, targetType int64,
	_ outboxx.Event,
) error {
	result, err := c.service.LikeRecordModel.UpdateStatusById(
		ctx, recordID, model.StatusActive, model.StatusInactive,
	)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("nil unlike result")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return model.ErrNoStateChange
	}
	if c.service.ActionCountModel != nil {
		return c.service.ActionCountModel.DecrLikeCount(ctx, targetID, targetType)
	}
	return nil
}

func (c *legacyTestInteractionCommands) Favorite(
	ctx context.Context,
	userID, postID int64,
	_ outboxx.Event,
) (int64, error) {
	result, err := c.service.FavoriteModel.UpsertFavoriteStatus(ctx, userID, postID, model.StatusActive)
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, errors.New("nil favorite result")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if changed == 0 {
		return 0, model.ErrNoStateChange
	}
	if c.service.ActionCountModel != nil {
		if err := c.service.ActionCountModel.IncrFavoriteCount(ctx, postID, 1); err != nil {
			return 0, err
		}
	}
	return 1, nil
}

func (c *legacyTestInteractionCommands) Unfavorite(
	ctx context.Context,
	recordID, postID int64,
	_ outboxx.Event,
) error {
	result, err := c.service.FavoriteModel.UpdateStatusById(
		ctx, recordID, model.StatusActive, model.StatusInactive,
	)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("nil unfavorite result")
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return model.ErrNoStateChange
	}
	if c.service.ActionCountModel != nil {
		return c.service.ActionCountModel.DecrFavoriteCount(ctx, postID, 1)
	}
	return nil
}
