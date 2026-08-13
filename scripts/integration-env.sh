#!/usr/bin/env bash
# Manage the isolated SeaweedFS service used by media integration tests.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKER_BIN="${INTEGRATION_DOCKER_BIN:-docker}"
ENV_NAME="${INTEGRATION_ENV_NAME:-xbh-test}"
NETWORK_NAME="${ENV_NAME}-network"
SEAWEEDFS_CONTAINER="${ENV_NAME}-seaweedfs"
S3_PORT="${INTEGRATION_S3_PORT:-8333}"
WAIT_SECONDS="${INTEGRATION_WAIT_SECONDS:-120}"
WAIT_INTERVAL="${INTEGRATION_WAIT_INTERVAL:-2}"
ENV_LABEL="com.xiaobaihe.integration-env=$ENV_NAME"

docker_cmd() {
  "$DOCKER_BIN" "$@"
}

require_docker() {
  if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
    echo "Docker command is not available: $DOCKER_BIN" >&2
    return 1
  fi
  if ! docker_cmd info >/dev/null 2>&1; then
    echo "Docker daemon is not available" >&2
    return 1
  fi
}

container_exists() {
  docker_cmd container inspect "$1" >/dev/null 2>&1
}

container_owned() {
  local owner
  owner="$(docker_cmd container inspect \
    --format '{{ index .Config.Labels "com.xiaobaihe.integration-env" }}' \
    "$1" 2>/dev/null)" || return 1
  [[ "$owner" == "$ENV_NAME" ]]
}

network_exists() {
  docker_cmd network inspect "$NETWORK_NAME" >/dev/null 2>&1
}

network_owned() {
  local owner
  owner="$(docker_cmd network inspect \
    --format '{{ index .Labels "com.xiaobaihe.integration-env" }}' \
    "$NETWORK_NAME" 2>/dev/null)" || return 1
  [[ "$owner" == "$ENV_NAME" ]]
}

clear_environment() {
  local status=0 container
  for container in "$SEAWEEDFS_CONTAINER"; do
    if container_exists "$container"; then
      if container_owned "$container"; then
        docker_cmd rm --force "$container" >/dev/null || status=1
      else
        echo "Refusing to remove container not owned by $ENV_LABEL: $container" >&2
        status=1
      fi
    fi
  done
  if network_exists; then
    if network_owned; then
      docker_cmd network rm "$NETWORK_NAME" >/dev/null || status=1
    else
      echo "Refusing to remove network not owned by $ENV_LABEL: $NETWORK_NAME" >&2
      status=1
    fi
  fi
  return "$status"
}

print_logs() {
  local container
  for container in "$SEAWEEDFS_CONTAINER"; do
    if container_exists "$container"; then
      echo "==> $container logs" >&2
      docker_cmd logs "$container" >&2 || true
    fi
  done
}

port_reachable() {
  local host="$1" port="$2"
  timeout 2 bash -c "</dev/tcp/$host/$port" 2>/dev/null
}

wait_for_services() {
  local attempts attempt
  attempts=$(((WAIT_SECONDS + WAIT_INTERVAL - 1) / WAIT_INTERVAL))
  ((attempts > 0)) || attempts=1
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if port_reachable 127.0.0.1 "$S3_PORT"; then
      return 0
    fi
    sleep "$WAIT_INTERVAL"
  done
  echo "Integration services did not become ready within ${WAIT_SECONDS}s" >&2
  return 1
}

init_environment() (
  set -e
  clear_environment
  docker_cmd network create --label "$ENV_LABEL" "$NETWORK_NAME" >/dev/null
  docker_cmd run --detach \
    --name "$SEAWEEDFS_CONTAINER" \
    --label "$ENV_LABEL" \
    --network "$NETWORK_NAME" \
    --publish "127.0.0.1:${S3_PORT}:8333" \
    --volume "$ROOT_DIR/deploy/seaweedfs/s3_config.json:/etc/seaweedfs/s3_config.json:ro" \
    chrislusf/seaweedfs:latest \
    server -master -volume -filer -s3 \
    -s3.config=/etc/seaweedfs/s3_config.json -dir=/data >/dev/null
  wait_for_services
)

case "${1:-}" in
  init)
    require_docker
    set +e
    init_environment
    status=$?
    set -e
    if ((status != 0)); then
      print_logs
      clear_environment || true
      exit "$status"
    fi
    echo "Integration services are ready: S3=127.0.0.1:$S3_PORT"
    ;;
  clear)
    require_docker
    clear_environment
    echo "Integration services cleared: $ENV_NAME"
    ;;
  logs)
    require_docker
    print_logs
    ;;
  *)
    echo "usage: $0 {init|clear|logs}" >&2
    exit 2
    ;;
esac
