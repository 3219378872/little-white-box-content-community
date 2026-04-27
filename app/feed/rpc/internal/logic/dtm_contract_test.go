package logic

import (
	"testing"

	feedpb "esx/app/feed/xiaobaihe/feed/pb"
)

func TestFeedDTMContractTypesCompile(t *testing.T) {
	var _ = (*feedpb.FanoutPostReq)(nil)
}
