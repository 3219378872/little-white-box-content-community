package logic

import (
	"context"
	"encoding/json"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/idempotencyx"
	"esx/pkg/mqx"
	"esx/pkg/visibilityx"
	"strings"

	"esx/pkg/logging"
)

type UpdatePostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewUpdatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePostLogic {
	return &UpdatePostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// UpdatePost 更新帖子（局部更新语义，B3）：.api 中 title/content 为 optional，
// 空串表示未提供、保留现值；合并后的完整字段再统一做长度校验。
// 流程：校验 → 幂等重放 → 校验媒体 → 读取并鉴权 → 合并字段与标签 → 事务内 CAS 写入并写 outbox → 清缓存。
func (l *UpdatePostLogic) UpdatePost(in *pb.UpdatePostReq) (*pb.UpdatePostResp, error) {
	if err := validateUpdatePost(in); err != nil {
		return nil, err
	}
	idem, err := updatePostIdempotency(in)
	if err != nil {
		return nil, err
	}
	idemModel, result, found, err := replayIdempotentPostCommand(l.ctx, l.svcCtx, idem)
	if err != nil {
		return nil, err
	}
	if found {
		status, revision := decodePostCommandResult(result)
		return &pb.UpdatePostResp{Status: status, Revision: revision}, nil
	}
	mediaURLs, err := validatePostMedia(l.ctx, l.Logger, l.svcCtx.MediaService, in.AuthorId, in.MediaIds)
	if err != nil {
		return nil, err
	}

	post, err := loadOwnedPost(l.ctx, l.Logger, l.svcCtx, in.PostId, in.AuthorId, in.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	patch, err := l.mergePostFields(in, post, mediaURLs)
	if err != nil {
		return nil, err
	}
	tags, err := l.tagsForUpdate(in, post)
	if err != nil {
		return nil, err
	}

	outboxEvent, err := buildPostOutboxEvent(mqx.TopicPostUpdate, postUpdatedEvent(post, patch, tags.event))
	if err != nil {
		l.Errorw("build post-updated event failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if l.svcCtx.PostCommandModel == nil {
		l.Errorw("PostCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	// 带幂等键时把结果（状态与新修订号）一并记下，供重试直接返回。
	nextRevision := post.Revision + 1
	if idem.Key != "" {
		_, err = idemModel.UpdatePostIdempotent(l.ctx, post.Id, patch.fields, tags.values, tags.ids, outboxEvent,
			in.ExpectedRevision, tags.replace, encodePostCommandResult(int32(patch.status), nextRevision), idem)
	} else {
		err = l.svcCtx.PostCommandModel.UpdatePost(l.ctx, post.Id, patch.fields, tags.values, tags.ids, outboxEvent, in.ExpectedRevision, tags.replace)
	}
	if err != nil {
		return nil, postCommandError(l.Logger, "update", post.Id, err)
	}
	invalidatePostCacheAfter(l.ctx, l.Logger, l.svcCtx, "update", post.Id)

	return &pb.UpdatePostResp{
		Status:   int32(patch.status),
		Revision: nextRevision,
	}, nil
}

// postUpdatedEvent 用合并后的字段组装更新事件；未改动的标签沿用旧值，计数沿用读取时的快照。
func postUpdatedEvent(post *model.Post, patch *postFieldUpdate, tags []string) event.PostEvent {
	createdAt := post.CreatedAt.UnixMilli()
	if createdAt < 0 {
		createdAt = 0
	}
	return event.PostEvent{
		Type:         event.PostEventUpdated,
		PostID:       post.Id,
		AuthorID:     post.AuthorId,
		Title:        patch.title,
		Body:         patch.content,
		BodyExcerpt:  runePrefix(patch.content, postEventExcerptRunes),
		Tags:         tags,
		Status:       int32(patch.status),
		Revision:     post.Revision + 1,
		LikeCount:    post.LikeCount,
		CommentCount: post.CommentCount,
		CreatedAt:    createdAt,
	}
}

// updatePostIdempotency 以完整请求（含“显式清空”标记）的 JSON 作为命令摘要。
func updatePostIdempotency(in *pb.UpdatePostReq) (idempotencyx.IdempotencyRecord, error) {
	key := strings.TrimSpace(in.GetIdempotencyKey())
	payload, err := json.Marshal(struct {
		PostID           int64    `json:"post_id"`
		AuthorID         int64    `json:"author_id"`
		Title            string   `json:"title"`
		Content          string   `json:"content"`
		Images           []string `json:"images"`
		Tags             []string `json:"tags"`
		Status           *int32   `json:"status"`
		ExpectedRevision int64    `json:"expected_revision"`
		MediaIDs         []int64  `json:"media_ids"`
		ImagesProvided   bool     `json:"images_provided,omitempty"`
		MediaIDsProvided bool     `json:"media_ids_provided,omitempty"`
	}{in.PostId, in.AuthorId, in.Title, in.Content, in.Images, in.Tags, in.Status, in.ExpectedRevision, in.MediaIds,
		in.ImagesProvided && len(in.Images) == 0, in.MediaIdsProvided && len(in.MediaIds) == 0})
	if err != nil {
		return idempotencyx.IdempotencyRecord{}, errx.NewWithCode(errx.ParamError)
	}
	row := idempotencyx.IdempotencyRecord{Scope: "post:update", UserID: in.AuthorId, Key: key, CommandHash: idempotencyx.CommandHash(string(payload))}
	if !row.Valid() {
		return idempotencyx.IdempotencyRecord{}, errx.NewWithCode(errx.ParamError)
	}
	return row, nil
}

// encodePostCommandResult 把状态（个位）与修订号压进一个整数存入幂等记录。
func encodePostCommandResult(status int32, revision int64) int64 {
	return revision*10 + int64(status)
}

// decodePostCommandResult 是 encodePostCommandResult 的逆运算。
func decodePostCommandResult(result int64) (int32, int64) {
	return int32(result % 10), result / 10
}

// validateUpdatePost 校验与现值无关的字段；标题与正文要等合并现值后才能校验。
// 至少要提供一个字段，否则这次更新没有意义。
func validateUpdatePost(in *pb.UpdatePostReq) error {
	if in.PostId <= 0 || in.AuthorId <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	if err := validatePostCollections(in.Images, in.Tags, in.MediaIds); err != nil {
		return err
	}
	if in.Status != nil && !visibilityx.IsDraft(*in.Status) && !visibilityx.IsPublished(*in.Status) {
		return errx.NewWithCode(errx.ParamError)
	}
	if in.Title == "" && in.Content == "" && in.Images == nil && in.Tags == nil &&
		in.Status == nil && len(in.MediaIds) == 0 && !in.ImagesProvided && !in.MediaIdsProvided {
		return errx.NewWithCode(errx.ParamError)
	}

	return nil
}

// postFieldUpdate 是合并现值后的写入字段，以及事件需要的完整标题、正文与新状态。
type postFieldUpdate struct {
	fields         map[string]any
	title, content string
	status         int64
}

// mergePostFields 把请求与现值合并并校验；只有显式提供的字段进入写入集合。
func (l *UpdatePostLogic) mergePostFields(in *pb.UpdatePostReq, post *model.Post, mediaURLs []string) (*postFieldUpdate, error) {
	mergedTitle := post.Title
	if in.GetTitle() != "" {
		mergedTitle = in.GetTitle()
	}
	mergedContent := post.Content
	if in.GetContent() != "" {
		mergedContent = in.GetContent()
	}
	if err := validatePostText(mergedTitle, mergedContent); err != nil {
		return nil, err
	}

	// 校验图片
	for _, image := range in.Images {
		if strings.ContainsRune(image, ',') {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}

	// PATCH 语义：只写入客户端显式传入的字段，避免静默清空现有值；
	// title/content 写入的是与现值合并后的结果。
	fields := map[string]any{
		"title":   mergedTitle,
		"content": mergedContent,
	}
	if err := l.mergePostMedia(in, post, mediaURLs, fields); err != nil {
		return nil, err
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

	return &postFieldUpdate{fields: fields, title: mergedTitle, content: mergedContent, status: newStatus}, nil
}

// postTagUpdate 描述标签是否替换、写入的标签与 ID，以及事件携带的标签。
type postTagUpdate struct {
	replace       bool
	event, values []string
	ids           []int64
}

func (l *UpdatePostLogic) tagsForUpdate(in *pb.UpdatePostReq, post *model.Post) (*postTagUpdate, error) {
	var err error
	// 标签仅在显式提供时替换；缺省时保留现有标签并让事件沿用旧值，
	// 避免 title-only 更新静默清空标签（B3）。模型层在 replaceTags=false
	// 时不触碰 post_tag，因此传空切片。
	replaceTags := in.Tags != nil
	var eventTags []string
	var modelTags []string
	var modelTagIDs []int64
	if replaceTags {
		modelTags, modelTagIDs, err = allocateTagIDs(l.Logger, in.Tags)
		if err != nil {
			return nil, err
		}
		eventTags = modelTags
	} else {
		eventTags, err = l.svcCtx.PostTagModel.FindTagNamesByPostId(l.ctx, post.Id)
		if err != nil {
			l.Errorw("find existing tags for update failed",
				logging.Field("postId", post.Id), logging.Field("err", err.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}

	return &postTagUpdate{replace: replaceTags, event: eventTags, values: modelTags, ids: modelTagIDs}, nil
}
