package logic

import contentpb "esx/app/content/pb/xiaobaihe/content/pb"

func requireQueryPreparedContract() {
	var _ = (*contentpb.QueryPreparedReq)(nil)
}
