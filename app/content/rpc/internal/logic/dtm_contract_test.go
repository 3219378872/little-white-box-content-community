package logic

import (
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"testing"
)

func TestQueryPreparedTypesRemainOnGeneratedContract(t *testing.T) {
	var _ = (*contentpb.QueryPreparedReq)(nil)
}
