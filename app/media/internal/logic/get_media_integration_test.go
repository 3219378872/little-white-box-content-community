//go:build integration

package logic

import (
	"context"
	"errx"
	"testing"

	"esx/app/media/internal/model"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertTestMedia(t *testing.T, userId, status int64) int64 {
	t.Helper()
	row := &model.Media{
		UserId:      userId,
		FileName:    "test.jpg",
		FileType:    "image",
		Url:         "http://example.com/test.jpg",
		StorageType: 3,
		Status:      status,
	}
	res, err := testSvcCtx.MediaModel.Insert(context.Background(), row)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func TestGetMedia_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("成功获取", func(t *testing.T) {
		id := insertTestMedia(t, 1001, 1)
		l := NewGetMediaLogic(ctx, testSvcCtx)
		resp, err := l.GetMedia(&pb.GetMediaReq{MediaId: id})
		require.NoError(t, err)
		assert.Equal(t, id, resp.Media.Id)
		assert.Equal(t, int64(1001), resp.Media.UserId)
	})

	t.Run("id 非法", func(t *testing.T) {
		l := NewGetMediaLogic(ctx, testSvcCtx)
		_, err := l.GetMedia(&pb.GetMediaReq{MediaId: 0})
		assertBizError(t, err, errx.ParamError)
	})

	t.Run("不存在返 MediaNotFound", func(t *testing.T) {
		l := NewGetMediaLogic(ctx, testSvcCtx)
		_, err := l.GetMedia(&pb.GetMediaReq{MediaId: 99999999})
		assertBizError(t, err, errx.MediaNotFound)
	})

	t.Run("软删返 MediaNotFound", func(t *testing.T) {
		id := insertTestMedia(t, 1002, 0)
		l := NewGetMediaLogic(ctx, testSvcCtx)
		_, err := l.GetMedia(&pb.GetMediaReq{MediaId: id})
		assertBizError(t, err, errx.MediaNotFound)
	})
}
