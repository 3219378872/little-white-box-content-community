package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestNginxProductionRoutingContract(t *testing.T) {
	data, err := os.ReadFile("nginx/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	required := []string{
		"listen 80",
		"listen 443 ssl",
		"ssl_certificate /etc/nginx/tls/tls.crt",
		"ssl_certificate_key /etc/nginx/tls/tls.key",
		"server gateway-a:8888",
		"server gateway-b:8888",
		"limit_req_zone $binary_remote_addr zone=api",
		"limit_req_zone $binary_remote_addr zone=behavior",
		"limit_req_zone $binary_remote_addr zone=assistant",
		"location /api/v1/",
		"location /api/v2/",
		"location = /api/v2/assistant/chat",
		"proxy_set_header Accept-Encoding ''",
		"proxy_buffering off",
		"proxy_cache off",
		"chunked_transfer_encoding on",
		"proxy_read_timeout 10m",
		"proxy_set_header X-Forwarded-For $remote_addr",
		"location = /healthz",
		"proxy_pass http://gateway_backend/api/v1/health",
		"root /srv/www",
		"try_files $uri $uri/ /index.html",
	}
	for _, fragment := range required {
		if !strings.Contains(config, fragment) {
			t.Errorf("nginx config is missing %q", fragment)
		}
	}
	if strings.Contains(config, "server 127.0.0.1:888") || strings.Contains(config, "server localhost:888") {
		t.Fatal("production nginx upstreams must use Compose service discovery, not loopback ports")
	}
	if strings.Contains(config, "$proxy_add_x_forwarded_for") {
		t.Fatal("production edge must overwrite client-supplied X-Forwarded-For")
	}
}
