package logic

import (
	feedpb "esx/app/feed/rpc/xiaobaihe/feed/pb"
	"testing"
)

func TestFeedFanoutTypesCompile(t *testing.T) {
	var _ = (*feedpb.FanoutPostReq)(nil)
}
