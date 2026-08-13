package logic

import (
	"context"
	"testing"

	"errx"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"

	"github.com/stretchr/testify/require"
)

func TestQueryPreparedLogic_ReturnsSystemError(t *testing.T) {
	logic := NewQueryPreparedLogic(context.Background(), &svc.ServiceContext{})

	resp, err := logic.QueryPrepared(&pb.QueryPreparedReq{})

	require.Nil(t, resp)
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.SystemError))
}
