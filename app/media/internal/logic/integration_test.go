//go:build integration

package logic

import (
	"errx"
	"fmt"
	"os"
	"testing"

	"esx/app/media/internal/config"
	"esx/app/media/internal/storage"
	"esx/app/media/internal/svc"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

var testSvcCtx *svc.ServiceContext

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func TestMain(m *testing.M) {
	dsn := getEnv("TEST_MYSQL_DSN",
		"root:root@tcp(127.0.0.1:3306)/xbh_media?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai")
	redisHost := getEnv("TEST_REDIS_HOST", "127.0.0.1:6379")
	redisPass := getEnv("TEST_REDIS_PASS", "")
	s3Endpoint := getEnv("TEST_S3_ENDPOINT", "127.0.0.1:8333")
	s3AK := getEnv("TEST_S3_AK", "xbh-media")
	s3SK := getEnv("TEST_S3_SK", "xbh-media-secret")
	s3Bucket := getEnv("TEST_S3_BUCKET", "xbh-media-test")

	cfg := config.Config{
		RpcServerConf: zrpc.RpcServerConf{},
		DataSource:    dsn,
		S3Storage: storage.Config{
			Endpoint:      s3Endpoint,
			AccessKey:     s3AK,
			SecretKey:     s3SK,
			UseSSL:        false,
			Region:        "us-east-1",
			Bucket:        s3Bucket,
			PublicBaseURL: "http://" + s3Endpoint + "/" + s3Bucket,
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
		Host: redisHost,
		Pass: redisPass,
		Type: "node",
	}

	testSvcCtx = svc.NewServiceContext(cfg)
	if _, err := testSvcCtx.Conn.Exec("SELECT 1"); err != nil {
		fmt.Fprintf(os.Stderr, "数据库连接失败: %v\n", err)
		os.Exit(1)
	}

	truncateAll()
	code := m.Run()
	truncateAll()
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
