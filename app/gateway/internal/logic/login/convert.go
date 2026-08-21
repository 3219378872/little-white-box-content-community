package login

import (
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
)

func RegisterReqConvert(req *types.RegisterReq) *pb.RegisterReq {
	return &pb.RegisterReq{
		Username:   req.Username,
		Password:   req.Password,
		Phone:      req.Phone,
		VerifyCode: req.VerifyCode,
	}
}

func RegisterRespConvert(resp *pb.RegisterResp) *types.RegisterResp {
	return &types.RegisterResp{
		UserId:       resp.UserId,
		Token:        resp.Token,
		RefreshToken: resp.RefreshToken,
	}
}
