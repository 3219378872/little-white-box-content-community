//go:build integration

package logic

import (
	"context"
	"errx"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeleteMedia_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("成功软删", func(t *testing.T) {
		id := insertTestMedia(t, 3001, 1)
		l := NewDeleteMediaLogic(ctx, testSvcCtx)
		_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: id, UserId: 3001})
		require.NoError(t, err)

		gl := NewGetMediaLogic(ctx, testSvcCtx)
		_, err = gl.GetMedia(&pb.GetMediaReq{MediaId: id})
		assertBizError(t, err, errx.MediaNotFound)
	})

	t.Run("user_id 不匹配返 PermissionDenied", func(t *testing.T) {
		id := insertTestMedia(t, 3002, 1)
		l := NewDeleteMediaLogic(ctx, testSvcCtx)
		_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: id, UserId: 9999})
		assertBizError(t, err, errx.PermissionDenied)
	})

	t.Run("不存在返 MediaNotFound", func(t *testing.T) {
		l := NewDeleteMediaLogic(ctx, testSvcCtx)
		_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 99999999, UserId: 3003})
		assertBizError(t, err, errx.MediaNotFound)
	})

	t.Run("重复删除幂等", func(t *testing.T) {
		id := insertTestMedia(t, 3004, 1)
		l := NewDeleteMediaLogic(ctx, testSvcCtx)
		_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: id, UserId: 3004})
		require.NoError(t, err)
		_, err = l.DeleteMedia(&pb.DeleteMediaReq{MediaId: id, UserId: 3004})
		require.NoError(t, err, "重复删除应幂等不报错")
	})

	t.Run("非法参数", func(t *testing.T) {
		l := NewDeleteMediaLogic(ctx, testSvcCtx)
		_, err := l.DeleteMedia(&pb.DeleteMediaReq{MediaId: 0, UserId: 1})
		assertBizError(t, err, errx.ParamError)
	})
}
