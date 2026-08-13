#!/usr/bin/env bash
# Run build-tagged integration tests. Critical mode is sized for pull requests.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mode="${1:---all}"
parallelism="${INTEGRATION_PARALLELISM:-1}"

append_no_proxy() {
  local host="$1"
  local name current
  for name in NO_PROXY no_proxy; do
    current="${!name:-}"
    case ",$current," in
      *",$host,"*) ;;
      *) export "$name=${current:+$current,}$host" ;;
    esac
  done
}

# testcontainers resolves published ports through the container's default
# gateway in this topology. Keep local health probes out of HTTP proxies.
if [[ -f /.dockerenv && -S /var/run/docker.sock ]]; then
  docker_gateway="$(docker network inspect bridge \
    --format '{{(index .IPAM.Config 0).Gateway}}' 2>/dev/null || true)"
  if [[ -z "$docker_gateway" ]] && command -v ip >/dev/null; then
    docker_gateway="$(ip -4 route show default 2>/dev/null | awk 'NR == 1 { print $3 }')"
  fi
  if [[ -n "$docker_gateway" ]]; then
    append_no_proxy "$docker_gateway"
  fi
fi

require_port() {
  local name="$1"
  local host="$2"
  local port="$3"
  if ! timeout 2 bash -c "</dev/tcp/$host/$port" 2>/dev/null; then
    echo "$name is required for --all but is not reachable at $host:$port" >&2
    exit 1
  fi
}

if [[ "$mode" == "--critical" ]]; then
  go test -p "$parallelism" -tags=integration -count=1 -timeout=10m \
    ./app/interaction/rpc/internal/logic
  (cd app/user/rpc && go test -p "$parallelism" -tags=integration -count=1 \
    -timeout=10m ./internal/logic ./internal/model)
  exit 0
fi
if [[ "$mode" != "--all" ]]; then
  echo "usage: $0 [--critical|--all]" >&2
  exit 2
fi

# Media's upload integration uses the repository's SeaweedFS S3 configuration.
s3_endpoint="${TEST_S3_ENDPOINT:-127.0.0.1:${INTEGRATION_S3_PORT:-8333}}"
require_port "SeaweedFS S3" "${s3_endpoint%:*}" "${s3_endpoint##*:}"
export TEST_S3_ENDPOINT="$s3_endpoint"

mapfile -t modules < <(
  find . -name go.mod -not -path './.worktrees/*' -not -path './vendor/*' \
    -exec dirname {} \; | sort
)

for module in "${modules[@]}"; do
  echo "==> integration $module/..."
  (cd "$module" && go test -p "$parallelism" -tags=integration -count=1 \
    -timeout=20m ./...)
done
