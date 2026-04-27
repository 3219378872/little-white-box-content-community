package logic

import (
	"context"
	"errors"
	"errx"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"strings"
	"util"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdatePostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePostLogic {
	return &UpdatePostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// UpdatePost 更新帖子
func (l *UpdatePostLogic) UpdatePost(in *pb.UpdatePostReq) (*pb.UpdatePostResp, error) {
	if in.PostId <= 0 || in.AuthorId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.Title == "" {
		return nil, errx.NewWithCode(errx.TitleEmpty)
	}
	if in.Content == "" {
		return nil, errx.NewWithCode(errx.ContentEmpty)
	}

	// 鉴权：查帖子仅用于身份校验，不用于写回（防止 Lost Update）
	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, in.PostId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("PostModel.FindPostById failed",
			logx.Field("postId", in.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if post.Status == 2 {
		return nil, errx.NewWithCode(errx.PostAlreadyDeleted)
	}
	if post.AuthorId != in.AuthorId {
		return nil, errx.NewWithCode(errx.ContentForbidden)
	}

	// 校验图片
	for _, image := range in.Images {
		if strings.ContainsRune(image, ',') {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}

	// PATCH 语义：只写入客户端显式传入的字段，避免静默清空现有值
	fields := map[string]interface{}{
		"title":   in.Title,
		"content": in.Content,
	}
	if len(in.Images) > 0 {
		fields["images"] = util.ToJsonObject(in.Images)
	}
	// Status 只在显式设置（>0）时更新，避免 proto3 零值默认把已发布帖子降级为草稿
	if in.Status > 0 {
		fields["status"] = int64(in.Status)
	}

	if err = l.svcCtx.PostModel.UpdateFields(l.ctx, post.Id, fields); err != nil {
		l.Errorw("PostModel.UpdateFields failed",
			logx.Field("postId", post.Id),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// 收集有效标签并预生成 ID
	validTags := make([]string, 0, len(in.Tags))
	for _, tag := range in.Tags {
		if tag != "" {
			validTags = append(validTags, tag)
		}
	}
	tagIds := make([]int64, 0, len(validTags))
	for range validTags {
		tid, idErr := util.NextID()
		if idErr != nil {
			l.Errorw("generate tag id failed", logx.Field("err", idErr.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
		tagIds = append(tagIds, tid)
	}

	// 事务内原子替换标签
	if err = l.svcCtx.PostTagModel.TransactReplaceTagsByPostId(l.ctx, l.svcCtx.Conn, post.Id, validTags, tagIds); err != nil {
		l.Errorw("PostTagModel.TransactReplaceTagsByPostId failed",
			logx.Field("postId", post.Id),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.UpdatePostResp{}, nil
}
