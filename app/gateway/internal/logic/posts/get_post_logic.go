// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"
	"strings"

	"errx"
	"esx/app/content/rpc/contentservice"
	"gateway/internal/logic/viewerstate"
	"jwtx"
	"user/userservice"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取帖子详情
func NewGetPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostLogic {
	return &GetPostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostLogic) GetPost(req *types.GetPostReq) (resp *types.GetPostResp, err error) {
	// CORE：已发布帖子允许匿名读取；登录用户才回填互动状态。
	userId, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)

	result, err := l.svcCtx.ContentService.GetPost(l.ctx, &contentservice.GetPostReq{
		PostId: req.PostId,
		UserId: userId,
	})
	if err != nil {
		l.Errorw("ContentService.GetPost RPC failed",
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
	}

	post := result.Post
	if post == nil {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}

	liked, favorited, err := viewerstate.Enrich(l.ctx, l.svcCtx, userId, []int64{post.Id})
	if err != nil {
		l.Errorw("viewerstate.Enrich failed", logx.Field("postId", post.Id), logx.Field("err", err.Error()))
		return nil, err
	}

	authorName, authorAvatar := l.loadPostAuthor(post.AuthorId)
	return &types.GetPostResp{
		Id:            post.Id,
		AuthorId:      post.AuthorId,
		AuthorName:    authorName,
		AuthorAvatar:  authorAvatar,
		Title:         post.Title,
		Content:       post.Content,
		Images:        post.Images,
		Tags:          post.Tags,
		Status:        post.Status,
		ViewCount:     post.ViewCount,
		LikeCount:     post.LikeCount,
		CommentCount:  post.CommentCount,
		FavoriteCount: post.FavoriteCount,
		IsLiked:       liked[post.Id],
		IsFavorited:   favorited[post.Id],
		Revision:      post.Revision,
		CreatedAt:     post.CreatedAt,
	}, nil
}

func (l *GetPostLogic) loadPostAuthor(authorID int64) (string, string) {
	if authorID <= 0 || l.svcCtx == nil || l.svcCtx.UserService == nil {
		return "", ""
	}
	response, err := l.svcCtx.UserService.BatchGetUsers(l.ctx, &userservice.BatchGetUsersReq{UserIds: []int64{authorID}})
	if err != nil {
		l.Errorw("UserService.BatchGetUsers failed", logx.Field("authorId", authorID), logx.Field("err", err.Error()))
		return "", ""
	}
	if response == nil {
		l.Error("UserService.BatchGetUsers returned a nil response")
		return "", ""
	}
	for _, user := range response.Users {
		if user == nil || user.Id != authorID {
			continue
		}
		name := strings.TrimSpace(user.Nickname)
		if name == "" {
			name = strings.TrimSpace(user.Username)
		}
		return name, strings.TrimSpace(user.AvatarUrl)
	}
	return "", ""
}
