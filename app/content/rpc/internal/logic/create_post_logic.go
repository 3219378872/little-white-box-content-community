package logic

import (
	"context"
	"database/sql"
	"errors"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/idempotencyx"
	"esx/pkg/mqx"
	"esx/pkg/util"
	"strconv"
	"strings"
	"time"

	"esx/pkg/logging"
)

type CreatePostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewCreatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePostLogic {
	return &CreatePostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// CreatePost 创建帖子：校验 → 幂等记录 → 校验媒体归属 → 组装帖子与标签 →
// 在同一事务内写帖子、标签、outbox 事件与幂等记录 → 首次创建时清理缓存。
func (l *CreatePostLogic) CreatePost(in *pb.CreatePostReq) (*pb.CreatePostResp, error) {
	if err := validateCreatePost(in); err != nil {
		return nil, err
	}
	idem := createPostIdempotency(in)
	if !idem.Valid() {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	// 媒体必须属于作者且已上传完成，校验通过的媒体 URL 与客户端图片合并为帖子图片。
	mediaURLs, err := validatePostMedia(l.ctx, l.Logger, l.svcCtx.MediaService, in.AuthorId, in.MediaIds)
	if err != nil {
		return nil, err
	}
	images, err := createPostImages(in.Images, mediaURLs)
	if err != nil {
		return nil, err
	}
	post, err := l.newPost(in, images)
	if err != nil {
		return nil, err
	}
	validTags, tagIds, err := allocateTagIDs(l.Logger, in.Tags)
	if err != nil {
		return nil, err
	}

	if l.svcCtx.PostCommandModel == nil {
		l.Errorw("PostCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	// 帖子创建事件随事务写入 outbox，保证下游（搜索、推荐等）不会漏掉已提交的帖子。
	outboxEvent, err := buildPostOutboxEvent(mqx.TopicPostCreate, postCreatedEvent(in, post.Id, validTags, time.Now().UnixMilli()))
	if err != nil {
		l.Errorw("build post-created event failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	// 幂等重放时返回首次创建的帖子 ID，created=false。
	postID, created, err := l.svcCtx.PostCommandModel.CreatePost(l.ctx, post, validTags, tagIds, outboxEvent, idem)
	if err != nil {
		if errors.Is(err, idempotencyx.ErrIdempotencyConflict) {
			return nil, errx.NewWithCode(errx.IdempotencyConflict)
		}
		l.Errorw("create post transaction failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if created {
		invalidatePostCacheAfter(l.ctx, l.Logger, l.svcCtx, "create", post.Id)
	}

	return &pb.CreatePostResp{
		PostId:   postID,
		Status:   in.GetStatus(),
		Revision: 1,
	}, nil
}

// createPostIdempotency 以帖子内容各字段的哈希作为命令摘要：同一幂等键配不同内容会被判为冲突。
func createPostIdempotency(in *pb.CreatePostReq) idempotencyx.IdempotencyRecord {
	return idempotencyx.IdempotencyRecord{
		Scope:  "post:create",
		UserID: in.AuthorId,
		Key:    strings.TrimSpace(in.GetIdempotencyKey()),
		CommandHash: idempotencyx.CommandHash(
			in.GetTitle(), in.GetContent(), strings.Join(in.Images, ","), strings.Join(in.Tags, ","),
			strings.Join(sortedMediaIDs(in.MediaIds), ","), strconv.FormatInt(int64(in.GetStatus()), 10),
		),
	}
}

// postCreatedEvent 组装帖子创建事件；新帖修订号固定为 1，统计序号取创建时间。
func postCreatedEvent(in *pb.CreatePostReq, postID int64, tags []string, createdAt int64) event.PostEvent {
	content := in.GetContent()
	return event.PostEvent{
		EventTime:   createdAt,
		Type:        event.PostEventCreated,
		PostID:      postID,
		AuthorID:    in.GetAuthorId(),
		Title:       in.GetTitle(),
		Body:        content,
		BodyExcerpt: runePrefix(content, postEventExcerptRunes),
		Tags:        tags,
		Status:      in.GetStatus(),
		Revision:    1,
		CreatedAt:   createdAt,
		StatsSeq:    createdAt,
	}
}

// validateCreatePost 在任何外部调用前校验请求字段；图片 URL 不得含逗号，因为存储层按逗号分隔。
func validateCreatePost(in *pb.CreatePostReq) error {
	if in.AuthorId <= 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	if err := validatePostText(in.GetTitle(), in.GetContent()); err != nil {
		return err
	}
	if in.GetStatus() != 0 && in.GetStatus() != 1 {
		return errx.NewWithCode(errx.ParamError)
	}
	if err := validatePostCollections(in.Images, in.Tags, in.MediaIds); err != nil {
		return err
	}
	for _, image := range in.Images {
		if strings.ContainsRune(image, ',') {
			return errx.NewWithCode(errx.ParamError)
		}
	}
	return nil
}

// newPost 组装待写入的帖子行：预生成分布式 ID，图片与媒体 ID 以 JSON 存储。
func (l *CreatePostLogic) newPost(in *pb.CreatePostReq, images []string) (*model.Post, error) {
	id, err := util.NextID()
	if err != nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}

	imageJsonString, err := model.ToJSONObject(images).JSONString()
	if err != nil {
		l.Errorw("json convert images failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	mediaIDsJSON, err := encodeInt64sJSON(in.MediaIds)
	if err != nil {
		l.Errorw("json convert media ids failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	post := &model.Post{
		Id:       id,
		AuthorId: in.GetAuthorId(),
		Title:    in.GetTitle(),
		Content:  in.GetContent(),
		Status:   int64(in.GetStatus()),
		Revision: 1,
		Images: sql.NullString{
			String: imageJsonString,
			Valid:  len(images) > 0,
		},
		MediaIds: mediaIDsJSON,
	}

	return post, nil
}
