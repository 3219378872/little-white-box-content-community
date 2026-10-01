#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
run_id="$$-${RANDOM}"
network_name="xbh-model-pipeline-${run_id}"
clickhouse_name="xbh-model-clickhouse-${run_id}"
s3_name="xbh-model-seaweedfs-${run_id}"
s3_config="$(mktemp)"
runner_image="xbh-model-pipeline-integration:local"

cleanup() {
  docker rm --force "$clickhouse_name" "$s3_name" >/dev/null 2>&1 || true
  rm -f "$s3_config"
  docker network rm "$network_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker network create "$network_name" >/dev/null
docker run --detach --name "$clickhouse_name" --network "$network_name" \
  --network-alias clickhouse \
  --env CLICKHOUSE_DB=xbh_analytics \
  --env CLICKHOUSE_USER=pipeline \
  --env CLICKHOUSE_PASSWORD=pipeline-secret \
  --env CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 \
  --volume "$repo_root/deploy/sql/xbh_analytics.sql:/docker-entrypoint-initdb.d/xbh_analytics.sql:ro" \
  clickhouse/clickhouse-server:23.8-alpine >/dev/null
cat >"$s3_config" <<'JSON'
{"identities":[{"name":"model","credentials":[{"accessKey":"modeladmin","secretKey":"model-secret"}],"actions":["Admin","Read","Write","List"]}]}
JSON
chmod 644 "$s3_config"
docker run --detach --name "$s3_name" --network "$network_name" \
  --network-alias seaweedfs \
  --volume "$s3_config:/etc/seaweedfs/s3_config.json:ro" \
  chrislusf/seaweedfs:4.41 server -s3 -s3.config=/etc/seaweedfs/s3_config.json \
  -volume.max=32 -dir=/data >/dev/null

docker build --tag "$runner_image" --file algorithm/integration/Dockerfile "$repo_root"
docker run --rm --network "$network_name" \
  --env CLICKHOUSE_DSN=http://pipeline:pipeline-secret@clickhouse:8123/xbh_analytics \
  --env MODEL_REGISTRY_BUCKET=xbh-models \
  --env MODEL_REGISTRY_PREFIX=recommend-models \
  --env MODEL_S3_ENDPOINT=http://seaweedfs:8333 \
  --env MODEL_S3_ACCESS_KEY=modeladmin \
  --env MODEL_S3_SECRET_KEY=model-secret \
  --env MODEL_S3_REGION=us-east-1 \
  --env AWS_EC2_METADATA_DISABLED=true \
  "$runner_image"
