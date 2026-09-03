package feed

import (
	"context"
	"maps"

	feedpb "esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/gateway/internal/logic/authorx"
	"esx/app/gateway/internal/svc"
	"esx/app/interaction/rpc/interactionservice"
	"esx/pkg/errx"
)

const postTargetType int32 = 1

type feedEnrichment struct {
	authors map[int64]authorx.Author
	liked   map[int64]bool
}

func loadFeedEnrichment(ctx context.Context, svcCtx *svc.ServiceContext, items []*feedpb.FeedItem, userID int64) (*feedEnrichment, error) {
	authorIDs, postIDs := uniqueFeedIDs(items)
	authors, err := authorx.Load(ctx, svcCtx, authorIDs)
	if err != nil {
		return nil, err
	}
	enrichment := &feedEnrichment{
		authors: authors,
		liked:   make(map[int64]bool),
	}

	if userID > 0 && len(postIDs) > 0 {
		if svcCtx == nil || svcCtx.InteractionService == nil {
			return nil, errx.NewWithCode(errx.SystemError)
		}
		response, err := svcCtx.InteractionService.BatchCheckLiked(ctx, &interactionservice.BatchCheckLikedReq{
			UserId:     userID,
			TargetIds:  postIDs,
			TargetType: postTargetType,
		})
		if err != nil {
			return nil, errx.FromRPCError(err)
		}
		if response == nil {
			return nil, errx.NewWithCode(errx.SystemError)
		}
		maps.Copy(enrichment.liked, response.Results)
	}

	return enrichment, nil
}

func uniqueFeedIDs(items []*feedpb.FeedItem) ([]int64, []int64) {
	authorIDs := make([]int64, 0, len(items))
	postIDs := make([]int64, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		authorIDs = append(authorIDs, item.AuthorId)
		postIDs = append(postIDs, item.PostId)
	}
	return authorx.UniquePositive(authorIDs), authorx.UniquePositive(postIDs)
}

func (e *feedEnrichment) author(authorID int64) authorx.Author {
	if e == nil {
		return authorx.Author{}
	}
	return e.authors[authorID]
}

func (e *feedEnrichment) isLiked(postID int64) bool {
	return e != nil && e.liked[postID]
}
