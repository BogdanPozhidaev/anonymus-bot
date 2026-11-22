#!/bin/sh
set -eu

username=${PANEL_BOT_USERNAME:-}
case "$username" in
  *[!A-Za-z0-9_]*) echo 'PANEL_BOT_USERNAME must use letters, digits and underscores' >&2; exit 1 ;;
esac
if [ -n "$username" ] && { [ "${#username}" -lt 5 ] || [ "${#username}" -gt 32 ]; }; then
  echo 'PANEL_BOT_USERNAME must be 5–32 characters' >&2
  exit 1
fi
product=$(printf '%s' "${PANEL_PRODUCT_NAME:-}" | base64 | tr -d '\n')
path=/usr/share/nginx/html/panel-config.json
printf '{"botUsername":"%s","productNameBase64":"%s"}\n' "$username" "$product" > "$path.tmp"
mv "$path.tmp" "$path"
