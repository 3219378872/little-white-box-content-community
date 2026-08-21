package logic

import (
	"context"

	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/internal/svc"
	"esx/pkg/errx"
)

const (
	postTargetType    int32 = 1
	commentTargetType int32 = 2
)

func requirePublishedPost(ctx context.Context, content svc.ContentService, postID int64) error {
	return requirePublishedLikeTarget(ctx, content, postID, postTargetType)
}

func requirePublishedLikeTarget(ctx context.Context, content svc.ContentService, targetID int64, targetType int32) error {
	if targetID <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	if targetType != postTargetType && targetType != commentTargetType {
		return errx.NewWithCode(errx.ParamError)
	}
	if content == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	_, err := content.AssertInteractable(ctx, &contentservice.AssertInteractableReq{
		TargetId:   targetID,
		TargetType: targetType,
	})
	if err != nil {
		return errx.FromRPCError(err)
	}
	return nil
}
