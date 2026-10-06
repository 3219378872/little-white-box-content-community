package logic

import (
	"context"
	"errors"
	"esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/visibilityx"

	"esx/pkg/logging"

	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"
)

const recordViewMetadata = "x-xbh-record-view"

// recordViewRequested 判断调用方是否通过 RPC 元数据要求记一次浏览。
func recordViewRequested(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	values := md.Get(recordViewMetadata)
	return len(values) > 0 && values[0] == "1"
}

// GetPostLogic 承载 GetPost 接口的业务逻辑；每个请求新建一个实例。
type GetPostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetPostLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostLogic {
	return &GetPostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetPost 获取帖子详情
func (l *GetPostLogic) GetPost(in *pb.GetPostReq) (*pb.GetPostResp, error) {
	if in.PostId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	post, err := l.svcCtx.PostModel.FindPostById(l.ctx, in.PostId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		l.Errorw("PostModel.FindPostById failed",
			logging.Field("postId", in.PostId),
			logging.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	// CORE-012/016：草稿仅作者可读；已删除或非公开状态对非作者统一返回不存在。
	// 已发布内容对所有人可见。
	if !visibilityx.IsPublished(int32(post.Status)) && in.GetUserId() != post.AuthorId {
		return nil, errx.NewWithCode(errx.ContentNotFound)
	}
	if recordViewRequested(l.ctx) && visibilityx.IsPublished(int32(post.Status)) {
		if err = l.svcCtx.PostModel.IncrViewCount(l.ctx, post.Id); err != nil {
			l.Errorw("IncrViewCount failed", logging.Field("postId", post.Id), logging.Field("err", err.Error()))
		} else {
			post.ViewCount++
		}
	}

	tags, err := l.svcCtx.PostTagModel.FindTagNamesByPostId(l.ctx, post.Id)
	if err != nil {
		l.Errorw("PostTagModel.FindTagNamesByPostId failed",
			logging.Field("postId", post.Id),
			logging.Field("err", err.Error()),
		)
		tags = []string{}
	}

	return &pb.GetPostResp{
		Post: PostToPostInfo(post, tags),
	}, nil
}
