package login

import (
	"esx/app/gateway/internal/types"
	pb "esx/kitex_gen/user"
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
