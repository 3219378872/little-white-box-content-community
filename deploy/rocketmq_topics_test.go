package deploy

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestRocketMQBootstrapUsesOnlyActiveTopicsAndGroups(t *testing.T) {
	raw, err := os.ReadFile("rocketmq/init-topics.sh")
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	for _, required := range []string{
		"post-create post-update post-delete",
		"user-behavior-v2",
		"message-push",
		"media-deleted",
		"behavior-log-service-group",
		"recommend-feature-service-group",
		"embedding-service-group",
		"content-cleanup-service-group",
		"content-count-sync-service-group",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("RocketMQ bootstrap is missing active contract %q", required)
		}
	}
	for _, retired := range []string{
		"user-behavior ",
		"user-follow",
		"user-unfollow",
		"comment-create",
		"comment-delete",
		"like unlike",
		"favorite unfavorite",
		"search-index",
		"search-delete",
		"feed-generate",
	} {
		if strings.Contains(content, retired) {
			t.Errorf("RocketMQ bootstrap still creates retired topic %q", retired)
		}
	}
}

// TestRocketMQTopicsMatchCodeConstants 保证 pkg/mqx/topics.go 中定义的主题常量
// 与 rocketmq/init-topics.sh 预创建的主题完全一致，防止代码与部署清单漂移。
func TestRocketMQTopicsMatchCodeConstants(t *testing.T) {
	topicsSrc, err := os.ReadFile("../pkg/mqx/topics.go")
	if err != nil {
		t.Fatalf("read topics.go: %v", err)
	}
	codeTopics := map[string]bool{}
	constant := regexp.MustCompile(`Topic[A-Za-z0-9]+ = "([a-z0-9-]+)"`)
	for _, match := range constant.FindAllStringSubmatch(string(topicsSrc), -1) {
		codeTopics[match[1]] = true
	}

	raw, err := os.ReadFile("rocketmq/init-topics.sh")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`TOPICS=\(([^)]*)\)`).FindStringSubmatch(string(raw))
	if block == nil {
		t.Fatal("init-topics.sh: TOPICS array not found")
	}
	scriptTopics := map[string]bool{}
	for _, word := range strings.Fields(block[1]) {
		scriptTopics[word] = true
	}

	if len(codeTopics) == 0 {
		t.Fatal("no topic constants found in pkg/mqx/topics.go")
	}
	for topic := range codeTopics {
		if !scriptTopics[topic] {
			t.Errorf("code topic %q is missing from init-topics.sh", topic)
		}
	}
	for topic := range scriptTopics {
		if !codeTopics[topic] {
			t.Errorf("init-topics.sh creates %q which is not declared in pkg/mqx/topics.go", topic)
		}
	}
}
