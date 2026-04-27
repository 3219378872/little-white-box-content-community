package logic

import (
	"context"
	"errors"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"testing"

	"errx"

	"github.com/dtm-labs/dtm/client/dtmcli"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestQueryPreparedLogic_MissingDTMMetadataReturnsSystemError(t *testing.T) {
	logic := NewQueryPreparedLogic(context.Background(), &svc.ServiceContext{})

	_, err := logic.QueryPrepared(&pb.QueryPreparedReq{})

	require.Error(t, err)
	require.True(t, errx.Is(err, errx.SystemError))
}

func TestQueryPreparedErrorPreservesDTMControlErrors(t *testing.T) {
	err := queryPreparedError(dtmcli.ErrFailure)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Aborted, st.Code())

	err = queryPreparedError(dtmcli.ErrOngoing)
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestQueryPreparedErrorWrapsGenericErrorsAsSystemError(t *testing.T) {
	err := queryPreparedError(errors.New("db is unavailable"))

	require.True(t, errx.Is(err, errx.SystemError))
}
