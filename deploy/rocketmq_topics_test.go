package deploy

import (
	"os"
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
