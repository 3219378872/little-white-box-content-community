package logic

import (
	"context"
	"errors"
	"errx"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/event"
	"mqx"
	"strings"
	"unicode/utf8"
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
	if in.ExpectedRevision <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	titleRunes := utf8.RuneCountInString(in.GetTitle())
	if titleRunes < 1 {
		return nil, errx.NewWithCode(errx.TitleEmpty)
	}
	if titleRunes > 120 {
		return nil, errx.NewWithCode(errx.ContentTooLong)
	}
	contentRunes := utf8.RuneCountInString(in.GetContent())
	if contentRunes < 1 {
		return nil, errx.NewWithCode(errx.ContentEmpty)
	}
	if contentRunes > 20000 {
		return nil, errx.NewWithCode(errx.ContentTooLong)
	}
	if len(in.Images) > 9 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if len(in.Tags) > 10 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if len(in.MediaIds) > 9 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	for _, tag := range in.Tags {
		if tag == "" {
			continue
		}
		tagRunes := utf8.RuneCountInString(tag)
		if tagRunes < 1 || tagRunes > 32 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}
	if in.Status != nil && *in.Status != 0 && *in.Status != 1 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if err := validatePostMedia(l.ctx, l.Logger, l.svcCtx.MediaService, in.AuthorId, in.MediaIds); err != nil {
		return nil, err
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
	if post.Revision != in.ExpectedRevision {
		return nil, errx.NewWithCode(errx.ContentVersionConflict)
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
	// Status 只在显式设置时更新，支持 draft ⇄ published 双向转换
	if in.Status != nil && int64(*in.Status) != post.Status {
		fields["status"] = int64(*in.Status)
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

	bodyExcerpt := in.GetContent()
	if len(bodyExcerpt) > 256 {
		bodyExcerpt = bodyExcerpt[:256]
	}
	outboxEvent, err := buildPostOutboxEvent(mqx.TopicPostUpdate, event.PostEvent{
		Type:        event.PostEventUpdated,
		PostID:      post.Id,
		AuthorID:    post.AuthorId,
		Title:       in.GetTitle(),
		BodyExcerpt: bodyExcerpt,
		Tags:        validTags,
	})
	if err != nil {
		l.Errorw("build post-updated event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if l.svcCtx.PostCommandModel == nil {
		l.Errorw("PostCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.PostCommandModel.UpdatePost(l.ctx, post.Id, fields, validTags, tagIds, outboxEvent, in.ExpectedRevision); err != nil {
		if errors.Is(err, model.ErrVersionConflict) {
			return nil, errx.NewWithCode(errx.ContentVersionConflict)
		}
		l.Errorw("update post transaction failed",
			logx.Field("postId", post.Id), logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.PostModel.InvalidatePostCache(l.ctx, post.Id); err != nil {
		l.Errorw("invalidate post cache after update failed",
			logx.Field("postId", post.Id), logx.Field("err", err.Error()))
	}

	newStatus := post.Status
	if in.Status != nil {
		newStatus = int64(*in.Status)
	}
	return &pb.UpdatePostResp{
		Status:   int32(newStatus),
		Revision: post.Revision + 1,
	}, nil
}
