package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestRocketMQBrokerDisablesContainerSupport(t *testing.T) {
	body, err := os.ReadFile("docker-compose.middleware.yml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	broker := strings.Index(content, "  rocketmq-broker:")
	if broker < 0 {
		t.Fatal("rocketmq-broker service block not found")
	}
	console := strings.Index(content[broker:], "  rocketmq-console:")
	if console < 0 {
		t.Fatal("rocketmq-console service block not found after broker")
	}
	block := content[broker : broker+console]
	if !strings.Contains(block, `-XX:-UseContainerSupport`) {
		t.Fatal("rocketmq-broker JAVA_OPT_EXT must disable UseContainerSupport so StoreUtil can initialize on cgroup v2 hosts")
	}
}
