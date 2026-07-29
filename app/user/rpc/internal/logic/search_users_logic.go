package logic

import (
	"context"
	"strings"

	"errx"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchUsersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSearchUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchUsersLogic {
	return &SearchUsersLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 按公开资料搜索用户
func (l *SearchUsersLogic) SearchUsers(in *pb.SearchUsersReq) (*pb.SearchUsersResp, error) {
	if in == nil || in.Page <= 0 || in.PageSize <= 0 || in.PageSize > 100 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	keyword := strings.TrimSpace(in.Keyword)
	if keyword == "" || len([]rune(keyword)) > 100 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	offset := int64(in.Page-1) * int64(in.PageSize)
	if offset >= 10_000 || offset+int64(in.PageSize) > 10_000 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	profiles, total, err := l.svcCtx.UserProfileModel.SearchPublic(
		l.ctx, keyword, offset, int64(in.PageSize),
	)
	if err != nil {
		l.Errorw("UserProfileModel.SearchPublic failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	users := make([]*pb.UserInfo, 0, len(profiles))
	for _, profile := range profiles {
		if profile != nil {
			users = append(users, UserProfileToUserInfo(profile))
		}
	}

	return &pb.SearchUsersResp{Users: users, Total: total}, nil
}
