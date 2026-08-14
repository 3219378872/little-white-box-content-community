package feed

import (
	"context"
	"strings"

	"errx"
	feedpb "esx/app/feed/rpc/xiaobaihe/feed/pb"
	"esx/app/interaction/rpc/interactionservice"
	"gateway/internal/svc"
	"user/userservice"
)

const postTargetType int32 = 1

type feedAuthor struct {
	name   string
	avatar string
}

type feedEnrichment struct {
	authors map[int64]feedAuthor
	liked   map[int64]bool
}

func loadFeedEnrichment(ctx context.Context, svcCtx *svc.ServiceContext, items []*feedpb.FeedItem, userID int64) (*feedEnrichment, error) {
	enrichment := &feedEnrichment{
		authors: make(map[int64]feedAuthor),
		liked:   make(map[int64]bool),
	}
	authorIDs, postIDs := uniqueFeedIDs(items)

	if len(authorIDs) > 0 {
		if svcCtx == nil || svcCtx.UserService == nil {
			return nil, errx.NewWithCode(errx.SystemError)
		}
		response, err := svcCtx.UserService.BatchGetUsers(ctx, &userservice.BatchGetUsersReq{UserIds: authorIDs})
		if err != nil {
			return nil, errx.FromRPCError(err)
		}
		if response == nil {
			return nil, errx.NewWithCode(errx.SystemError)
		}
		for _, user := range response.Users {
			if user == nil || user.Id <= 0 {
				continue
			}
			name := strings.TrimSpace(user.Nickname)
			if name == "" {
				name = strings.TrimSpace(user.Username)
			}
			enrichment.authors[user.Id] = feedAuthor{
				name:   name,
				avatar: strings.TrimSpace(user.AvatarUrl),
			}
		}
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
		for postID, liked := range response.Results {
			enrichment.liked[postID] = liked
		}
	}

	return enrichment, nil
}

func uniqueFeedIDs(items []*feedpb.FeedItem) ([]int64, []int64) {
	authorIDs := make([]int64, 0, len(items))
	postIDs := make([]int64, 0, len(items))
	seenAuthors := make(map[int64]struct{}, len(items))
	seenPosts := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.AuthorId > 0 {
			if _, ok := seenAuthors[item.AuthorId]; !ok {
				seenAuthors[item.AuthorId] = struct{}{}
				authorIDs = append(authorIDs, item.AuthorId)
			}
		}
		if item.PostId > 0 {
			if _, ok := seenPosts[item.PostId]; !ok {
				seenPosts[item.PostId] = struct{}{}
				postIDs = append(postIDs, item.PostId)
			}
		}
	}
	return authorIDs, postIDs
}

func (e *feedEnrichment) author(authorID int64) feedAuthor {
	if e == nil {
		return feedAuthor{}
	}
	return e.authors[authorID]
}

func (e *feedEnrichment) isLiked(postID int64) bool {
	return e != nil && e.liked[postID]
}
