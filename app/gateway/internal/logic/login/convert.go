package login

import (
	"esx/app/gateway/internal/types"
	pb "esx/kitex_gen/user"
)

// RegisterReqConvert 把 REST 注册请求映射为用户服务 RPC 请求。
func RegisterReqConvert(req *types.RegisterReq) *pb.RegisterReq {
	return &pb.RegisterReq{
		Username:   req.Username,
		Password:   req.Password,
		Phone:      req.Phone,
		VerifyCode: req.VerifyCode,
	}
}

// RegisterRespConvert 把注册结果映射为 REST 响应，携带首次登录的令牌对。
func RegisterRespConvert(resp *pb.RegisterResp) *types.RegisterResp {
	return &types.RegisterResp{
		UserId:       resp.UserId,
		Token:        resp.Token,
		RefreshToken: resp.RefreshToken,
	}
}
