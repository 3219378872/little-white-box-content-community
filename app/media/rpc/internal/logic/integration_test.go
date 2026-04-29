//go:build integration

package logic

import (
	"errx"
	"esx/app/media/rpc/internal/config"
	"esx/app/media/rpc/internal/storage"
	"esx/app/media/rpc/internal/svc"
	"esx/pkg/testutil"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

var testEnv *testutil.TestEnv
var testSvcCtx *svc.ServiceContext

func TestMain(m *testing.M) {
	testEnv = testutil.SetupTestEnvM("xbh_media", testutil.SchemaPath("xbh_media.sql"))

	// S3 storage 仍从环境变量读取（MinIO/SeaweedFS 端到端暂不纳入 testcontainers）
	s3Endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if s3Endpoint == "" {
		s3Endpoint = "127.0.0.1:8333"
	}

	cfg := config.Config{
		RpcServerConf: zrpc.RpcServerConf{},
		DataSource:    testEnv.MySQLDSN,
		S3Storage: storage.Config{
			Endpoint:      s3Endpoint,
			AccessKey:     "xbh-media",
			SecretKey:     "xbh-media-secret",
			UseSSL:        false,
			Region:        "us-east-1",
			Bucket:        "xbh-media-test",
			PublicBaseURL: "http://" + s3Endpoint + "/xbh-media-test",
		},
		Upload: config.UploadConf{
			MaxImageSize:      10 * 1024 * 1024,
			MaxVideoSize:      100 * 1024 * 1024,
			DefaultQuality:    85,
			ThumbnailLongSide: 256,
			TempDir:           "",
		},
	}
	cfg.Redis.RedisConf = redis.RedisConf{
		Host: testEnv.RedisAddr,
		Type: "node",
	}

	testSvcCtx = svc.NewServiceContext(cfg)

	truncateAll()
	code := m.Run()
	truncateAll()
	testEnv.Close()
	os.Exit(code)
}

func truncateAll() {
	for _, t := range []string{"media", "media_task"} {
		if _, err := testSvcCtx.Conn.Exec("DELETE FROM `" + t + "`"); err != nil {
			fmt.Fprintf(os.Stderr, "truncate %s 失败: %v\n", t, err)
			os.Exit(1)
		}
	}
}

func assertBizError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errx.Is(err, expectedCode),
		"期望错误码 %d，实际错误: %v", expectedCode, err)
}
