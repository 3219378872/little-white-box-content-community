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

func TestUploadAssetRollsBackUnrecordedObjects(t *testing.T) {
	content := pngBytes(t)
	upload := func(t *testing.T, ctx context.Context, fs *fakeStore, objects *fakeAssets) error {
		t.Helper()
		svcCtx, _, _ := newSvcCtx(fs)
		svcCtx.Assets = objects
		_, err := NewUploadAssetLogic(ctx, svcCtx).UploadAsset(&pb.UploadAssetReq{
			UserId: 1, Kind: store.AssetCreative, Content: content,
		})
		return err
	}
	key := assets.PrivateKey(77)

	t.Run("record absent: object deleted", func(t *testing.T) {
		fs, objects := &fakeStore{assetID: 77, err: errBoom, getAssetErr: store.ErrNotFound}, &fakeAssets{}
		requireCode(t, upload(t, context.Background(), fs, objects), errx.SystemError)
		require.Equal(t, []string{key}, objects.deleted)
		require.NotContains(t, objects.objects, key)
	})

	t.Run("record committed despite the error: object kept", func(t *testing.T) {
		fs := &fakeStore{assetID: 77, err: errBoom, recorded: &store.Asset{ID: 77, ObjectKey: key}}
		objects := &fakeAssets{}
		requireCode(t, upload(t, context.Background(), fs, objects), errx.SystemError)
		require.Empty(t, objects.deleted)
		require.Contains(t, objects.objects, key)
	})

	t.Run("record state unknown: object kept", func(t *testing.T) {
		fs, objects := &fakeStore{assetID: 77, err: errBoom}, &fakeAssets{}
		requireCode(t, upload(t, context.Background(), fs, objects), errx.SystemError)
		require.Empty(t, objects.deleted)
	})

	t.Run("cleanup failure keeps the original error", func(t *testing.T) {
		fs := &fakeStore{assetID: 77, err: store.ErrAssetInvalid, getAssetErr: store.ErrNotFound}
		objects := &fakeAssets{deleteErr: errBoom}
		requireCode(t, upload(t, context.Background(), fs, objects), errx.AdMediaInvalid)
		require.Equal(t, []string{key}, objects.deleted)
	})

	t.Run("cleanup survives a canceled request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		fs, objects := &fakeStore{assetID: 77, err: errBoom, getAssetErr: store.ErrNotFound}, &fakeAssets{}
		requireCode(t, upload(t, ctx, fs, objects), errx.SystemError)
		require.Equal(t, []string{key}, objects.deleted)
	})
}

func TestUploadAssetReplayDeletesTheDuplicateObject(t *testing.T) {
	first := &store.Asset{ID: 5, UserID: 1, Kind: store.AssetCreative, SHA256: "abc", MimeType: "image/png", ObjectKey: assets.PrivateKey(5)}
	fs := &fakeStore{assetID: 77, replayed: first}
	svcCtx, objects, _ := newSvcCtx(fs)
	objects.objects = map[string][]byte{first.ObjectKey: []byte("original")}

	resp, err := NewUploadAssetLogic(context.Background(), svcCtx).UploadAsset(&pb.UploadAssetReq{
		UserId: 1, Kind: store.AssetCreative, Content: pngBytes(t), IdempotencyKey: "k",
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), resp.Asset.AssetId, "the replay answers with the first upload")
	require.Equal(t, []string{assets.PrivateKey(77)}, objects.deleted)
	require.Equal(t, []byte("original"), objects.objects[first.ObjectKey])

	objects.deleteErr = errBoom
	_, err = NewUploadAssetLogic(context.Background(), svcCtx).UploadAsset(&pb.UploadAssetReq{
		UserId: 1, Kind: store.AssetCreative, Content: pngBytes(t), IdempotencyKey: "k",
	})
	require.NoError(t, err, "a failed cleanup does not fail an idempotent replay")
}

func TestUploadAssetKeepsRecordedObjects(t *testing.T) {
	fs := &fakeStore{assetID: 77}
	svcCtx, objects, _ := newSvcCtx(fs)
	_, err := NewUploadAssetLogic(context.Background(), svcCtx).UploadAsset(&pb.UploadAssetReq{
		UserId: 1, Kind: store.AssetCreative, Content: pngBytes(t),
	})
	require.NoError(t, err)
	require.Empty(t, objects.deleted)
	require.Contains(t, objects.objects, assets.PrivateKey(77))
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
