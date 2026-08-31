#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: apply_production_sql_patches.sh [--check|--prepare-destructive-backup]

Without arguments, applies pending SQL patches and records their SHA-256 checksums.
With --check, performs a read-only pending/checksum check and fails if migration is required.
With --prepare-destructive-backup, writes and verifies the pre-reset backup, then exits
without applying SQL and prints the exact confirmation value for a later apply run.
EOF
}

mode=apply
case "${1:-}" in
  "") ;;
  --check) mode=check ;;
  --prepare-destructive-backup) mode=prepare-backup ;;
  -h|--help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
destructive_patch_name="20260829_assistant_runtime_v3.sql"
destructive_confirm_prefix="RESET_ASSISTANT_RUNTIME_V3"

read_env_file_value() {
  local key="$1"
  awk -v key="$key" '
    /^[[:space:]]*(#|$)/ { next }
    {
      line=$0
      sub(/^[[:space:]]*export[[:space:]]+/, "", line)
      prefix=key "="
      compact=line
      sub(/^[[:space:]]*/, "", compact)
      if (index(compact, prefix) != 1) next
      value=substr(compact, length(prefix)+1)
      sub(/[[:space:]]*$/, "", value)
      if (length(value) >= 2) {
        first=substr(value, 1, 1)
        last=substr(value, length(value), 1)
        if ((first == "\"" && last == "\"") || (first == "\047" && last == "\047")) {
          value=substr(value, 2, length(value)-2)
        }
      }
      found=value
      seen=1
    }
    END { if (seen) printf "%s", found }
  ' "$PRODUCTION_ENV_FILE"
}

load_migration_control() {
  local key="$1"
  if [[ -n "${!key+x}" || -z "${PRODUCTION_ENV_FILE:-}" ]]; then
    return
  fi
  local value
  value="$(read_env_file_value "$key")"
  printf -v "$key" '%s' "$value"
  export "$key"
}

if [[ -n "${PRODUCTION_ENV_FILE:-}" ]]; then
  if [[ ! -f "$PRODUCTION_ENV_FILE" || -L "$PRODUCTION_ENV_FILE" ]]; then
    echo "PRODUCTION_ENV_FILE must be a regular non-symlink file" >&2
    exit 1
  fi
  env_mode="$(stat -c '%a' "$PRODUCTION_ENV_FILE")"
  if (( (8#$env_mode & 8#077) != 0 )); then
    echo "PRODUCTION_ENV_FILE permissions must not grant group or other access" >&2
    exit 1
  fi
fi
load_migration_control PRODUCTION_MYSQL_SERVER_UUID
load_migration_control PRODUCTION_MIGRATION_BACKUP_DIR
load_migration_control PRODUCTION_DESTRUCTIVE_MIGRATION_CONFIRM

expected_uuid="${PRODUCTION_MYSQL_SERVER_UUID:-}"
if [[ ! "$expected_uuid" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]]; then
  echo "PRODUCTION_MYSQL_SERVER_UUID must be the exact target MySQL server UUID" >&2
  exit 1
fi

compose=(docker compose)
if [[ -n "${PRODUCTION_ENV_FILE:-}" ]]; then
  compose+=(--env-file "$PRODUCTION_ENV_FILE")
fi
compose+=(
  -f "$repo/deploy/docker-compose.middleware.yml"
  -f "$repo/deploy/docker-compose.production.yml"
  --profile production
)

mysql_query() {
  local query="$1"
  "${compose[@]}" exec -T mysql sh -ec \
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --default-character-set=utf8mb4 --batch --skip-column-names -e "$1"' \
    sh "$query"
}

mysql_stdin() {
  "${compose[@]}" exec -T mysql sh -ec \
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --default-character-set=utf8mb4'
}

"${compose[@]}" up -d --wait mysql

actual_uuid="$(mysql_query 'SELECT @@server_uuid;')"
if [[ "${actual_uuid,,}" != "${expected_uuid,,}" ]]; then
  echo "target MySQL server UUID does not match PRODUCTION_MYSQL_SERVER_UUID" >&2
  exit 1
fi

shopt -s nullglob
patches=("$repo"/deploy/sql/patches/*.sql)
shopt -u nullglob
if (( ${#patches[@]} == 0 )); then
  echo "no production SQL patches found" >&2
  exit 1
fi

ledger_exists="$(mysql_query "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='xbh_schema_migrations' AND table_name='schema_patch_ledger';")"
create_migration_ledger() {
  mysql_stdin <<'SQL'
CREATE DATABASE IF NOT EXISTS `xbh_schema_migrations`
  DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS `xbh_schema_migrations`.`schema_patch_ledger` (
  `patch_name` VARCHAR(255) NOT NULL,
  `sha256` CHAR(64) NOT NULL,
  `server_uuid` CHAR(36) NOT NULL,
  `applied_at_ms` BIGINT NOT NULL,
  `applied_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`patch_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
SQL
  ledger_exists=1
}

pending=()
destructive_patch_pending=false
destructive_patch_applied=false
destructive_patch_checksum=""
for patch in "${patches[@]}"; do
  name="$(basename "$patch")"
  checksum="$(sha256sum "$patch" | awk '{print $1}')"
  if [[ ! "$name" =~ ^[0-9A-Za-z._-]+$ || ! "$checksum" =~ ^[0-9a-f]{64}$ ]]; then
    echo "invalid production patch identity: $name" >&2
    exit 1
  fi

  ledger_row=""
  if [[ "$ledger_exists" == "1" ]]; then
    ledger_row="$(mysql_query "SELECT CONCAT(sha256, CHAR(9), server_uuid) FROM xbh_schema_migrations.schema_patch_ledger WHERE patch_name='$name';")"
  fi
  if [[ -n "$ledger_row" ]]; then
    IFS=$'\t' read -r stored_checksum stored_uuid <<<"$ledger_row"
    if [[ "$stored_checksum" != "$checksum" ]]; then
      echo "checksum mismatch for already applied patch: $name" >&2
      exit 1
    fi
    if [[ "${stored_uuid,,}" != "${actual_uuid,,}" ]]; then
      echo "migration ledger target mismatch for patch: $name" >&2
      exit 1
    fi
    if [[ "$name" == "$destructive_patch_name" ]]; then
      destructive_patch_applied=true
      destructive_patch_checksum="$checksum"
    fi
    continue
  fi
  pending+=("$patch")
  if [[ "$name" == "$destructive_patch_name" ]]; then
    destructive_patch_pending=true
    destructive_patch_checksum="$checksum"
  fi
done

marker_exists=0
marker_table_exists="$(mysql_query "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='xbh_assistant' AND table_name='runtime_marker';")"
if [[ "$marker_table_exists" == "1" ]]; then
  marker_exists="$(mysql_query "SELECT COUNT(*) FROM xbh_assistant.runtime_marker WHERE name='assistant_runtime_v3';")"
fi
if [[ "$destructive_patch_applied" == true && "$marker_exists" != "1" ]]; then
  echo "assistant v3 patch is ledgered but its runtime marker is missing" >&2
  exit 1
fi

if [[ "$mode" == check ]]; then
  if (( ${#pending[@]} > 0 )); then
    echo "production SQL migration required; pending patches:" >&2
    for patch in "${pending[@]}"; do
      echo "  $(basename "$patch")" >&2
    done
    exit 1
  fi
  echo "production SQL migration check passed (${#patches[@]} patches)"
  exit 0
fi

backup_tmp_files=()
cleanup_backup_temps() {
  local path
  for path in "${backup_tmp_files[@]}"; do
    [[ -n "$path" && -f "$path" ]] && rm -f -- "$path"
  done
  return 0
}
trap cleanup_backup_temps EXIT

backup_dir=""
prepare_backup_dir() {
  local create_missing="${1:-false}"
  local requested="${PRODUCTION_MIGRATION_BACKUP_DIR:-}"
  if [[ "$requested" != /* ]]; then
    echo "PRODUCTION_MIGRATION_BACKUP_DIR must be an absolute path" >&2
    return 1
  fi
  if [[ -L "$requested" ]]; then
    echo "PRODUCTION_MIGRATION_BACKUP_DIR must not be a symbolic link" >&2
    return 1
  fi
  backup_dir="$(realpath -m -- "$requested")"
  if [[ "$backup_dir" == "/" || "$backup_dir" == "$repo" || "$backup_dir" == "$repo/"* ]]; then
    echo "PRODUCTION_MIGRATION_BACKUP_DIR must be a dedicated directory outside the repository" >&2
    return 1
  fi
  if [[ ! -e "$backup_dir" ]]; then
    if [[ "$create_missing" != true ]]; then
      echo "prepared destructive-migration backup directory does not exist" >&2
      return 1
    fi
    mkdir -p -- "$backup_dir"
    chmod 0700 "$backup_dir"
  fi
  if [[ ! -d "$backup_dir" || -L "$backup_dir" ]]; then
    echo "PRODUCTION_MIGRATION_BACKUP_DIR must resolve to a regular directory" >&2
    return 1
  fi
  local mode owner
  mode="$(stat -c '%a' "$backup_dir")"
  owner="$(stat -c '%u' "$backup_dir")"
  if (( (8#$mode & 8#077) != 0 )); then
    echo "PRODUCTION_MIGRATION_BACKUP_DIR permissions must not grant group or other access" >&2
    return 1
  fi
  if [[ "$owner" != "$(id -u)" ]]; then
    echo "PRODUCTION_MIGRATION_BACKUP_DIR must be owned by the current operator" >&2
    return 1
  fi
}

read_manifest_value() {
  local manifest="$1" key="$2"
  awk -F= -v key="$key" '$1 == key { value=substr($0, length(key)+2); seen++ } END { if (seen == 1) printf "%s", value; else exit 1 }' "$manifest"
}

validate_prepared_backup() {
  prepare_backup_dir false
  local ready_file="$backup_dir/assistant-v3-pre-reset.ready"
  if [[ ! -f "$ready_file" || -L "$ready_file" ]]; then
    echo "prepared destructive-migration backup marker is missing" >&2
    return 1
  fi
  local -a ready_lines=()
  mapfile -t ready_lines <"$ready_file"
  if (( ${#ready_lines[@]} != 1 )) || [[ ! "${ready_lines[0]}" =~ ^assistant-v3-pre-reset-[0-9]{8}T[0-9]{6}Z\.manifest$ ]]; then
    echo "prepared destructive-migration backup marker is invalid" >&2
    return 1
  fi

  local manifest="$backup_dir/${ready_lines[0]}"
  if [[ ! -f "$manifest" || -L "$manifest" ]]; then
    echo "prepared destructive-migration manifest is missing" >&2
    return 1
  fi
  local server_uuid patch_name patch_sha assistant_file assistant_sha consent_file consent_sha
  server_uuid="$(read_manifest_value "$manifest" server_uuid)"
  patch_name="$(read_manifest_value "$manifest" patch_name)"
  patch_sha="$(read_manifest_value "$manifest" patch_sha256)"
  assistant_file="$(read_manifest_value "$manifest" assistant_file)"
  assistant_sha="$(read_manifest_value "$manifest" assistant_sha256)"
  consent_file="$(read_manifest_value "$manifest" consent_file)"
  consent_sha="$(read_manifest_value "$manifest" consent_sha256)"

  if [[ "${server_uuid,,}" != "${actual_uuid,,}" || "$patch_name" != "$destructive_patch_name" ||
        "$patch_sha" != "$destructive_patch_checksum" ]]; then
    echo "prepared destructive-migration backup does not match the target and patch" >&2
    return 1
  fi
  if [[ ! "$assistant_file" =~ ^assistant-v3-pre-reset-[0-9]{8}T[0-9]{6}Z\.sql\.gz$ ||
        ! "$consent_file" =~ ^agent-consent-v3-pre-reset-[0-9]{8}T[0-9]{6}Z\.sql\.gz$ ||
        ! "$assistant_sha" =~ ^[0-9a-f]{64}$ || ! "$consent_sha" =~ ^[0-9a-f]{64}$ ]]; then
    echo "prepared destructive-migration backup manifest has invalid file identities" >&2
    return 1
  fi

  local assistant_path="$backup_dir/$assistant_file" consent_path="$backup_dir/$consent_file"
  if [[ ! -f "$assistant_path" || -L "$assistant_path" || ! -f "$consent_path" || -L "$consent_path" ]]; then
    echo "prepared destructive-migration backup files are missing" >&2
    return 1
  fi
  if [[ "$(sha256sum "$assistant_path" | awk '{print $1}')" != "$assistant_sha" ||
        "$(sha256sum "$consent_path" | awk '{print $1}')" != "$consent_sha" ]]; then
    echo "prepared destructive-migration backup checksum verification failed" >&2
    return 1
  fi
  gzip -t "$assistant_path"
  gzip -t "$consent_path"
  gzip -dc "$assistant_path" | awk 'index($0, "xbh_assistant") { found=1 } END { exit !found }'
  gzip -dc "$consent_path" | awk 'index($0, "agent_capability_consent") { found=1 } END { exit !found }'
  destructive_confirmation="$destructive_confirm_prefix:$(sha256sum "$manifest" | awk '{print $1}')"
  verified_backup_manifest="$manifest"
}

create_destructive_backup() {
  prepare_backup_dir true

  local stamp assistant_final consent_final manifest_final ready_file
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  assistant_final="$backup_dir/assistant-v3-pre-reset-$stamp.sql.gz"
  consent_final="$backup_dir/agent-consent-v3-pre-reset-$stamp.sql.gz"
  manifest_final="$backup_dir/assistant-v3-pre-reset-$stamp.manifest"
  ready_file="$backup_dir/assistant-v3-pre-reset.ready"
  if [[ -e "$assistant_final" || -e "$consent_final" || -e "$manifest_final" ]]; then
    echo "refusing to overwrite an existing production migration backup" >&2
    return 1
  fi

  local assistant_tmp consent_tmp manifest_tmp ready_tmp
  assistant_tmp="$(mktemp "$backup_dir/.assistant-v3.XXXXXX.sql.gz")"
  consent_tmp="$(mktemp "$backup_dir/.agent-consent-v3.XXXXXX.sql.gz")"
  manifest_tmp="$(mktemp "$backup_dir/.assistant-v3.XXXXXX.manifest")"
  ready_tmp="$(mktemp "$backup_dir/.assistant-v3-ready.XXXXXX")"
  backup_tmp_files=("$assistant_tmp" "$consent_tmp" "$manifest_tmp" "$ready_tmp")

  "${compose[@]}" exec -T mysql sh -ec \
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysqldump -uroot --single-transaction --quick --hex-blob --routines --events --triggers --set-gtid-purged=OFF --databases xbh_assistant' \
    | gzip -9 >"$assistant_tmp"
  "${compose[@]}" exec -T mysql sh -ec \
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysqldump -uroot --single-transaction --quick --hex-blob --set-gtid-purged=OFF xbh_user agent_capability_consent' \
    | gzip -9 >"$consent_tmp"

  [[ -s "$assistant_tmp" && -s "$consent_tmp" ]]
  gzip -t "$assistant_tmp"
  gzip -t "$consent_tmp"
  gzip -dc "$assistant_tmp" | awk 'index($0, "xbh_assistant") { found=1 } END { exit !found }'
  gzip -dc "$consent_tmp" | awk 'index($0, "agent_capability_consent") { found=1 } END { exit !found }'

  local assistant_sha consent_sha
  assistant_sha="$(sha256sum "$assistant_tmp" | awk '{print $1}')"
  consent_sha="$(sha256sum "$consent_tmp" | awk '{print $1}')"
  {
    printf 'server_uuid=%s\n' "$actual_uuid"
    printf 'created_at_utc=%s\n' "$stamp"
    printf 'patch_name=%s\n' "$destructive_patch_name"
    printf 'patch_sha256=%s\n' "$destructive_patch_checksum"
    printf 'assistant_file=%s\n' "$(basename "$assistant_final")"
    printf 'assistant_sha256=%s\n' "$assistant_sha"
    printf 'consent_file=%s\n' "$(basename "$consent_final")"
    printf 'consent_sha256=%s\n' "$consent_sha"
  } >"$manifest_tmp"

  mv -- "$assistant_tmp" "$assistant_final"
  mv -- "$consent_tmp" "$consent_final"
  mv -- "$manifest_tmp" "$manifest_final"
  printf '%s\n' "$(basename "$manifest_final")" >"$ready_tmp"
  chmod 0600 "$assistant_final" "$consent_final" "$manifest_final" "$ready_tmp"
  mv -f -- "$ready_tmp" "$ready_file"
  backup_tmp_files=()
  echo "verified destructive-migration backup: $manifest_final"
}

if [[ "$destructive_patch_pending" == true && "$marker_exists" != "1" ]]; then
  if [[ "$mode" == prepare-backup ]]; then
    create_destructive_backup
    validate_prepared_backup
    echo "destructive-migration backup verified: $verified_backup_manifest"
    echo "exact confirmation: PRODUCTION_DESTRUCTIVE_MIGRATION_CONFIRM=$destructive_confirmation"
    exit 0
  fi
  validate_prepared_backup
  if [[ "${PRODUCTION_DESTRUCTIVE_MIGRATION_CONFIRM:-}" != "$destructive_confirmation" ]]; then
    echo "assistant v3 reset requires the confirmation printed by --prepare-destructive-backup" >&2
    echo "exact confirmation: PRODUCTION_DESTRUCTIVE_MIGRATION_CONFIRM=$destructive_confirmation" >&2
    exit 1
  fi
elif [[ "$mode" == prepare-backup ]]; then
  echo "no destructive Assistant reset requires a backup"
  exit 0
fi

if [[ "$mode" == apply && "$ledger_exists" != "1" ]]; then
  create_migration_ledger
fi

for patch in "${pending[@]}"; do
  name="$(basename "$patch")"
  checksum="$(sha256sum "$patch" | awk '{print $1}')"
  echo "applying production SQL patch: $name"
  mysql_stdin <"$patch"
  mysql_query "INSERT INTO xbh_schema_migrations.schema_patch_ledger
    (patch_name, sha256, server_uuid, applied_at_ms)
    VALUES ('$name', '$checksum', '$actual_uuid', ROUND(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(3))*1000));" >/dev/null
done

echo "production SQL migration completed (${#pending[@]} newly applied, ${#patches[@]} total)"
