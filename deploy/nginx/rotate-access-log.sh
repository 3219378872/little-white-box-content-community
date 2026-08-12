#!/bin/sh
# REL-021：安全访问日志最多保留 7 天。nginx:alpine 自带 busybox crond，
# 通过 /etc/crontabs/root 每日轮转 access.log 并保留最近 7 份。
set -eu

log_dir="/var/log/nginx"
access_log="${log_dir}/access.log"

if [ ! -f "${access_log}" ]; then
  exit 0
fi

stamp="$(date +%F)"
rotated="${access_log}.${stamp}"
if [ ! -f "${rotated}" ]; then
  mv "${access_log}" "${rotated}"
fi

# 通知 nginx 重新打开日志文件。
if [ -f /var/run/nginx.pid ]; then
  pid="$(cat /var/run/nginx.pid)"
  kill -USR1 "${pid}" 2>/dev/null || true
fi

# 删除超过 7 天的轮转文件。
find "${log_dir}" -maxdepth 1 -name 'access.log.*' -type f -mtime +7 -delete
