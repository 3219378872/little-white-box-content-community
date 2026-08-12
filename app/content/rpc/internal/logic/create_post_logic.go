package logic

import (
	"context"
	"database/sql"
	"errors"
	"errx"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/event"
	"mqx"
	"strings"
	"time"
	"unicode/utf8"
	"util"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreatePostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePostLogic {
	return &CreatePostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CreatePost 创建帖子
func (l *CreatePostLogic) CreatePost(in *pb.CreatePostReq) (*pb.CreatePostResp, error) {
	// 校验基本字段
	if in.AuthorId <= 0 {
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
	if in.GetStatus() != 0 && in.GetStatus() != 1 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if len(in.Images) > 9 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	// 校验图片url（不得含','，因为我们用逗号分隔存储）
	for _, image := range in.Images {
		if strings.ContainsRune(image, ',') {
			return nil, errx.NewWithCode(errx.ParamError)
		}
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
	idempotencyKey := strings.TrimSpace(in.GetIdempotencyKey())
	idem := model.IdempotencyRecord{
		Scope:       "post:create",
		UserID:      in.AuthorId,
		Key:         idempotencyKey,
		CommandHash: model.CommandHash(in.GetTitle(), in.GetContent(), strings.Join(in.Images, ","), strings.Join(in.Tags, ",")),
	}
	if !idem.Valid() {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if err := validatePostMedia(l.ctx, l.Logger, l.svcCtx.MediaService, in.AuthorId, in.MediaIds); err != nil {
		return nil, err
	}
	// 生成分布式id
	id, err := util.NextID()
	if err != nil {
		return nil, errx.NewWithCode(errx.SystemError)
	}

	imageJsonString, err := util.ToJsonObject(in.Images).JsonString()
	if err != nil {
		l.Errorw("json convert images failed", logx.Field("err", err.Error()))
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
			Valid:  len(in.Images) > 0,
		},
	}

	// 收集有效标签并预生成分布式 ID
	validTags := make([]string, 0, len(in.Tags))
	tagIds := make([]int64, 0, len(in.Tags))
	for _, tag := range in.Tags {
		if tag == "" {
			continue
		}
		tid, idErr := util.NextID()
		if idErr != nil {
			l.Errorw("generate tag id failed", logx.Field("err", idErr.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
		validTags = append(validTags, tag)
		tagIds = append(tagIds, tid)
	}

	if l.svcCtx.PostCommandModel == nil {
		l.Errorw("PostCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	createdAt := time.Now().UnixMilli()
	bodyExcerpt := in.GetContent()
	if len(bodyExcerpt) > 256 {
		bodyExcerpt = bodyExcerpt[:256]
	}
	outboxEvent, err := buildPostOutboxEvent(mqx.TopicPostCreate, event.PostEvent{
		EventTime:   createdAt,
		Type:        event.PostEventCreated,
		PostID:      id,
		AuthorID:    in.GetAuthorId(),
		Title:       in.GetTitle(),
		BodyExcerpt: bodyExcerpt,
		Tags:        validTags,
		Status:      in.GetStatus(),
	})
	if err != nil {
		l.Errorw("build post-created event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	postID, created, err := l.svcCtx.PostCommandModel.CreatePost(l.ctx, post, validTags, tagIds, outboxEvent, idem)
	if err != nil {
		if errors.Is(err, model.ErrIdempotencyConflict) {
			return nil, errx.NewWithCode(errx.IdempotencyConflict)
		}
		l.Errorw("create post transaction failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if created {
		if err = l.svcCtx.PostModel.InvalidatePostCache(l.ctx, id); err != nil {
			l.Errorw("invalidate post cache after create failed", logx.Field("postId", id), logx.Field("err", err.Error()))
		}
	}

	return &pb.CreatePostResp{
		PostId:   postID,
		Status:   in.GetStatus(),
		Revision: 1,
	}, nil
}
