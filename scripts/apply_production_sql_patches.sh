#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose=(docker compose)
if [[ -n "${PRODUCTION_ENV_FILE:-}" ]]; then
  compose+=(--env-file "$PRODUCTION_ENV_FILE")
fi
compose+=(
  -f "$repo/deploy/docker-compose.middleware.yml"
  -f "$repo/deploy/docker-compose.production.yml"
  --profile production
)

"${compose[@]}" up -d --wait mysql

shopt -s nullglob
patches=("$repo"/deploy/sql/patches/*.sql)
shopt -u nullglob
for patch in "${patches[@]}"; do
  echo "applying production SQL patch: $(basename "$patch")"
  "${compose[@]}" exec -T mysql sh -ec \
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --default-character-set=utf8mb4' \
    <"$patch"
done
