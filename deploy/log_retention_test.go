package deploy

import (
	"os"
	"strings"
	"testing"
)

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
