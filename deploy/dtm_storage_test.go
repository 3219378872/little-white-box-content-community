package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestDTMServiceUsesMySQLStorage(t *testing.T) {
	body, err := os.ReadFile("docker-compose.middleware.yml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	start := strings.Index(content, "  dtm:")
	if start < 0 {
		t.Fatal("dtm service block not found")
	}
	end := strings.Index(content[start+1:], "\n  # Jaeger")
	if end < 0 {
		t.Fatal("dtm service block terminator not found")
	}
	block := content[start : start+1+end]
	if !strings.Contains(block, "STORE_DRIVER: mysql") {
		t.Fatalf("dtm must use mysql storage, block:\n%s", block)
	}
	if strings.Contains(block, "STORE_DRIVER: redis") || strings.Contains(block, "STORE_HOST: redis") {
		t.Fatalf("dtm block still contains redis storage config:\n%s", block)
	}
	if !strings.Contains(block, `STORE_DSN: "${DTM_STORE_DSN}"`) {
		t.Fatalf("dtm block must use DTM_STORE_DSN env placeholder, block:\n%s", block)
	}
	if !strings.Contains(block, "mysql:") || !strings.Contains(block, "condition: service_healthy") {
		t.Fatalf("dtm must depend on healthy mysql, block:\n%s", block)
	}
}
