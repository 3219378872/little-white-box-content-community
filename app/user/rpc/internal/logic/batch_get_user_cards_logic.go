package logic

import (
	"context"

	"esx/app/user/rpc/internal/svc"
	pb "esx/kitex_gen/user"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

type BatchGetUserCardsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewBatchGetUserCardsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchGetUserCardsLogic {
	return &BatchGetUserCardsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// 批量获取作者展示卡片（缓存优先，未命中合并为一次主键查询）
func (l *BatchGetUserCardsLogic) BatchGetUserCards(in *pb.BatchGetUserCardsReq) (*pb.BatchGetUserCardsResp, error) {
	if in == nil || len(in.UserIds) == 0 || len(in.UserIds) > maxBatchGetUsers {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	ids := make([]int64, 0, len(in.UserIds))
	seen := make(map[int64]struct{}, len(in.UserIds))
	for _, id := range in.UserIds {
		if id <= 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	cards, err := l.svcCtx.UserProfileModel.FindCardsByIDs(l.ctx, ids)
	if err != nil {
		l.Errorw("UserProfileModel.FindCardsByIDs failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	users := make([]*pb.UserCard, 0, len(cards))
	for _, card := range cards {
		if card != nil {
			users = append(users, &pb.UserCard{
				Id: card.Id, Username: card.Username, Nickname: card.Nickname, AvatarUrl: card.AvatarUrl,
			})
		}
	}
	return &pb.BatchGetUserCardsResp{Users: users}, nil
}
