package comment

import (
	"context"
	"encoding/json"
	"esx/app/content/rpc/contentservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/jwtx"
	"testing"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type captureCommentTarget struct {
	contentservice.ContentService
	request *contentservice.CreateCommentReq
}

func (c *captureCommentTarget) CreateComment(_ context.Context, req *contentservice.CreateCommentReq, _ ...callopt.Option) (*contentservice.CreateCommentResp, error) {
	c.request = req
	return &contentservice.CreateCommentResp{CommentId: 57}, nil
}
func TestCreateCommentPreservesReplyTargetAcrossRPC(t *testing.T) {
	for _, tt := range []struct {
		body   string
		target int64
	}{
		{`{"postId":1,"parentId":55,"replyUserId":3,"content":"reply"}`, 0},
		{`{"postId":1,"parentId":55,"replyUserId":4,"replyToCommentId":56,"content":"reply"}`, 56},
	} {
		t.Run(tt.body, func(t *testing.T) {
			var req types.CreateCommentReq
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))
			content := new(captureCommentTarget)
			ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{UserId: 9})
			_, err := NewCreateCommentLogic(ctx, &svc.ServiceContext{ContentService: content}).CreateComment(&req)
			require.NoError(t, err)
			wire, err := proto.Marshal(content.request)
			require.NoError(t, err)
			decoded := new(contentservice.CreateCommentReq)
			require.NoError(t, proto.Unmarshal(wire, decoded))
			require.Equal(t, int64(55), decoded.ParentId)
			require.Equal(t, tt.target, decoded.ReplyToCommentId)
			require.Equal(t, req.ReplyUserId, decoded.ReplyUserId)
			require.Equal(t, int64(9), decoded.UserId)
		})
	}
}
