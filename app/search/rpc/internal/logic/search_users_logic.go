package logic

import (
	"context"

	"esx/app/search/rpc/internal/svc"
	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/search"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// SearchUsersLogic 承载 SearchUsers 接口的业务逻辑；每个请求新建一个实例。
type SearchUsersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewSearchUsersLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewSearchUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchUsersLogic {
	return &SearchUsersLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// 搜索用户
func (l *SearchUsersLogic) SearchUsers(in *pb.SearchUsersReq) (*pb.SearchUsersResp, error) {
	if in == nil || !validPage(in.Page, in.PageSize) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	searchKeyword, err := keyword(in.Keyword)
	if err != nil {
		return nil, err
	}
	if l.svcCtx.UserService == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	result, err := l.svcCtx.UserService.SearchUsers(l.ctx, &userservice.SearchUsersReq{
		Keyword: searchKeyword, Page: in.Page, PageSize: in.PageSize,
	})
	if err != nil {
		l.Errorw("search users RPC failed", logging.Field("err", err.Error()))
		return nil, storeError(err)
	}
	if result == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	return &pb.SearchUsersResp{Users: userResults(result.Users), Total: result.Total}, nil
}
