//go:build integration

package logic

import (
	"context"
	"errx"
	"testing"

	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchGetMedia_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("5 条正常", func(t *testing.T) {
		ids := make([]int64, 0, 5)
		for i := 0; i < 5; i++ {
			ids = append(ids, insertTestMedia(t, 2001, 1))
		}
		l := NewBatchGetMediaLogic(ctx, testSvcCtx)
		resp, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: ids})
		require.NoError(t, err)
		assert.Len(t, resp.Medias, 5)
	})

	t.Run("混合软删静默跳过", func(t *testing.T) {
		a := insertTestMedia(t, 2002, 1)
		b := insertTestMedia(t, 2002, 0) // 软删
		c := insertTestMedia(t, 2002, 1)
		l := NewBatchGetMediaLogic(ctx, testSvcCtx)
		resp, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: []int64{a, b, c}})
		require.NoError(t, err)
		assert.Len(t, resp.Medias, 2)
	})

	t.Run("空入参返 ParamError", func(t *testing.T) {
		l := NewBatchGetMediaLogic(ctx, testSvcCtx)
		_, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: []int64{}})
		assertBizError(t, err, errx.ParamError)
	})

	t.Run("超过 100 返 ParamError", func(t *testing.T) {
		ids := make([]int64, 101)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		l := NewBatchGetMediaLogic(ctx, testSvcCtx)
		_, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: ids})
		assertBizError(t, err, errx.ParamError)
	})

	t.Run("含非法 id 返 ParamError", func(t *testing.T) {
		l := NewBatchGetMediaLogic(ctx, testSvcCtx)
		_, err := l.BatchGetMedia(&pb.BatchGetMediaReq{MediaIds: []int64{1, 0, 3}})
		assertBizError(t, err, errx.ParamError)
	})
}
