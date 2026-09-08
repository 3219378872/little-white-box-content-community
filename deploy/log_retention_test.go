package deploy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLServiceContextsDisableParameterLogging(t *testing.T) {
	checked := 0
	err := filepath.WalkDir("../app", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "service_context.go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != "NewServiceContext" {
				continue
			}
			var disabled, opened token.Pos
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				qualifier, ok := selector.X.(*ast.Ident)
				if !ok || qualifier.Name != "sqlx" {
					return true
				}
				switch selector.Sel.Name {
				case "DisableLog":
					if disabled == token.NoPos {
						disabled = call.Pos()
					}
				case "NewMysql", "NewConn", "NewSqlConn", "NewSqlConnFromDB":
					if opened == token.NoPos {
						opened = call.Pos()
					}
				}
				return true
			})
			if opened != token.NoPos {
				checked++
				if disabled == token.NoPos || disabled > opened {
					t.Errorf("%s must disable normal, slow and error SQL parameter logs before opening a connection (REL-022)", path)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 10 {
		t.Fatalf("expected at least 10 SQL service contexts, checked %d", checked)
	}
}

// REL-022：业务日志最多保留 30 天（Loki retention_period）。
func TestLokiBusinessLogRetentionIsThirtyDays(t *testing.T) {
	data, err := os.ReadFile("loki/loki-config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	if !strings.Contains(config, "retention_period: 720h") {
		t.Error("loki config must set retention_period: 720h (30 days)")
	}
	if !strings.Contains(config, "retention_enabled: true") {
		t.Error("loki config must enable compactor retention")
	}
	if !strings.Contains(config, "delete_request_store: filesystem") {
		t.Error("loki config must set compactor.delete_request_store when retention is enabled")
	}
	if !strings.Contains(config, "schema: v13") {
		t.Error("loki config must use schema v13")
	}
	if !strings.Contains(config, "store: tsdb") {
		t.Error("loki config must use tsdb index store")
	}
}

// REL-022：禁止 grafana/loki:latest。3.x 默认校验会拒绝仓库曾用的 v11/boltdb-shipper。
func TestLokiImageIsPinned(t *testing.T) {
	data, err := os.ReadFile("docker-compose.middleware.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(data)
	if strings.Contains(compose, "grafana/loki:latest") {
		t.Error("loki image must not use :latest")
	}
	if !strings.Contains(compose, "grafana/loki:3.7.6") {
		t.Error("loki image must pin grafana/loki:3.7.6")
	}
}

// REL-021：安全访问日志最多保留 7 天（nginx access log 每日轮转 + 7 份）。
func TestNginxAccessLogRotationKeepsSevenDays(t *testing.T) {
	data, err := os.ReadFile("nginx/rotate-access-log.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if !strings.Contains(script, "kill -USR1") {
		t.Error("rotation script must signal nginx to reopen its log file")
	}
	if !strings.Contains(script, "-mtime +7 -delete") {
		t.Error("rotation script must delete rotated access logs older than 7 days")
	}

	compose, err := os.ReadFile("docker-compose.production.yml")
	if err != nil {
		t.Fatal(err)
	}
	composeText := string(compose)
	if !strings.Contains(composeText, "rotate-access-log.sh:/usr/local/bin/rotate-access-log.sh:ro") {
		t.Error("production compose must mount the access-log rotation script into nginx")
	}
	if !strings.Contains(composeText, "crond -b") {
		t.Error("production nginx must run crond for daily rotation")
	}
}

// REL-021：行为分析表不存完整客户端 IP（已改为 SHA-256 哈希）。
func TestBehaviorAnalyticsNeverStoresFullClientIP(t *testing.T) {
	store, err := os.ReadFile("../app/pipeline/behaviorlog/internal/store/clickhouse_store.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(store), "anonymizeIP") {
		t.Error("clickhouse store must anonymize the client IP before insert")
	}

	schema, err := os.ReadFile("sql/xbh_analytics.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), "SHA-256") {
		t.Error("analytics schema must document that client_ip stores a hash, not the full IP")
	}
}

// REL-022：每个 RPC 服务必须抑制框架自动的请求/回复内容日志，
// 避免私信正文、Assistant 输入、社区正文与认证字段进入业务日志。
func TestEveryRPCSuppressesContentLogging(t *testing.T) {
	dirs, err := filepath.Glob("../app/*/rpc/etc/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 10 {
		t.Fatalf("expected 10 RPC service configs, found %d", len(dirs))
	}
	for _, configPath := range dirs {
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		config := string(data)
		if !strings.Contains(config, "IgnoreContentMethods:") {
			t.Errorf("%s must set Middlewares.StatConf.IgnoreContentMethods (REL-022)", configPath)
			continue
		}
		if !strings.Contains(config, "Middlewares:") {
			t.Errorf("%s must declare Middlewares block", configPath)
		}
	}
}

// REL-022：关键敏感方法必须显式列入忽略列表。
func TestContentSensitiveMethodsAreIgnored(t *testing.T) {
	checks := []struct {
		config string
		method string
	}{
		{"../app/assistant/rpc/etc/assistant.yaml", "/assistant.AssistantService/PostMessage"},
		{"../app/assistant/rpc/etc/assistant.yaml", "/assistant.AssistantService/SubscribeRunEvents"},
		{"../app/message/rpc/etc/message.yaml", "/message.MessageService/SendMessage"},
		{"../app/content/rpc/etc/content.yaml", "/content.ContentService/CreatePost"},
		{"../app/content/rpc/etc/content.yaml", "/content.ContentService/CreateComment"},
		{"../app/user/rpc/etc/user.yaml", "/user.UserService/Login"},
	}
	for _, check := range checks {
		data, err := os.ReadFile(check.config)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), check.method) {
			t.Errorf("%s must ignore content for %s", check.config, check.method)
		}
	}
}
