package tool

import (
	"context"
	"esx/app/content/rpc/contentservice"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type captureUpdateTags struct {
	contentservice.ContentService
	request *contentservice.UpdatePostReq
}

func (c *captureUpdateTags) UpdatePost(_ context.Context, req *contentservice.UpdatePostReq, _ ...callopt.Option) (*contentservice.UpdatePostResp, error) {
	c.request = req
	return &contentservice.UpdatePostResp{Revision: 4}, nil
}
func TestUpdatePostExecutorPreservesTagsAcrossRPC(t *testing.T) {
	for _, tt := range []struct {
		args    string
		present bool
		count   int
	}{
		{`{"post_id":1,"title":"title","expected_revision":3}`, false, 0},
		{`{"post_id":1,"tags":[],"expected_revision":3}`, true, 0},
		{`{"post_id":1,"tags":["tag"],"expected_revision":3}`, true, 1},
	} {
		t.Run(tt.args, func(t *testing.T) {
			service := new(captureUpdateTags)
			_, _, err := updatePostExecutor(service, nil)(context.Background(), &Session{UserID: 1, RequestID: "request"}, "call", tt.args)
			require.NoError(t, err)
			wire, err := proto.Marshal(service.request)
			require.NoError(t, err)
			decoded := new(contentservice.UpdatePostReq)
			require.NoError(t, proto.Unmarshal(wire, decoded))
			require.Equal(t, tt.present, decoded.TagsProvided)
			require.Len(t, decoded.Tags, tt.count)
		})
	}
}
