//go:build integration

package logic

import (
	"context"
	"esx/app/content/rpc/internal/config"
	"esx/app/content/rpc/internal/svc"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"fmt"
	"os"
	"testing"

	"errx"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

var testSvcCtx *svc.ServiceContext

// getEnv 读取环境变量，未设置时返回 defaultVal
func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func TestMain(m *testing.M) {
	dsn := getEnv("TEST_MYSQL_DSN",
		"root:root@tcp(127.0.0.1:3306)/xbh_content?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai")
	redisHost := getEnv("TEST_REDIS_HOST", "127.0.0.1:6379")
	redisPass := getEnv("TEST_REDIS_PASS", "")

	cfg := config.Config{
		RpcServerConf: zrpc.RpcServerConf{},
		DataSource:    dsn,
	}
	cfg.Redis.RedisConf = redis.RedisConf{
		Host: redisHost,
		Pass: redisPass,
		Type: "node",
	}

	testSvcCtx = svc.NewServiceContext(cfg)

	// 验证数据库连接
	if _, err := testSvcCtx.Conn.Exec("SELECT 1"); err != nil {
		fmt.Fprintf(os.Stderr, "数据库连接失败: %v\n", err)
		os.Exit(1)
	}

	truncateAll()
	seedTags()

	code := m.Run()

	truncateAll()
	os.Exit(code)
}

func truncateAll() {
	tables := []string{"post_tag", "comment", "post", "tag"}
	for _, table := range tables {
		if _, err := testSvcCtx.Conn.Exec("DELETE FROM `" + table + "`"); err != nil {
			fmt.Fprintf(os.Stderr, "truncate %s 失败: %v\n", table, err)
			os.Exit(1)
		}
	}
}

func seedTags() {
	seeds := []struct {
		name      string
		postCount int
	}{
		{"golang", 100},
		{"python", 80},
		{"rust", 50},
	}
	for _, s := range seeds {
		_, err := testSvcCtx.Conn.Exec(
			"INSERT INTO `tag` (`name`, `post_count`, `status`) VALUES (?, ?, 1)",
			s.name, s.postCount,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seedTags 插入 %s 失败: %v\n", s.name, err)
			os.Exit(1)
		}
	}
}

func createTestPost(t *testing.T, authorId int64, title, content string, tags []string) int64 {
	t.Helper()
	ctx := context.Background()
	l := NewCreatePostLogic(ctx, testSvcCtx)
	resp, err := l.CreatePost(&pb.CreatePostReq{
		AuthorId: authorId,
		Title:    title,
		Content:  content,
		Tags:     tags,
		Status:   1,
	})
	require.NoError(t, err)
	require.NotZero(t, resp.PostId)
	return resp.PostId
}

func createTestComment(t *testing.T, postId, userId int64, content string) int64 {
	t.Helper()
	ctx := context.Background()
	l := NewCreateCommentLogic(ctx, testSvcCtx)
	resp, err := l.CreateComment(&pb.CreateCommentReq{
		PostId:  postId,
		UserId:  userId,
		Content: content,
	})
	require.NoError(t, err)
	require.NotZero(t, resp.CommentId)
	return resp.CommentId
}

func assertBizError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errx.Is(err, expectedCode),
		"期望错误码 %d，实际错误: %v", expectedCode, err)
}
