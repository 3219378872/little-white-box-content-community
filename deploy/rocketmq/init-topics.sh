#!/usr/bin/env bash
set -uo pipefail
MQADMIN="/home/rocketmq/rocketmq-5.1.3/bin/mqadmin"
NAMESRV="rocketmq-namesrv:9876"
CLUSTER="DefaultCluster"
MAX_WAIT=120
waited=0

echo "[init] waiting for broker to register with namesrv..."

# Wait for broker to be fully ready by polling clusterList
while true; do
  if $MQADMIN clusterList -n "$NAMESRV" 2>/dev/null | grep -q "$CLUSTER"; then
    echo "[init] broker ready (waited ${waited}s)"
    break
  fi
  sleep 3
  waited=$((waited + 3))
  if [ $waited -ge $MAX_WAIT ]; then
    echo "[init] broker not ready after ${MAX_WAIT}s, giving up"
    exit 1
  fi
done

echo "[init] creating topics and consumer groups..."

TOPICS=(
  post-create post-update post-delete
  user-behavior-v2
  message-push
  media-deleted
)

create_topic() {
  local t=$1
  local retries=3
  for i in $(seq 1 $retries); do
    if $MQADMIN updateTopic -n "$NAMESRV" -c "$CLUSTER" -t "$t" -r 4 -w 4 2>/dev/null; then
      echo "[init] topic created: $t"
      return 0
    fi
    sleep 2
  done
  echo "[init] topic FAILED after $retries retries: $t"
  return 1
}

failed=0
for t in "${TOPICS[@]}"; do
  create_topic "$t" || failed=1
done

CONSUMER_GROUPS=(
  content-cleanup-service-group
  content-count-sync-service-group
  embedding-service-group
  search-service-group
  feed-service-group
  message-service-group
  recommend-feature-service-group
  behavior-log-service-group
  media-service-group
)

create_group() {
  local cg=$1
  local retries=3
  for i in $(seq 1 $retries); do
    if $MQADMIN updateSubGroup -n "$NAMESRV" -c "$CLUSTER" -g "$cg" 2>/dev/null; then
      echo "[init] group created: $cg"
      return 0
    fi
    sleep 2
  done
  echo "[init] group FAILED after $retries retries: $cg"
  return 1
}

for cg in "${CONSUMER_GROUPS[@]}"; do
  create_group "$cg" || failed=1
done

if [ "$failed" -ne 0 ]; then
  echo "[init] one or more topics or consumer groups failed"
  exit 1
fi

echo "[init] done."
