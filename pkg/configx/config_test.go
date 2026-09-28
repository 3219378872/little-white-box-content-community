package configx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKeepsExplicitValuesAndLiteralSecrets(t *testing.T) {
	type Config struct {
		Enabled bool `json:",default=true"`
		Count   int  `json:",default=10"`
		Secret  string
		Nested  struct {
			Enabled bool `json:",default=true"`
		}
	}
	t.Setenv("MIGRATION_SECRET", "123456\n# value: with YAML punctuation")
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte("Enabled: false\nCount: 0\nSecret: ${MIGRATION_SECRET}\nNested: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := Load(file, &cfg, UseEnv()); err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.Count != 0 || !cfg.Nested.Enabled || cfg.Secret != os.Getenv("MIGRATION_SECRET") {
		t.Fatalf("configuration default or literal secret was changed")
	}
}
func TestLoadRejectsRangeAndDuplicateKeys(t *testing.T) {
	type Config struct {
		Count int `json:",default=2,range=[1:3]"`
	}
	for _, body := range []string{"Count: 0\n", "Count: 4\n", "Count: 1\nCount: 2\n"} {
		file := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		var cfg Config
		if err := Load(file, &cfg); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}
