package model

import (
	"context"
	"fmt"

	"esx/app/content/rpc/contentservice"
	"esx/app/user/rpc/userservice"
	"esx/pkg/visibilityx"
)

type ContentPostRecallSource struct {
	name    string
	reason  string
	sortBy  int32
	content contentservice.ContentService
}

func NewContentPostRecallSource(name, reason string, sortBy int32, content contentservice.ContentService) *ContentPostRecallSource {
	return &ContentPostRecallSource{name: name, reason: reason, sortBy: sortBy, content: content}
}

func (s *ContentPostRecallSource) Name() string {
	return s.name
}

func (s *ContentPostRecallSource) Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error) {
	if s.content == nil {
		return nil, ErrNotApplicable
	}
	pageSize := req.Limit
	if pageSize > 50 {
		pageSize = 50
	}
	if pageSize <= 0 {
		return nil, fmt.Errorf("%s content recall limit must be positive", s.name)
	}
	response, err := s.content.GetPostList(ctx, &contentservice.GetPostListReq{
		PageSize: int32(pageSize), SortBy: s.sortBy, UserId: req.UserID,
	})
	if err != nil {
		return nil, fmt.Errorf("load %s content recall: %w", s.name, err)
	}
	if response == nil {
		return []PostCandidate{}, nil
	}
	result := make([]PostCandidate, 0, len(response.Posts))
	for index, post := range response.Posts {
		if post == nil || post.Id <= 0 || !visibilityx.IsPublished(post.Status) {
			continue
		}
		category := ""
		if len(post.Tags) > 0 {
			category = post.Tags[0]
		}
		result = append(result, PostCandidate{
			PostID:       post.Id,
			RecallScore:  1 / float64(index+1),
			RecallSource: s.name,
			Reason:       s.reason,
			AuthorID:     post.AuthorId,
			Category:     category,
			Features: PostFeatures{
				Known:      true,
				Available:  true,
				Visibility: "public",
				AuthorID:   post.AuthorId,
				Category:   category,
				Popularity: float64(post.ViewCount) + float64(post.LikeCount)*3 +
					float64(post.CommentCount)*4 + float64(post.FavoriteCount)*3,
			},
		})
	}
	return result, nil
}

type SocialUserRecallSource struct {
	users userservice.UserService
}

func NewSocialUserRecallSource(users userservice.UserService) *SocialUserRecallSource {
	return &SocialUserRecallSource{users: users}
}

func (s *SocialUserRecallSource) Name() string {
	return "social"
}

func (s *SocialUserRecallSource) Recall(ctx context.Context, req RecallRequest) ([]UserCandidate, error) {
	if s.users == nil || req.UserID <= 0 {
		return nil, ErrNotApplicable
	}
	if req.Limit <= 0 {
		return nil, fmt.Errorf("social user recall limit must be positive")
	}
	response, err := s.users.GetFollowers(ctx, &userservice.GetFollowersReq{
		UserId: req.UserID, Page: 1, PageSize: int32(req.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("load social user recall: %w", err)
	}
	if response == nil {
		return []UserCandidate{}, nil
	}
	result := make([]UserCandidate, 0, len(response.Users))
	for index, user := range response.Users {
		if user == nil || user.Id <= 0 || user.Id == req.UserID {
			continue
		}
		result = append(result, UserCandidate{
			UserID:       user.Id,
			RecallScore:  1 / float64(index+1),
			RecallSource: "social",
			Reason:       "followed by people in your network",
			Features: UserFeatures{
				Known:       true,
				Available:   true,
				Visibility:  "public",
				MutualCount: float64(user.FollowerCount),
			},
		})
	}
	return result, nil
}
