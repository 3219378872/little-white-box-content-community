package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type composeProject struct {
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	Build       *composeBuild                `json:"build"`
	DependsOn   map[string]composeDependency `json:"depends_on"`
	Environment map[string]string            `json:"environment"`
	Healthcheck *composeHealthcheck          `json:"healthcheck"`
	Ports       []composePort                `json:"ports"`
	Profiles    []string                     `json:"profiles"`
	Volumes     []composeVolume              `json:"volumes"`
}

type composeBuild struct {
	Dockerfile string            `json:"dockerfile"`
	Args       map[string]string `json:"args"`
}

type composeDependency struct {
	Condition string `json:"condition"`
}

type composeHealthcheck struct {
	Test []string `json:"test"`
}

type composePort struct {
	Published string `json:"published"`
	Protocol  string `json:"protocol"`
	Target    int    `json:"target"`
}

type composeVolume struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Type     string `json:"type"`
	ReadOnly bool   `json:"read_only"`
}

func TestProductionComposeParsesAndCoversRuntimeTopology(t *testing.T) {
	project := loadProductionCompose(t)

	runtimeServices := []string{
		"nginx", "gateway-a", "gateway-b",
		"user-rpc", "content-rpc", "media-rpc", "interaction-rpc", "feed-rpc",
		"message-rpc", "behavior-rpc", "search-rpc", "recommend-rpc", "assistant-rpc",
		"feed-consumer", "media-consumer", "behavior-log-consumer",
		"recommend-consumer", "search-consumer", "embedding-consumer", "content-cleanup-consumer",
		"embedding-service", "online-infer",
	}
	for _, name := range runtimeServices {
		service, ok := project.Services[name]
		if !ok {
			t.Errorf("production service %q is missing", name)
			continue
		}
		if !contains(service.Profiles, "production") {
			t.Errorf("service %q is not enabled by the production profile: %v", name, service.Profiles)
		}
		if service.Healthcheck == nil || len(service.Healthcheck.Test) == 0 {
			t.Errorf("service %q has no executable healthcheck", name)
		}
	}

	goServices := runtimeServices[1:20]
	for _, name := range goServices {
		service := project.Services[name]
		if service.Build == nil || service.Build.Dockerfile != "deploy/Dockerfile.service" {
			t.Errorf("Go service %q does not use the generic production Dockerfile", name)
			continue
		}
		if service.Build.Args["SERVICE_PATH"] == "" || service.Build.Args["SERVICE_CONFIG"] == "" {
			t.Errorf("Go service %q is missing build path/config arguments", name)
		}
	}

	nginx := project.Services["nginx"]
	for _, gateway := range []string{"gateway-a", "gateway-b"} {
		dependency, ok := nginx.DependsOn[gateway]
		if !ok || dependency.Condition != "service_healthy" {
			t.Errorf("nginx must wait for healthy %s", gateway)
		}
		if len(project.Services[gateway].Ports) != 0 {
			t.Errorf("%s must not publish a host port; only nginx is public", gateway)
		}
	}
	if project.Services["milvus"].Environment["MINIO_ACCESS_KEY_ID"] == "" ||
		project.Services["milvus"].Environment["MINIO_SECRET_ACCESS_KEY"] == "" {
		t.Error("milvus must receive the injected credentials used by minio-milvus")
	}
	if got := project.Services["online-infer"].Environment["MODEL_REGISTRY_MANIFEST_URI"]; got != "s3://xbh-models/recommend-models/rank-production/manifest.json" {
		t.Errorf("online-infer model registry manifest = %q", got)
	}
	if got := project.Services["online-infer"].Environment["MODEL_MANIFEST_JSON"]; got != "[]" {
		t.Errorf("production online-infer local model manifest = %q, want disabled", got)
	}
	if got := project.Services["online-infer"].Environment["MODEL_TRAFFIC_JSON"]; got != "{}" {
		t.Errorf("production online-infer static model traffic = %q, want disabled", got)
	}
	if got := project.Services["assistant-rpc"].Environment["ASSISTANT_LLM_WIRE_API"]; got != "responses" {
		t.Errorf("production assistant LLM wire API = %q, want responses", got)
	}
}

