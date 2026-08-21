package logic

import (
	"context"
	"testing"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeUserTagModel struct {
	tags map[int64][]*model.UserTag
	err  error
}

func (f *fakeUserTagModel) FindByUserId(_ context.Context, userID int64) ([]*model.UserTag, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tags[userID], nil
}

func TestGetUserTagsReturnsTagsOrderedByWeight(t *testing.T) {
	svcCtx := &svc.ServiceContext{UserTagModel: &fakeUserTagModel{tags: map[int64][]*model.UserTag{
		7: {
			{UserId: 7, TagName: "golang", Weight: 3},
			{UserId: 7, TagName: "go-zero", Weight: 1},
		},
	}}}
	resp, err := NewGetUserTagsLogic(context.Background(), svcCtx).GetUserTags(&pb.GetUserTagsReq{UserId: 7})
	require.NoError(t, err)
	assert.Equal(t, []string{"golang", "go-zero"}, resp.Tags)
}

func TestGetUserTagsEmptyForUserWithoutTags(t *testing.T) {
	svcCtx := &svc.ServiceContext{UserTagModel: &fakeUserTagModel{tags: map[int64][]*model.UserTag{}}}
	resp, err := NewGetUserTagsLogic(context.Background(), svcCtx).GetUserTags(&pb.GetUserTagsReq{UserId: 8})
	require.NoError(t, err)
	assert.Empty(t, resp.Tags)
}

func TestGetUserTagsInvalidParam(t *testing.T) {
	_, err := NewGetUserTagsLogic(context.Background(), &svc.ServiceContext{}).GetUserTags(&pb.GetUserTagsReq{UserId: 0})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ParamError))
}
