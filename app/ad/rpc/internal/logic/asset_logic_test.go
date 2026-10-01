package logic

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/store"
	pb "esx/kitex_gen/ad"
	"esx/pkg/errx"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	return buf.Bytes()
}

func TestUploadAssetStoresPrivatelyAndRecords(t *testing.T) {
	fs := &fakeStore{assetID: 77}
	svcCtx, objects, _ := newSvcCtx(fs)
	content := pngBytes(t)

	resp, err := NewUploadAssetLogic(context.Background(), svcCtx).UploadAsset(&pb.UploadAssetReq{
		UserId: 1, Kind: store.AssetCreative, Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, int64(77), resp.Asset.AssetId)
	require.Equal(t, "image/png", resp.Asset.MimeType)
	require.Equal(t, int64(len(content)), resp.Asset.Size)
	key := assets.PrivateKey(77)
	require.Equal(t, content, objects.objects[key])
	require.Equal(t, key, fs.createdAsset.ObjectKey)
	require.Equal(t, int64(1), fs.createdAsset.UserID)
}

func TestUploadAssetFailures(t *testing.T) {
	content := pngBytes(t)
	cases := map[string]struct {
		req   *pb.UploadAssetReq
		setup func(*fakeStore, *fakeAssets)
		code  int
	}{
		"unknown kind": {req: &pb.UploadAssetReq{UserId: 1, Kind: "avatar", Content: content}, code: errx.ParamError},
		"long idempotency": {req: &pb.UploadAssetReq{
			UserId: 1, Kind: store.AssetCreative, Content: content, IdempotencyKey: strings.Repeat("k", 129),
		}, code: errx.ParamError},
		"empty content":   {req: &pb.UploadAssetReq{UserId: 1, Kind: store.AssetCreative}, code: errx.ParamError},
		"not an image":    {req: &pb.UploadAssetReq{UserId: 1, Kind: store.AssetCreative, Content: []byte("plain text")}, code: errx.FileTypeNotAllowed},
		"id allocation":   {req: &pb.UploadAssetReq{UserId: 1, Kind: store.AssetDocument, Content: content}, setup: func(fs *fakeStore, _ *fakeAssets) { fs.idErr = errBoom }, code: errx.SystemError},
		"private storage": {req: &pb.UploadAssetReq{UserId: 1, Kind: store.AssetCreative, Content: content}, setup: func(_ *fakeStore, a *fakeAssets) { a.putErr = errBoom }, code: errx.SystemError},
		"record":          {req: &pb.UploadAssetReq{UserId: 1, Kind: store.AssetCreative, Content: content}, setup: func(fs *fakeStore, _ *fakeAssets) { fs.err = errBoom }, code: errx.SystemError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fs := &fakeStore{assetID: 1}
			svcCtx, objects, _ := newSvcCtx(fs)
			if tc.setup != nil {
				tc.setup(fs, objects)
			}
			_, err := NewUploadAssetLogic(context.Background(), svcCtx).UploadAsset(tc.req)
			requireCode(t, err, tc.code)
		})
	}
}

func TestReadAssetOnlyForOwnerOrAuthorizedReviewer(t *testing.T) {
	ctx := context.Background()
	asset := &store.Asset{ID: 5, UserID: 1, ObjectKey: "assets/5", MimeType: "image/png"}
	fs := &fakeStore{asset: asset}
	svcCtx, objects, _ := newSvcCtx(fs)
	objects.objects = map[string][]byte{"assets/5": []byte("bytes")}

	resp, err := NewReadAssetLogic(ctx, svcCtx).ReadAsset(&pb.ReadAssetReq{UserId: 1, AssetId: 5})
	require.NoError(t, err)
	require.Equal(t, []byte("bytes"), resp.Content)
	require.Equal(t, "image/png", resp.MimeType)

	_, err = NewReadAssetLogic(ctx, svcCtx).ReadAsset(&pb.ReadAssetReq{UserId: 2, AssetId: 5})
	requireCode(t, err, errx.NotFound)

	resp, err = NewReadAssetLogic(ctx, svcCtx).ReadAsset(&pb.ReadAssetReq{UserId: 2, AssetId: 5, ReviewerAuthorized: true})
	require.NoError(t, err)
	require.Equal(t, []byte("bytes"), resp.Content)

	objects.getErr = errBoom
	_, err = NewReadAssetLogic(ctx, svcCtx).ReadAsset(&pb.ReadAssetReq{UserId: 1, AssetId: 5})
	requireCode(t, err, errx.SystemError)

	svcCtx, _, _ = newSvcCtx(&fakeStore{err: store.ErrNotFound})
	_, err = NewReadAssetLogic(ctx, svcCtx).ReadAsset(&pb.ReadAssetReq{UserId: 1, AssetId: 5})
	requireCode(t, err, errx.NotFound)
}
