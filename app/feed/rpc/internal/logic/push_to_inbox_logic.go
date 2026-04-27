package logic

import (
	"context"

	"errx"
	"esx/app/feed/internal/model"
	"esx/app/feed/internal/svc"
	"esx/app/feed/xiaobaihe/feed/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type PushToInboxLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewPushToInboxLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PushToInboxLogic {
	return &PushToInboxLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *PushToInboxLogic) PushToInbox(in *pb.PushToInboxReq) (*pb.PushToInboxResp, error) {
	if in.AuthorId <= 0 || in.PostId <= 0 || in.CreatedAt <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if len(in.FollowerIds) == 0 {
		return &pb.PushToInboxResp{PushedCount: 0}, nil
	}
	rows := make([]*model.FeedInbox, 0, len(in.FollowerIds))
	for _, followerID := range in.FollowerIds {
		if followerID <= 0 {
			continue
		}
		rows = append(rows, &model.FeedInbox{UserId: followerID, AuthorId: in.AuthorId, PostId: in.PostId, CreatedAt: in.CreatedAt})
	}
	if len(rows) == 0 {
		return &pb.PushToInboxResp{PushedCount: 0}, nil
	}
	affected, err := l.svcCtx.InboxModel.BatchInsertIgnore(l.ctx, rows)
	if err != nil {
		l.Errorw("InboxModel.BatchInsertIgnore failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &pb.PushToInboxResp{PushedCount: affected}, nil
}
