#!/bin/sh
set -eu

: "${S3_ACCESS_KEY:?S3_ACCESS_KEY is required}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY is required}"
: "${MILVUS_S3_ACCESS_KEY:?MILVUS_S3_ACCESS_KEY is required}"
: "${MILVUS_S3_SECRET_KEY:?MILVUS_S3_SECRET_KEY is required}"
: "${MODEL_S3_ACCESS_KEY:?MODEL_S3_ACCESS_KEY is required}"
: "${MODEL_S3_SECRET_KEY:?MODEL_S3_SECRET_KEY is required}"

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

access_key="$(json_escape "${S3_ACCESS_KEY}")"
secret_key="$(json_escape "${S3_SECRET_KEY}")"
milvus_access_key="$(json_escape "${MILVUS_S3_ACCESS_KEY}")"
milvus_secret_key="$(json_escape "${MILVUS_S3_SECRET_KEY}")"
model_access_key="$(json_escape "${MODEL_S3_ACCESS_KEY}")"
model_secret_key="$(json_escape "${MODEL_S3_SECRET_KEY}")"

umask 077
{
  printf '%s\n' '{'
  printf '%s\n' '  "identities": ['
  printf '%s\n' '    {"name":"anonymous","actions":["Read:xbh-media"]},'
  printf '    {"name":"xbh-media","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","Write","List","Tagging"]},\n' "${access_key}" "${secret_key}"
  # Milvus creates its own bucket on first start, so it needs Admin on that bucket only.
  printf '    {"name":"xbh-milvus","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin:xbh-milvus","Read:xbh-milvus","Write:xbh-milvus","List:xbh-milvus"]},\n' "${milvus_access_key}" "${milvus_secret_key}"
  printf '    {"name":"xbh-models","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Read:xbh-models","Write:xbh-models","List:xbh-models"]}\n' "${model_access_key}" "${model_secret_key}"
  printf '%s\n' '  ]'
  printf '%s\n' '}'
} > /tmp/s3_config.json

exec /usr/bin/weed "$@"
