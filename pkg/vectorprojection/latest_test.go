package vectorprojection

import (
	"context"
	"errors"
	"testing"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/stretchr/testify/require"
)

type queryFunc func(context.Context, string, []string, string, []string, ...client.SearchQueryOptionFunc) (client.ResultSet, error)

func (f queryFunc) Query(c context.Context, s string, p []string, e string, o []string, opts ...client.SearchQueryOptionFunc) (client.ResultSet, error) {
	return f(c, s, p, e, o, opts...)
}
func TestLatestRequiresStrongCompleteVersionHistory(t *testing.T) {
	cases := []struct {
		name string
		rows client.ResultSet
		err  error
		want string
	}{
		{name: "error", err: errors.New("unavailable"), want: "unavailable"},
		{name: "old schema", rows: client.ResultSet{entity.NewColumnInt64("post_id", []int64{1})}, want: "revision metadata missing"},
		{name: "truncated", rows: client.ResultSet{entity.NewColumnInt64("post_id", make([]int64, MaxHistoryRows)), entity.NewColumnInt64("revision", make([]int64, MaxHistoryRows)), entity.NewColumnBool("deleted", make([]bool, MaxHistoryRows))}, want: "safety limit"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			q := queryFunc(func(_ context.Context, _ string, _ []string, _ string, _ []string, opts ...client.SearchQueryOptionFunc) (client.ResultSet, error) {
				options := &client.SearchQueryOption{}
				for _, opt := range opts {
					opt(options)
				}
				require.Equal(t, entity.ClStrong, options.ConsistencyLevel)
				require.Equal(t, int64(MaxHistoryRows), options.Limit)
				return tt.rows, tt.err
			})
			_, err := Latest(context.Background(), q, "test", []int64{1}, false)
			require.ErrorContains(t, err, tt.want)
		})
	}
}
