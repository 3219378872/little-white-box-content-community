package logic

import (
	"context"
	"encoding/json"
	"esx/app/content/rpc/internal/svc"
	pb "esx/kitex_gen/content"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/idempotencyx"
	"esx/pkg/mqx"
	"strings"

	"esx/pkg/logging"
)

// DeletePostLogic 承载 DeletePost 接口的业务逻辑；每个请求新建一个实例。
type DeletePostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewDeletePostLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewDeletePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePostLogic {
	return &DeletePostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// DeletePost 删除帖子（软删除，status=2）：幂等重放 → 读取并鉴权 → 事务内 CAS 删除并写 outbox → 清缓存。
func (l *DeletePostLogic) DeletePost(in *pb.DeletePostReq) (*pb.DeletePostResp, error) {
	if in.PostId <= 0 || in.AuthorId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	idem, err := deletePostIdempotency(in)
	if err != nil {
		return nil, err
	}
	idemModel, _, found, err := replayIdempotentPostCommand(l.ctx, l.svcCtx, idem)
	if err != nil {
		return nil, err
	}
	if found {
		return &pb.DeletePostResp{}, nil
	}

	post, err := loadOwnedPost(l.ctx, l.Logger, l.svcCtx, in.PostId, in.AuthorId, in.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	// 删除事件只携带身份与新修订号，下游据此下架帖子。
	outboxEvent, err := buildPostOutboxEvent(mqx.TopicPostDelete, event.PostEvent{
		Type:     event.PostEventDeleted,
		PostID:   post.Id,
		AuthorID: post.AuthorId,
		Revision: post.Revision + 1,
	})
	if err != nil {
		l.Errorw("build post-deleted event failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if l.svcCtx.PostCommandModel == nil {
		l.Errorw("PostCommandModel is nil")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if idem.Key != "" {
		_, err = idemModel.DeletePostIdempotent(l.ctx, post.Id, outboxEvent, in.ExpectedRevision, post.Revision+1, idem)
	} else {
		err = l.svcCtx.PostCommandModel.DeletePost(l.ctx, post.Id, outboxEvent, in.ExpectedRevision)
	}
	if err != nil {
		return nil, postCommandError(l.Logger, "delete", post.Id, err)
	}
	invalidatePostCacheAfter(l.ctx, l.Logger, l.svcCtx, "delete", post.Id)

	return &pb.DeletePostResp{}, nil
}

// deletePostIdempotency 以帖子、作者与期望修订号作为删除命令摘要。
func deletePostIdempotency(in *pb.DeletePostReq) (idempotencyx.IdempotencyRecord, error) {
	key := strings.TrimSpace(in.GetIdempotencyKey())
	payload, err := json.Marshal(struct {
		PostID           int64 `json:"post_id"`
		AuthorID         int64 `json:"author_id"`
		ExpectedRevision int64 `json:"expected_revision"`
	}{in.PostId, in.AuthorId, in.ExpectedRevision})
	if err != nil {
		return idempotencyx.IdempotencyRecord{}, errx.NewWithCode(errx.ParamError)
	}
	row := idempotencyx.IdempotencyRecord{Scope: "post:delete", UserID: in.AuthorId,
		Key: key, CommandHash: idempotencyx.CommandHash(string(payload))}
	if !row.Valid() {
		return idempotencyx.IdempotencyRecord{}, errx.NewWithCode(errx.ParamError)
	}
	return row, nil
}
