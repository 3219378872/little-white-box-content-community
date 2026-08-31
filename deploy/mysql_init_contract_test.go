package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestMySQLInitMountsOnlyMySQLSchemasAndUsesAuthenticatedHealthcheck(t *testing.T) {
	raw, err := os.ReadFile("docker-compose.middleware.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)
	if strings.Contains(compose, "./sql:/docker-entrypoint-initdb.d") {
		t.Fatal("MySQL must not mount the mixed SQL directory")
	}
	for _, schema := range []string{
		"xbh_assistant.sql", "xbh_content.sql", "xbh_feed.sql", "xbh_interaction.sql",
		"xbh_media.sql", "xbh_message.sql", "xbh_user.sql",
	} {
		if !strings.Contains(compose, "./sql/"+schema+":/docker-entrypoint-initdb.d/") {
			t.Errorf("MySQL init mount is missing %s", schema)
		}
	}
	if count := strings.Count(compose, "./sql/xbh_analytics.sql:/docker-entrypoint-initdb.d/xbh_analytics.sql:ro"); count != 1 {
		t.Fatalf("ClickHouse schema mount count=%d, want exactly one ClickHouse mount", count)
	}
	if !strings.Contains(compose, `MYSQL_PWD=\"$${MYSQL_ROOT_PASSWORD}\" mysqladmin ping`) {
		t.Fatal("MySQL healthcheck must authenticate with the configured root password")
	}
}
