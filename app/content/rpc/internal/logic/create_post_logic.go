package logic

import (
	"context"
	"database/sql"
	"errx"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/event"
	"mqx"
	"strings"
	"time"
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
	if in.Title == "" {
		return nil, errx.NewWithCode(errx.TitleEmpty)
	}
	if in.Content == "" {
		return nil, errx.NewWithCode(errx.ContentEmpty)
	}
	// 校验图片url（不得含','，因为我们用逗号分隔存储）
	for _, image := range in.Images {
		if strings.ContainsRune(image, ',') {
			return nil, errx.NewWithCode(errx.ParamError)
		}
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
	})
	if err != nil {
		l.Errorw("build post-created event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.PostCommandModel.CreatePost(l.ctx, post, validTags, tagIds, outboxEvent); err != nil {
		l.Errorw("create post transaction failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.PostModel.InvalidatePostCache(l.ctx, id); err != nil {
		l.Errorw("invalidate post cache after create failed", logx.Field("postId", id), logx.Field("err", err.Error()))
	}

	return &pb.CreatePostResp{
		PostId: id,
	}, nil
}
