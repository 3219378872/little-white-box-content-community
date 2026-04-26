package logic

import feedpb "esx/app/feed/xiaobaihe/feed/pb"

func requireFanoutPostContract() {
	var _ = (*feedpb.FanoutPostReq)(nil)
}
