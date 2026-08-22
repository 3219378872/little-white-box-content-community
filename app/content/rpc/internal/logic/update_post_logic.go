package logic

import (
	"context"
	"errors"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/util"
	"esx/pkg/visibilityx"
	"strings"
	"unicode/utf8"

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

// UpdatePost 更新帖子（局部更新语义，B3）：.api 中 title/content 为 optional，
// 空串表示未提供、保留现值；合并后的完整字段再统一做长度校验。
func (l *UpdatePostLogic) UpdatePost(in *pb.UpdatePostReq) (*pb.UpdatePostResp, error) {
	if in.PostId <= 0 || in.AuthorId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
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
	if in.Status != nil && !visibilityx.IsDraft(*in.Status) && !visibilityx.IsPublished(*in.Status) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.Title == "" && in.Content == "" && in.Images == nil && in.Tags == nil &&
		in.Status == nil && len(in.MediaIds) == 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if err := validatePostMedia(l.ctx, l.Logger, l.svcCtx.MediaService, in.AuthorId, in.MediaIds); err != nil {
		return nil, err
	}

	// 鉴权：查帖子仅用于身份校验与现值合并不用于写回（防止 Lost Update）
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
	if visibilityx.IsDeleted(int32(post.Status)) {
		return nil, errx.NewWithCode(errx.PostAlreadyDeleted)
	}
	if post.AuthorId != in.AuthorId {
		return nil, errx.NewWithCode(errx.ContentForbidden)
	}
	// CORE-013：提供 expected_revision 时做乐观并发检测；0 表示旧客户端
	// 迁移期（CORE-062），跳过版本检查以保持 /api/v1 向后兼容。
	if in.ExpectedRevision > 0 && post.Revision != in.ExpectedRevision {
		return nil, errx.NewWithCode(errx.ContentVersionConflict)
	}

	mergedTitle := post.Title
	if in.GetTitle() != "" {
		mergedTitle = in.GetTitle()
	}
	mergedContent := post.Content
	if in.GetContent() != "" {
		mergedContent = in.GetContent()
	}
	titleRunes := utf8.RuneCountInString(mergedTitle)
	if titleRunes < 1 {
		return nil, errx.NewWithCode(errx.TitleEmpty)
	}
	if titleRunes > 120 {
		return nil, errx.NewWithCode(errx.ContentTooLong)
	}
	contentRunes := utf8.RuneCountInString(mergedContent)
	if contentRunes < 1 {
		return nil, errx.NewWithCode(errx.ContentEmpty)
	}
	if contentRunes > 20000 {
		return nil, errx.NewWithCode(errx.ContentTooLong)
	}

	// 校验图片
	for _, image := range in.Images {
		if strings.ContainsRune(image, ',') {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}

	// PATCH 语义：只写入客户端显式传入的字段，避免静默清空现有值；
	// title/content 写入的是与现值合并后的结果。
	fields := map[string]interface{}{
		"title":   mergedTitle,
		"content": mergedContent,
	}
	if len(in.Images) > 0 {
		fields["images"] = util.ToJsonObject(in.Images)
	}
	// Status 只在显式设置时更新，支持 draft ⇄ published 双向转换
	if in.Status != nil && int64(*in.Status) != post.Status {
		fields["status"] = int64(*in.Status)
	}
	// 计算更新后的状态，供下游（搜索索引等）判断是否仍可发现（CORE-015）。
	newStatus := post.Status
	if in.Status != nil {
		newStatus = int64(*in.Status)
	}

	// 标签仅在显式提供时替换；缺省时保留现有标签并让事件沿用旧值，
	// 避免 title-only 更新静默清空标签（B3）。模型层在 replaceTags=false
	// 时不触碰 post_tag，因此传空切片。
	replaceTags := in.Tags != nil
	var eventTags []string
	var modelTags []string
	var modelTagIDs []int64
	if replaceTags {
		for _, tag := range in.Tags {
			if tag != "" {
				eventTags = append(eventTags, tag)
			}
		}
		modelTags = eventTags
		modelTagIDs = make([]int64, 0, len(eventTags))
		for range eventTags {
			tid, idErr := util.NextID()
			if idErr != nil {
				l.Errorw("generate tag id failed", logx.Field("err", idErr.Error()))
				return nil, errx.NewWithCode(errx.SystemError)
			}
			modelTagIDs = append(modelTagIDs, tid)
		}
	} else {
		eventTags, err = l.svcCtx.PostTagModel.FindTagNamesByPostId(l.ctx, post.Id)
		if err != nil {
			l.Errorw("find existing tags for update failed",
				logx.Field("postId", post.Id), logx.Field("err", err.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}

	bodyExcerpt := mergedContent
	if len(bodyExcerpt) > 256 {
		bodyExcerpt = bodyExcerpt[:256]
	}
	outboxEvent, err := buildPostOutboxEvent(mqx.TopicPostUpdate, event.PostEvent{
		Type:        event.PostEventUpdated,
		PostID:      post.Id,
		AuthorID:    post.AuthorId,
		Title:       mergedTitle,
		BodyExcerpt: bodyExcerpt,
		Tags:        eventTags,
		Status:      int32(newStatus),
		Revision:    post.Revision + 1,
	})
	if err != nil {
		l.Errorw("build post-updated event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if l.svcCtx.PostCommandModel == nil {
		l.Errorw("PostCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.PostCommandModel.UpdatePost(l.ctx, post.Id, fields, modelTags, modelTagIDs, outboxEvent, in.ExpectedRevision, replaceTags); err != nil {
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

	return &pb.UpdatePostResp{
		Status:   int32(newStatus),
		Revision: post.Revision + 1,
	}, nil
}
