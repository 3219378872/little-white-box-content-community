//go:build integration

package assets

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

// 与 media 集成测试一致：S3 从 TEST_S3_ENDPOINT 读取（缺省本地 SeaweedFS）。
func TestPrivateObjectRoundTripAndIdempotentDelete(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "127.0.0.1:8333"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	storage, err := New(ctx, Config{
		Endpoint: endpoint, AccessKey: "xbh-media", SecretKey: "xbh-media-secret", Region: "us-east-1",
		PrivateBucket: "xbh-ad-test-private", PublicBucket: "xbh-ad-test-public",
		PublicBaseURL: "http://" + endpoint + "/xbh-ad-test-public",
	})
	require.NoError(t, err)

	key := PrivateKey(time.Now().UnixNano())
	require.NoError(t, storage.PutPrivate(ctx, key, []byte("payload"), "image/png"))
	content, err := storage.GetPrivate(ctx, key)
	require.NoError(t, err)
	require.Equal(t, []byte("payload"), content)

	require.NoError(t, storage.DeletePrivate(ctx, key))
	require.NoError(t, storage.DeletePrivate(ctx, key), "deleting a missing object is a no-op")
	_, err = storage.cli.StatObject(ctx, storage.private, key, minio.StatObjectOptions{})
	require.Error(t, err, "object %s must be gone", strconv.Quote(key))
}
