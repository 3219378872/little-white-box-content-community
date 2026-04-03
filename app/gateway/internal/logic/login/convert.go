package login

import (
	"gateway/internal/types"
	"user/pb/xiaobaihe/user/pb"
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
		UserId: resp.UserId,
		Token:  resp.Token,
	}
}
