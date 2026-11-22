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
color=${PANEL_PRIMARY_COLOR:-blue}
case "$color" in
  blue|cyan|teal|green|grape|violet|indigo) ;;
  *) echo 'PANEL_PRIMARY_COLOR must be blue, cyan, teal, green, grape, violet or indigo' >&2; exit 1 ;;
esac
scheme=${PANEL_COLOR_SCHEME:-light}
case "$scheme" in
  light|dark|auto) ;;
  *) echo 'PANEL_COLOR_SCHEME must be light, dark or auto' >&2; exit 1 ;;
esac

encode() { printf '%s' "$1" | base64 | tr -d '\n'; }
product=$(encode "${PANEL_PRODUCT_NAME:-}")
subtitle_ru=$(encode "${PANEL_LOGIN_SUBTITLE_RU:-}")
subtitle_en=$(encode "${PANEL_LOGIN_SUBTITLE_EN:-}")
path=/tmp/panel-config.json
printf '{"botUsername":"%s","productNameBase64":"%s","primaryColor":"%s","colorScheme":"%s","loginSubtitleRuBase64":"%s","loginSubtitleEnBase64":"%s"}\n' \
  "$username" "$product" "$color" "$scheme" "$subtitle_ru" "$subtitle_en" > "$path.tmp"
mv "$path.tmp" "$path"
exec "$@"
