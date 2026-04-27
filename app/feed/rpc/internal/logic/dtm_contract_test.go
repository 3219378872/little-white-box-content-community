package logic

import (
	"testing"

	feedpb "esx/app/feed/rpc/xiaobaihe/feed/pb"
)

func TestFeedDTMContractTypesCompile(t *testing.T) {
	var _ = (*feedpb.FanoutPostReq)(nil)
}
