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
		"MODEL_S3_ENDPOINT: ${MODEL_S3_ENDPOINT:-http://seaweedfs:8333}",
		"MODEL_S3_ACCESS_KEY: ${MODEL_S3_ACCESS_KEY}",
		"MODEL_S3_SECRET_KEY: ${MODEL_S3_SECRET_KEY}",
	}
	for _, fragment := range required {
		if !strings.Contains(trainingServices, fragment) {
			t.Errorf("training services must source credentials from the environment; missing %q", fragment)
		}
	}

	forbidden := []string{
		"MODEL_S3_ACCESS_KEY: ${MODEL_S3_ACCESS_KEY:-",
		"MODEL_S3_SECRET_KEY: ${MODEL_S3_SECRET_KEY:-",
		"minio",
	}
	for _, fragment := range forbidden {
		if strings.Contains(trainingServices, fragment) {
			t.Errorf("training services contain %q", fragment)
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

func TestMiddlewareDoesNotDeployMinIO(t *testing.T) {
	body, err := os.ReadFile("docker-compose.middleware.yml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	for _, fragment := range []string{"image: minio/", "\n  minio:", "\n  minio-milvus:", "http://minio:"} {
		if strings.Contains(content, fragment) {
			t.Errorf("middleware compose still references MinIO: %q", fragment)
		}
	}
	if !strings.Contains(content, "MINIO_ADDRESS: seaweedfs:8333") {
		t.Error("milvus must use SeaweedFS S3 for object storage")
	}
}
