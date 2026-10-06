package logic

import (
	"context"
	model2 "esx/app/message/rpc/internal/model"
	"esx/app/message/rpc/internal/svc"
	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/message"
	"strings"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

type GetConversationsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewGetConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetConversationsLogic {
	return &GetConversationsLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// 获取会话列表
func (l *GetConversationsLogic) GetConversations(in *pb.GetConversationsReq) (*pb.GetConversationsResp, error) {
	if in == nil || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	page, pageSize := normalizePage(in.Page, in.PageSize)
	rows, total, err := l.svcCtx.ConversationModel.FindByUser(l.ctx, in.UserId, page, pageSize)
	if err != nil {
		l.Errorw("ConversationModel.FindByUser failed", logging.Field("err", err.Error()))
		return nil, errx.Wrap(err, errx.SystemError)
	}
	targetIDs := uniqueConversationTargetIDs(rows)
	profiles := make(map[int64]*userservice.UserCard, len(targetIDs))
	if len(targetIDs) > 0 {
		if l.svcCtx.UserService == nil {
			l.Error("UserService is not configured")
			return nil, errx.NewWithCode(errx.SystemError)
		}
		users, err := l.svcCtx.UserService.BatchGetUserCards(l.ctx, &userservice.BatchGetUserCardsReq{UserIds: targetIDs})
		if err != nil {
			l.Errorw("UserService.BatchGetUserCards failed", logging.Field("err", err.Error()))
			return nil, errx.Wrap(err, errx.SystemError)
		}
		if users == nil {
			l.Error("UserService.BatchGetUserCards returned a nil response")
			return nil, errx.NewWithCode(errx.SystemError)
		}
		for _, profile := range users.Users {
			if profile != nil && profile.Id > 0 {
				profiles[profile.Id] = profile
			}
		}
	}
	items := make([]*pb.ConversationInfo, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		item := toConversationInfo(row)
		if profile := profiles[row.TargetUserId]; profile != nil {
			item.TargetUserName = conversationDisplayName(profile)
			item.TargetUserAvatar = profile.AvatarUrl
		}
		items = append(items, item)
	}
	return &pb.GetConversationsResp{Conversations: items, Total: total}, nil
}

func uniqueConversationTargetIDs(rows []*model2.Conversation) []int64 {
	ids := make([]int64, 0, len(rows))
	seen := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if row == nil || row.TargetUserId <= 0 {
			continue
		}
		if _, exists := seen[row.TargetUserId]; exists {
			continue
		}
		seen[row.TargetUserId] = struct{}{}
		ids = append(ids, row.TargetUserId)
	}
	return ids
}

func conversationDisplayName(profile *userservice.UserCard) string {
	if profile == nil {
		return ""
	}
	if nickname := strings.TrimSpace(profile.Nickname); nickname != "" {
		return nickname
	}
	return strings.TrimSpace(profile.Username)
}
