package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestProductionUpOnlyChecksMigrationState(t *testing.T) {
	raw, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(raw)
	start := strings.Index(makefile, "production-up:")
	end := strings.Index(makefile[start:], "\nproduction-down:")
	if start < 0 || end < 0 {
		t.Fatal("production-up target not found")
	}
	target := makefile[start : start+end]
	if !strings.Contains(target, "production-migration-check") {
		t.Fatal("production-up must check migration state")
	}
	if strings.Contains(target, "production-migrate") {
		t.Fatal("production-up must not apply SQL patches")
	}
}

func TestProductionMigrationBackupIsSeparateFromApply(t *testing.T) {
	raw, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(raw)
	start := strings.Index(makefile, "production-migration-backup:")
	end := strings.Index(makefile[start:], "\nproduction-migrate:")
	if start < 0 || end < 0 {
		t.Fatal("production-migration-backup target not found")
	}
	target := makefile[start : start+end]
	if !strings.Contains(target, "--prepare-destructive-backup") {
		t.Fatal("backup target must stop after preparing the verified backup")
	}
}

func TestProductionMigrationRequiresTargetLedgerAndVerifiedBackup(t *testing.T) {
	raw, err := os.ReadFile("../scripts/apply_production_sql_patches.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, required := range []string{
		"PRODUCTION_MYSQL_SERVER_UUID",
		"SELECT @@server_uuid",
		"schema_patch_ledger",
		"sha256sum",
		"--check",
		"PRODUCTION_DESTRUCTIVE_MIGRATION_CONFIRM",
		"RESET_ASSISTANT_RUNTIME_V3",
		"PRODUCTION_MIGRATION_BACKUP_DIR",
		"mysqldump",
		"gzip -t",
		"agent_capability_consent",
		"--prepare-destructive-backup",
		"assistant-v3-pre-reset.ready",
		"patch_sha256",
		"destructive_confirm_prefix:",
		"dedicated directory outside the repository",
		"read_env_file_value",
		"permissions must not grant group or other access",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("migration script is missing %q", required)
		}
	}
}

func TestAssistantRetentionIndexesCoverEveryGlobalPurge(t *testing.T) {
	baseline, err := os.ReadFile("sql/xbh_assistant.sql")
	if err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile("sql/patches/20260831_assistant_retention_indexes.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"idx_msg_retention", "idx_watch_hit_created", "idx_watch_exec_created"} {
		if !strings.Contains(string(baseline), index) {
			t.Errorf("Assistant baseline is missing %s", index)
		}
		if !strings.Contains(string(patch), index) {
			t.Errorf("Assistant retention patch is missing %s", index)
		}
	}
}
