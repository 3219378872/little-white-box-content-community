#!/bin/sh
set -eu

: "${S3_ACCESS_KEY:?S3_ACCESS_KEY is required}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY is required}"

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

access_key="$(json_escape "${S3_ACCESS_KEY}")"
secret_key="$(json_escape "${S3_SECRET_KEY}")"

umask 077
{
  printf '%s\n' '{'
  printf '%s\n' '  "identities": ['
  printf '%s\n' '    {"name":"anonymous","actions":["Read"]},'
  printf '    {"name":"xbh-media","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","Write","List","Tagging"]}\n' "${access_key}" "${secret_key}"
  printf '%s\n' '  ]'
  printf '%s\n' '}'
} > /tmp/s3_config.json

exec /usr/bin/weed "$@"
