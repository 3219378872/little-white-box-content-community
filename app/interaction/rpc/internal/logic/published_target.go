package logic

import (
	"context"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/internal/svc"
	"esx/pkg/visibilityx"
)

const postTargetType int32 = 1

func requirePublishedPost(ctx context.Context, content svc.ContentService, postID int64) error {
	if postID <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	if content == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	resp, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: postID})
	if err != nil {
		return errx.FromRPCError(err)
	}
	if resp == nil || resp.Post == nil || !visibilityx.IsPublished(resp.Post.Status) {
		return errx.NewWithCode(errx.ContentNotFound)
	}
	return nil
}

func requirePublishedLikeTarget(ctx context.Context, content svc.ContentService, targetID int64, targetType int32) error {
	if targetType == postTargetType {
		return requirePublishedPost(ctx, content, targetID)
	}
	if targetType == 2 {
		// Comment likes need the parent post to be published (CORE-034).
		// Content has no GetComment RPC yet; do not invent a post id.
		return nil
	}
	return errx.NewWithCode(errx.ParamError)
}
