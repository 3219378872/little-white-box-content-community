package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const migrationTestServerUUID = "11111111-2222-3333-4444-555555555555"

func TestProductionMigrationScriptRequiresPreparedBackupBeforeConfirmation(t *testing.T) {
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	fakeBin := filepath.Join(tmp, "bin")
	if err := os.Mkdir(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeDocker := filepath.Join(fakeBin, "docker")
	if err := os.WriteFile(fakeDocker, []byte(`#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_DOCKER_LOG"
args="$*"
last="${!#}"
if [[ "$args" == *" up -d --wait mysql"* ]]; then
  exit 0
fi
if [[ "$args" == *"mysqldump"* ]]; then
  if [[ "$args" == *"--databases xbh_assistant"* ]]; then
    printf '%s\n' '-- xbh_assistant verified dump' 'CREATE DATABASE xbh_assistant;'
  else
    printf '%s\n' '-- agent_capability_consent verified dump' 'CREATE TABLE agent_capability_consent (id BIGINT);'
  fi
  exit 0
fi
if [[ "$args" == *"--batch --skip-column-names -e"* ]]; then
  case "$last" in
    'SELECT @@server_uuid;') printf '%s\n' "$FAKE_MYSQL_UUID" ;;
    *"table_schema='xbh_schema_migrations'"*) printf '0\n' ;;
    *"table_schema='xbh_assistant' AND table_name='runtime_marker'"*) printf '0\n' ;;
    *"FROM xbh_schema_migrations.schema_patch_ledger"*) ;;
    *"INSERT INTO xbh_schema_migrations.schema_patch_ledger"*) ;;
    *) printf 'unexpected fake mysql query: %s\n' "$last" >&2; exit 90 ;;
  esac
  exit 0
fi
if [[ "$args" == *"exec mysql -uroot --default-character-set=utf8mb4"* ]]; then
  input="$(</dev/stdin)"
  if [[ "$input" == *"schema_patch_ledger"* ]]; then
    printf 'ledger\n' >>"$FAKE_DOCKER_LOG"
  else
    printf 'patch\n' >>"$FAKE_DOCKER_LOG"
  fi
  exit 0
fi
printf 'unexpected fake docker invocation: %s\n' "$args" >&2
exit 91
`), 0o700); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(tmp, "backups")
	envFile := filepath.Join(tmp, "production.env")
	writeMigrationEnv(t, envFile, migrationTestServerUUID, backupDir, "")
	baseEnv := migrationCommandEnv(fakeBin, envFile, filepath.Join(tmp, "docker.log"))
	script := filepath.Join(repo, "scripts", "apply_production_sql_patches.sh")

	wrongEnv := filepath.Join(tmp, "wrong.env")
	writeMigrationEnv(t, wrongEnv, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", filepath.Join(tmp, "wrong-backups"), "")
	wrongOutput, wrongErr := runMigrationScript(repo, script, []string{"--prepare-destructive-backup"}, migrationCommandEnv(fakeBin, wrongEnv, filepath.Join(tmp, "wrong.log")))
	if wrongErr == nil || !strings.Contains(wrongOutput, "server UUID does not match") {
		t.Fatalf("wrong target must fail before backup: err=%v output=%s", wrongErr, wrongOutput)
	}

	preparedOutput, err := runMigrationScript(repo, script, []string{"--prepare-destructive-backup"}, baseEnv)
	if err != nil {
		t.Fatalf("prepare backup: %v\n%s", err, preparedOutput)
	}
	confirmationMatch := regexp.MustCompile(`PRODUCTION_DESTRUCTIVE_MIGRATION_CONFIRM=(RESET_ASSISTANT_RUNTIME_V3:[0-9a-f]{64})`).FindStringSubmatch(preparedOutput)
	if len(confirmationMatch) != 2 {
		t.Fatalf("prepared backup did not print a manifest-bound confirmation: %s", preparedOutput)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "assistant-v3-pre-reset.ready")); err != nil {
		t.Fatalf("prepared backup marker: %v", err)
	}

	unconfirmedOutput, unconfirmedErr := runMigrationScript(repo, script, nil, baseEnv)
	if unconfirmedErr == nil || !strings.Contains(unconfirmedOutput, "requires the confirmation printed") {
		t.Fatalf("unconfirmed apply must fail: err=%v output=%s", unconfirmedErr, unconfirmedOutput)
	}
	if !strings.Contains(unconfirmedOutput, confirmationMatch[1]) {
		t.Fatal("unconfirmed apply did not identify the exact prepared-backup confirmation")
	}
	logBeforeConfirm, err := os.ReadFile(filepath.Join(tmp, "docker.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(logBeforeConfirm), "\n") {
		if line == "ledger" || line == "patch" {
			t.Fatal("backup preparation or unconfirmed apply changed migration state")
		}
	}

	writeMigrationEnv(t, envFile, migrationTestServerUUID, backupDir, confirmationMatch[1])
	confirmedOutput, err := runMigrationScript(repo, script, nil, baseEnv)
	if err != nil {
		t.Fatalf("confirmed apply: %v\n%s", err, confirmedOutput)
	}
	if !strings.Contains(confirmedOutput, "production SQL migration completed") {
		t.Fatalf("confirmed apply did not complete: %s", confirmedOutput)
	}
}

func writeMigrationEnv(t *testing.T, path, serverUUID, backupDir, confirmation string) {
	t.Helper()
	content := "PRODUCTION_MYSQL_SERVER_UUID=" + serverUUID + "\n" +
		"PRODUCTION_MIGRATION_BACKUP_DIR=" + backupDir + "\n" +
		"PRODUCTION_DESTRUCTIVE_MIGRATION_CONFIRM=" + confirmation + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

func migrationCommandEnv(fakeBin, envFile, logFile string) []string {
	env := make([]string, 0, len(os.Environ())+5)
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "PRODUCTION_MYSQL_SERVER_UUID=") ||
			strings.HasPrefix(item, "PRODUCTION_MIGRATION_BACKUP_DIR=") ||
			strings.HasPrefix(item, "PRODUCTION_DESTRUCTIVE_MIGRATION_CONFIRM=") ||
			strings.HasPrefix(item, "PRODUCTION_ENV_FILE=") {
			continue
		}
		env = append(env, item)
	}
	return append(env,
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PRODUCTION_ENV_FILE="+envFile,
		"FAKE_DOCKER_LOG="+logFile,
		"FAKE_MYSQL_UUID="+migrationTestServerUUID,
	)
}

func runMigrationScript(repo, script string, args []string, env []string) (string, error) {
	cmd := exec.Command(script, args...)
	cmd.Dir = repo
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}