func TestProductionComposeHostPortsAndNginxUpstreamsDoNotConflict(t *testing.T) {
	project := loadProductionCompose(t)
	published := make(map[string]string)
	for serviceName, service := range project.Services {
		for _, port := range service.Ports {
			key := port.Protocol + ":" + port.Published
			if previous, exists := published[key]; exists {
				t.Errorf("host port %s is published by both %s and %s", key, previous, serviceName)
			}
			published[key] = serviceName
		}
	}

	config, err := os.ReadFile("nginx/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	upstreamPattern := regexp.MustCompile(`(?m)^\s*server\s+([a-zA-Z0-9.-]+):(\d+)\s+max_fails=`)
	matches := upstreamPattern.FindAllStringSubmatch(string(config), -1)
	if len(matches) != 2 {
		t.Fatalf("gateway upstream count = %d, want 2", len(matches))
	}

	seen := make(map[string]struct{})
	for _, match := range matches {
		host, port := match[1], match[2]
		if host == "127.0.0.1" || host == "localhost" {
			t.Errorf("nginx upstream %s:%s uses a host-network endpoint", host, port)
		}
		endpoint := host + ":" + port
		if _, exists := seen[endpoint]; exists {
			t.Errorf("duplicate nginx upstream %s", endpoint)
		}
		seen[endpoint] = struct{}{}
		if owner, exists := published["tcp:"+port]; exists {
			t.Errorf("nginx upstream port %s conflicts with host port published by %s", port, owner)
		}
	}
	for _, expected := range []string{"gateway-a:8888", "gateway-b:8888"} {
		if _, ok := seen[expected]; !ok {
			t.Errorf("nginx upstream %s is missing", expected)
		}
	}
}

func TestProductionNginxMountsTLSAndStaticAssetsReadOnly(t *testing.T) {
	nginx := loadProductionCompose(t).Services["nginx"]
	targets := make(map[string]composeVolume)
	for _, volume := range nginx.Volumes {
		targets[volume.Target] = volume
	}
	for _, target := range []string{
		"/etc/nginx/tls/tls.crt",
		"/etc/nginx/tls/tls.key",
		"/srv/www",
		"/etc/nginx/nginx.conf",
	} {
		volume, ok := targets[target]
		if !ok {
			t.Errorf("nginx mount %s is missing", target)
			continue
		}
		if volume.Type != "bind" || !volume.ReadOnly {
			t.Errorf("nginx mount %s must be a read-only bind mount: %+v", target, volume)
		}
	}

	production, err := os.ReadFile("docker-compose.production.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(production)
	for _, required := range []string{
		"${TLS_CERT_FILE:?", "${TLS_KEY_FILE:?", "${STATIC_ROOT:?",
		"${JWT_SECRET_KEY:?", "${MYSQL_ROOT_PASSWORD:?", "${REDIS_PASSWORD:?",
		"${MODEL_REGISTRY_MANIFEST_URI:?",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("production compose does not require %q", required)
		}
	}
	for _, forbidden := range []string{"Xbh@", "xbh-media-secret", "minioadmin"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("production compose contains a hard-coded credential marker %q", forbidden)
		}
	}
}

func loadProductionCompose(t *testing.T) composeProject {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required to parse the production Compose contract")
	}

	tempDir := t.TempDir()
	tlsCert := filepath.Join(tempDir, "tls.crt")
	tlsKey := filepath.Join(tempDir, "tls.key")
	staticRoot := filepath.Join(tempDir, "static")
	if err := os.Mkdir(staticRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	values := map[string]string{
		"ASSISTANT_LLM_API_KEY":                            "",
		"ASSISTANT_LLM_COMPLETION_COST_PER_MILLION_TOKENS": "0",
		"ASSISTANT_LLM_ENABLED":                            "false",
		"ASSISTANT_LLM_ENDPOINT":                           "",
		"ASSISTANT_LLM_MODEL":                              "",
		"ASSISTANT_LLM_PROMPT_COST_PER_MILLION_TOKENS":     "0",
		"ASSISTANT_LLM_WIRE_API":                           "responses",
		"DB_CONTENT":                                       "contract-content-dsn",
		"DB_FEED":                                          "contract-feed-dsn",
		"DB_INTERACTION":                                   "contract-interaction-dsn",
		"DB_MEDIA":                                         "contract-media-dsn",
		"DB_MESSAGE":                                       "contract-message-dsn",
		"DB_USER":                                          "contract-user-dsn",
		"FEED_CURSOR_SECRET":                               "contract-feed-cursor",
		"GRAFANA_PASSWORD":                                 "contract-grafana-password",
		"HTTP_PORT":                                        "18080",
		"HTTPS_PORT":                                       "18443",
		"JWT_SECRET_KEY":                                   "contract-jwt-secret",
		"JWT_REFRESH_SECRET":                               "contract-jwt-refresh-secret",
		"MILVUS_MINIO_ROOT_PASSWORD":                       "contract-milvus-minio-password",
		"MILVUS_MINIO_ROOT_USER":                           "contract-milvus-minio-user",
		"MINIO_ROOT_PASSWORD":                              "contract-minio-password",
		"MINIO_ROOT_USER":                                  "contract-minio-user",
		"MODEL_REGISTRY_MANIFEST_URI":                      "s3://xbh-models/recommend-models/rank-production/manifest.json",
		"MYSQL_ROOT_PASSWORD":                              "contract-mysql-password",
		"RECOMMEND_CURSOR_SECRET":                          "contract-recommend-cursor",
		"REDIS_PASSWORD":                                   "contract-redis-password",
		"RPC_INTERNAL_SECRET":                              "contract-rpc-internal-secret",
		"S3_ACCESS_KEY":                                    "contract-s3-access",
		"S3_PUBLIC_BASE_URL":                               "https://media.example.test",
		"S3_SECRET_KEY":                                    "contract-s3-secret",
		"STATIC_ROOT":                                      staticRoot,
		"TLS_CERT_FILE":                                    tlsCert,
		"TLS_KEY_FILE":                                     tlsKey,
	}

	cmd := exec.Command("docker", "compose",
		"-f", "docker-compose.middleware.yml",
		"-f", "docker-compose.production.yml",
		"--profile", "production",
		"config", "--format", "json",
	)
	cmd.Env = replaceEnvironment(os.Environ(), values)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker compose config failed: %v\n%s", err, stderr.String())
	}

	var project composeProject
	if err := json.Unmarshal(output, &project); err != nil {
		t.Fatalf("decode docker compose config: %v", err)
	}
	return project
}

func replaceEnvironment(current []string, values map[string]string) []string {
	result := make([]string, 0, len(current)+len(values))
	for _, item := range current {
		key, _, _ := strings.Cut(item, "=")
		if _, replaced := values[key]; !replaced {
			result = append(result, item)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return result
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
