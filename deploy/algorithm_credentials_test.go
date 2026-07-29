package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestTrainingServicesDoNotDefaultCredentials(t *testing.T) {
	body, err := os.ReadFile("docker-compose.middleware.yml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	start := strings.Index(content, "  model-registry-init:")
	if start < 0 {
		t.Fatal("model-registry-init service block not found")
	}
	trainingServices := content[start:]

	required := []string{
		"MINIO_ROOT_USER: ${MINIO_ROOT_USER}",
		"MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}",
		"MODEL_S3_ACCESS_KEY: ${MINIO_ROOT_USER}",
		"MODEL_S3_SECRET_KEY: ${MINIO_ROOT_PASSWORD}",
	}
	for _, fragment := range required {
		if !strings.Contains(trainingServices, fragment) {
			t.Errorf("training services must source credentials from the environment; missing %q", fragment)
		}
	}

	forbidden := []string{
		"MINIO_ROOT_USER: ${MINIO_ROOT_USER:-",
		"MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD:-",
		"MODEL_S3_ACCESS_KEY: ${MINIO_ROOT_USER:-",
		"MODEL_S3_SECRET_KEY: ${MINIO_ROOT_PASSWORD:-",
	}
	for _, fragment := range forbidden {
		if strings.Contains(trainingServices, fragment) {
			t.Errorf("training services contain a credential fallback %q", fragment)
		}
	}

	nonSecretDefaults := []string{
		"MODEL_REGISTRY_BUCKET: ${MODEL_REGISTRY_BUCKET:-xbh-models}",
		"CLICKHOUSE_DSN: ${CLICKHOUSE_DSN:-http://clickhouse:8123/xbh_analytics}",
		"MODEL_REGISTRY_PREFIX: ${MODEL_REGISTRY_PREFIX:-recommend-models}",
	}
	for _, fragment := range nonSecretDefaults {
		if !strings.Contains(trainingServices, fragment) {
			t.Errorf("training services lost non-secret default %q", fragment)
		}
	}
}
