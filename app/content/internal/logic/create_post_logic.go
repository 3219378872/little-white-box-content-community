package logic

import (
	"context"
	"database/sql"
	"errx"
	"esx/app/content/internal/model"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"fmt"
	"strings"
	"util"

	"esx/app/content/internal/svc"

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
		return nil, err
	}
	// 插入帖子
	if err = l.svcCtx.PostModel.InsertPost(l.ctx, &model.Post{
		Id:       id,
		AuthorId: in.GetAuthorId(),
		Title:    in.GetTitle(),
		Content:  in.GetContent(),
		Status:   int64(in.GetStatus()),
		Images: sql.NullString{
			String: imageJsonString,
			Valid:  len(in.Images) > 0,
		},
	}); err != nil {
		return nil, fmt.Errorf("创建帖子失败: %w", err)
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
			return nil, fmt.Errorf("生成标签ID失败: %w", idErr)
		}
		validTags = append(validTags, tag)
		tagIds = append(tagIds, tid)
	}

	// 事务内批量插入标签，全部成功或全部回滚
	if err = l.svcCtx.PostTagModel.BatchInsertTagsByPostId(l.ctx, l.svcCtx.Conn, id, validTags, tagIds); err != nil {
		return nil, fmt.Errorf("创建帖子标签失败: %w", err)
	}

	return &pb.CreatePostResp{
		PostId: id,
	}, nil
}
