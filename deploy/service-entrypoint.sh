#!/bin/sh
set -eu

source_config="${SERVICE_CONFIG_SOURCE:-/etc/xbh/source.yaml}"
rendered_config="${SERVICE_CONFIG_RENDERED:-/tmp/xbh-service.yaml}"

if [ ! -r "${source_config}" ]; then
  echo "service config is not readable: ${source_config}" >&2
  exit 1
fi

load_secret_file() {
  variable_name="$1"
  eval "secret_file=\${${variable_name}_FILE:-}"
  if [ -z "${secret_file}" ]; then
    return
  fi
  if [ ! -r "${secret_file}" ]; then
    echo "secret file for ${variable_name} is not readable" >&2
    exit 1
  fi
  secret_value="$(cat "${secret_file}")"
  export "${variable_name}=${secret_value}"
}

for secret_name in \
  JWT_SECRET_KEY RPC_INTERNAL_SECRET REDIS_PASS REDIS_PASSWORD \
  DB_USER DB_CONTENT DB_MEDIA DB_INTERACTION DB_FEED DB_MESSAGE \
  S3_ACCESS_KEY S3_SECRET_KEY FEED_CURSOR_SECRET RECOMMEND_CURSOR_SECRET \
  ES_PASSWORD MILVUS_PASSWORD ASSISTANT_LLM_API_KEY; do
  load_secret_file "${secret_name}"
done

cp "${source_config}" "${rendered_config}"

escape_replacement() {
  printf '%s' "$1" | sed 's/[\\&|]/\\&/g'
}

replace_endpoint() {
  expression="$1"
  replacement="$2"
  if [ -z "${replacement}" ]; then
    return
  fi
  escaped_replacement="$(escape_replacement "${replacement}")"
  sed -i "s|${expression}|${escaped_replacement}|g" "${rendered_config}"
}

# Source configs remain convenient for host-based development. The production
# image rewrites only infrastructure endpoints; secret expansion stays in
# go-zero's conf.UseEnv path.
replace_endpoint 'http://127\.0\.0\.1:8333' "${S3_PUBLIC_BASE_URL:-}"
replace_endpoint '127\.0\.0\.1:8333' "${S3_ENDPOINT:-}"
replace_endpoint '127\.0\.0\.1:6379' "${REDIS_HOST:-}"
replace_endpoint '127\.0\.0\.1:2379' "${ETCD_ENDPOINT:-}"

exec /usr/local/bin/xbh-service -f "${rendered_config}" "$@"
