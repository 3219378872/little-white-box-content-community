package logic

import (
	"context"
	"testing"

	"errx"
	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/stretchr/testify/require"
)

func TestQueryPreparedLogic_MissingDTMMetadataReturnsSystemError(t *testing.T) {
	logic := NewQueryPreparedLogic(context.Background(), &svc.ServiceContext{})

	_, err := logic.QueryPrepared(&pb.QueryPreparedReq{})

	require.Error(t, err)
	require.True(t, errx.Is(err, errx.SystemError))
}
