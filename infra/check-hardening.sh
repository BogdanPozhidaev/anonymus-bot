#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
project="anonymus-hardening-$$-${RANDOM}"
fixture=$(mktemp -d)
host_fixture=$fixture
if command -v cygpath >/dev/null; then
  host_fixture=$(cygpath -m "$fixture")
  export MSYS_NO_PATHCONV=1
fi

compose=(docker compose -p "$project" --env-file "$host_fixture/fixture.env" -f docker-compose.yml)
monitoring=("${compose[@]}" -f docker-compose.monitoring.yml)
cleanup_compose=("${compose[@]}")
extra_logs=()
webhook_fixture_started=0
http_status() {
  curl --noproxy '*' -sS -w '\n%{http_code}\n' "$@" | tail -n 1
}
if [[ ${CHECK_MONITORING:-0} == 1 ]]; then
  cleanup_compose=("${monitoring[@]}")
  extra_logs=(prometheus grafana)
fi
cleanup() {
  local result=$?
  if (( result != 0 )) && [[ -f $fixture/fixture.env ]]; then
    "${cleanup_compose[@]}" logs --no-color --tail 12 backend panel nginx redis postgres media-init migrate db-permissions "${extra_logs[@]}" >&2 || true
  fi
  if [[ $webhook_fixture_started == 1 ]]; then
    docker stop "$project-webhook-fixture" >/dev/null 2>&1 || true
  fi
  "${cleanup_compose[@]}" down --volumes
  rm -f -- "$fixture/fixture.env" "$fixture/backend-token" "$fixture/bot-token" \
    "$fixture/metrics-token" "$fixture/grafana-password" "$fixture/certs/privkey.pem" "$fixture/certs/fullchain.pem"
  rm -f -- "$fixture/webhook-token" "$fixture/webhook-fixture.conf"
  if [[ -d $fixture/certs ]]; then rmdir -- "$fixture/certs"; fi
  rmdir -- "$fixture"
}
trap cleanup EXIT

printf '%064d' 0 > "$fixture/backend-token"
printf '%064d' 1 > "$fixture/bot-token"
printf '%064d' 2 > "$fixture/metrics-token"
printf '%064d' 3 > "$fixture/webhook-token"
printf '%s' 'isolated-grafana-fixture-password' > "$fixture/grafana-password"
chmod 0444 "$fixture/backend-token" "$fixture/bot-token" "$fixture/metrics-token" "$fixture/grafana-password" "$fixture/webhook-token"
mkdir "$fixture/certs"
openssl req -x509 -nodes -newkey rsa:2048 -days 1 -subj /CN=localhost \
  -addext subjectAltName=DNS:localhost \
  -keyout "$host_fixture/certs/privkey.pem" -out "$host_fixture/certs/fullchain.pem" 2>/dev/null
chmod 0600 "$fixture/certs/privkey.pem"
cat > "$fixture/fixture.env" <<EOF
POSTGRES_USER=fixture
POSTGRES_DB=fixture
POSTGRES_PASSWORD=isolated-fixture-password
POSTGRES_RUNTIME_PASSWORD=isolated-fixture-runtime-password-0000
REDIS_PASSWORD=isolated-fixture-redis-password-0000
PII_ENCRYPTION_KEY=$(printf '%032d' 0 | base64 | tr -d '\n')
PII_ENCRYPTION_KEYS=2:$(printf '%032d' 1 | base64 | tr -d '\n')
PII_ENCRYPTION_ACTIVE_VERSION=1
INTERNAL_API_TOKEN_FILE=$host_fixture/backend-token
BOT_INTERNAL_TOKEN_FILE=$host_fixture/bot-token
METRICS_TOKEN_FILE=$host_fixture/metrics-token
GRAFANA_ADMIN_PASSWORD_FILE=$host_fixture/grafana-password
GRAFANA_PORT=${HARDENING_GRAFANA_PORT:-13000}
TELEGRAM_WEBHOOK_URL=https://fixture.example.invalid/telegram/webhook
TELEGRAM_WEBHOOK_SECRET_FILE=$host_fixture/webhook-token
PANEL_BOT_USERNAME=fixture_bot
PANEL_PRODUCT_NAME=Fixture Panel
PANEL_PRIMARY_COLOR=teal
PANEL_COLOR_SCHEME=dark
PANEL_LOGIN_SUBTITLE_RU='Вход в тестовую панель'
PANEL_LOGIN_SUBTITLE_EN='Sign in to "Fixture & Co"'
BOT_ORGANIZATION_NAME='Fixture & Co'
BOT_JOINED_TEXT_RU=
BOT_JOINED_TEXT_EN='Welcome to the fixture session'
BOT_REQUEST_RECEIVED_TEXT_RU=
BOT_REQUEST_RECEIVED_TEXT_EN=
TELEGRAM_BOT_TOKEN=
OPERATOR_GROUP_CHAT_ID=
PANEL_HTTP_BIND=127.0.0.1
PANEL_HTTP_PORT=${HARDENING_HTTP_PORT:-18180}
PANEL_HTTPS_PORT=${HARDENING_HTTPS_PORT:-18443}
PANEL_HTTPS_BIND=127.0.0.1
AUTH_PROXY_SUBNET=${HARDENING_PROXY_SUBNET:-172.30.242.0/24}
AUTH_PROXY_IP_RANGE=${HARDENING_PROXY_RANGE:-172.30.242.128/25}
AUTH_PROXY_IP=${HARDENING_PROXY_IP:-172.30.242.2}
TLS_CERT_DIR=$host_fixture/certs
EOF
while IFS='=' read -r name _; do
  unset "$name"
done < "$fixture/fixture.env"

"${compose[@]}" config --quiet
"${compose[@]}" -f docker-compose.dev.yml config --quiet
"${compose[@]}" -f docker-compose.tls.yml config --quiet
"${compose[@]}" build backend bot retention panel

"${compose[@]}" run --rm --no-deps media-init
# Seed the layout of an existing root-owned media volume, including a private
# voice file. A normal start must prepare these files for the application UID.
"${compose[@]}" run --rm --no-deps --entrypoint /bin/sh media-init -ec '
  chown 0:0 /data/photos /data/voice
  chmod 0750 /data/photos /data/voice
  umask 077
  printf photo > /data/photos/fixture.jpg
  printf voice > /data/voice/fixture.ogg
'
"${compose[@]}" up -d --wait --wait-timeout 90 nginx
"${compose[@]}" run --rm -T --no-deps db-permissions
"${compose[@]}" run --rm -T --no-deps --entrypoint /bin/sh db-permissions -ec '
  export PGPASSWORD="$POSTGRES_RUNTIME_PASSWORD"
  exec psql -X --no-password -q -v ON_ERROR_STOP=1 -h postgres -U anonymus_runtime -d "$POSTGRES_DB"
' < check-db-permissions.sql
curl --noproxy '*' -fsS "http://127.0.0.1:${HARDENING_HTTP_PORT:-18180}/api/health" | grep -q '"status":"ok"'
panel_config=$(curl --noproxy '*' -fsS "http://127.0.0.1:${HARDENING_HTTP_PORT:-18180}/panel-config.json")
grep -q '"botUsername":"fixture_bot"' <<< "$panel_config"
grep -q '"primaryColor":"teal","colorScheme":"dark"' <<< "$panel_config"
encoded_subtitle=$(printf '%s' 'Sign in to "Fixture & Co"' | base64 | tr -d '\n')
grep -Fq "\"loginSubtitleEnBase64\":\"$encoded_subtitle\"" <<< "$panel_config"
if "${compose[@]}" run --rm -T --no-deps -e PANEL_PRIMARY_COLOR=invalid panel; then
  echo 'Panel accepted an invalid accent color' >&2
  exit 1
fi
asset=$(curl --noproxy '*' -fsS "http://127.0.0.1:${HARDENING_HTTP_PORT:-18180}/dashboard" |
  grep -oE '/assets/[^" ]+\.js')
curl --noproxy '*' -fsSI "http://127.0.0.1:${HARDENING_HTTP_PORT:-18180}$asset" |
  grep -qiE '^content-type: (application|text)/javascript'
"${compose[@]}" exec -T backend sh -ec '
  test "$(id -u)" = 10001
  test "$POSTGRES_USER" = anonymus_runtime
  test "$(cat /data/photos/fixture.jpg)" = photo
  test "$(cat /data/voice/fixture.ogg)" = voice
  test -r /run/secrets/internal_api_token
  test -r /run/secrets/bot_internal_token
  grep -q " /data/photos .* ro," /proc/mounts
  grep -q " /data/voice .* ro," /proc/mounts
'
"${compose[@]}" exec -T panel sh -ec 'test "$(id -u)" = 10001; nginx -t'
"${compose[@]}" exec -T nginx sh -ec 'ps -o user,args | grep -Eq "^nginx[[:space:]]+nginx: worker process"'
"${compose[@]}" exec -T postgres sh -ec '
  test "$(stat -c %u /proc/1)" = "$(id -u postgres)"
'
"${compose[@]}" run --rm --no-deps --entrypoint /bin/sh bot -ec '
  test "$(id -u)" = 10001
  test "$BOT_ORGANIZATION_NAME" = "Fixture & Co"
  test "$BOT_JOINED_TEXT_EN" = "Welcome to the fixture session"
  test -r /run/secrets/internal_api_token
  test -r /run/secrets/bot_internal_token
  convert -size 2x2 xc:white -strip /data/photos/hardening-test.jpg
  printf published > /data/voice/hardening-test.ogg
'
"${compose[@]}" run --rm --no-deps --entrypoint /bin/sh retention -ec '
  test "$(id -u)" = 10001
  test "$POSTGRES_USER" = anonymus_runtime
  rm /data/photos/hardening-test.jpg /data/voice/hardening-test.ogg
'
"${compose[@]}" run --rm --no-deps --entrypoint /retention retention -dry-run
"${compose[@]}" exec -T postgres sh -ec \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' <<'SQL'
INSERT INTO operators (name, login, role, status, totp_secret)
VALUES ('Rotation fixture', 'rotation-fixture', 'admin', 'active', 'JBSWY3DPEHPK3PXP');
SQL
"${compose[@]}" run --rm -T --no-deps --entrypoint /admin \
  -e PII_ENCRYPTION_ACTIVE_VERSION=2 backend rotate-keys |
  grep -qx 'applied=false users=0 operators=1'
"${compose[@]}" run --rm -T --no-deps --entrypoint /admin \
  -e PII_ENCRYPTION_ACTIVE_VERSION=2 backend rotate-keys --apply |
  grep -qx 'applied=true users=0 operators=1'
"${compose[@]}" run --rm -T --no-deps --entrypoint /admin \
  -e PII_ENCRYPTION_ACTIVE_VERSION=2 -e PII_ENCRYPTION_KEY= backend rotate-keys |
  grep -qx 'applied=false users=0 operators=0'
"${compose[@]}" exec -T redis sh -ec '
  redis-cli ping | grep -q NOAUTH
  export REDISCLI_AUTH="$REDIS_PASSWORD"
  redis-cli SET hardening:fixture persisted EX 600 | grep -qx OK
'
"${compose[@]}" up -d --force-recreate --wait --wait-timeout 60 redis
"${compose[@]}" exec -T redis sh -ec '
  export REDISCLI_AUTH="$REDIS_PASSWORD"
  redis-cli GET hardening:fixture | grep -qx persisted
'
for service in backend panel postgres redis nginx; do
  id=$("${compose[@]}" ps -q "$service")
  docker inspect --format '{{.HostConfig.ReadonlyRootfs}} {{.HostConfig.Memory}} {{.HostConfig.NanoCpus}} {{.HostConfig.PidsLimit}}' "$id" |
    grep -Eq '^true [1-9][0-9]* [1-9][0-9]* [1-9][0-9]*$'
done

if [[ ${CHECK_MONITORING:-0} == 1 ]]; then
  "${monitoring[@]}" config --quiet
  "${monitoring[@]}" up -d --wait --wait-timeout 90 grafana
  "${monitoring[@]}" exec -T backend sh -ec '
    endpoint=http://127.0.0.1:8080/metrics
    headers=$(wget -S -O /dev/null "$endpoint" 2>&1 || true)
    printf "%s\n" "$headers" | grep -q "401 Unauthorized"
    internal_token=$(cat /run/secrets/internal_api_token)
    headers=$(wget -S -O /dev/null --header="Authorization: Bearer $internal_token" "$endpoint" 2>&1 || true)
    printf "%s\n" "$headers" | grep -q "401 Unauthorized"
  '
  for path in /metrics /metrics/ /api/metrics; do
    status=$(http_status "http://127.0.0.1:${HARDENING_HTTP_PORT:-18180}$path")
    test "$status" = 404
  done
  "${monitoring[@]}" exec -T postgres sh -ec \
    'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' <<'SQL'
WITH fixture_session AS (INSERT INTO sessions (title) VALUES ('metrics fixture') RETURNING id),
fixture_message AS (
    INSERT INTO messages (session_id, sender_role, content_type)
    SELECT id, 'client', 'text' FROM fixture_session RETURNING id
)
INSERT INTO message_outbox (message_id) SELECT id FROM fixture_message;
SQL
  prometheus_id=$("${monitoring[@]}" ps -q prometheus)
  grafana_id=$("${monitoring[@]}" ps -q grafana)
  docker inspect --format '{{len .HostConfig.PortBindings}}' "$prometheus_id" | grep -qx 0
  docker inspect --format '{{(index (index .HostConfig.PortBindings "3000/tcp") 0).HostIp}}' "$grafana_id" | grep -qx 127.0.0.1
  grafana="http://127.0.0.1:${HARDENING_GRAFANA_PORT:-13000}"
  dashboard_path="$grafana/api/dashboards/uid/anonymus-overview"
  test "$(http_status "$dashboard_path")" = 401
  grafana_auth="admin:$(cat "$fixture/grafana-password")"
  dashboard=$(curl --noproxy '*' -fsS --user "$grafana_auth" "$dashboard_path")
  grep -Fq '"title":"Anonymus Bot overview"' <<< "$dashboard"
  grep -Fq '"title":"HTTP p95 latency"' <<< "$dashboard"
  query_url="$grafana/api/datasources/proxy/uid/anonymus-prometheus/api/v1/query"
  result=''
  for _ in {1..35}; do
    result=$(curl --noproxy '*' -fsS --user "$grafana_auth" --get \
      --data-urlencode 'query=anonymus_outbox_pending{status="queued"} and on(job,instance) (up{job="anonymus-backend"} == 1)' \
      "$query_url")
    if grep -Eq '"value":\[[^,]+,"1"\]' <<< "$result"; then break; fi
    sleep 1
  done
  grep -Eq '"value":\[[^,]+,"1"\]' <<< "$result"
  echo 'Monitoring scrape and provisioned Grafana dashboard passed'
fi

"${cleanup_compose[@]}" -f docker-compose.tls.yml config --quiet
"${cleanup_compose[@]}" -f docker-compose.tls.yml up -d --wait --wait-timeout 60 nginx
curl --noproxy '*' -fsS --cacert "$host_fixture/certs/fullchain.pem" --resolve "localhost:${HARDENING_HTTPS_PORT:-18443}:127.0.0.1" \
  "https://localhost:${HARDENING_HTTPS_PORT:-18443}/api/health" | grep -q '"status":"ok"'

tls_args=(--cacert "$host_fixture/certs/fullchain.pem" --resolve "localhost:${HARDENING_HTTPS_PORT:-18443}:127.0.0.1")
webhook_url="https://localhost:${HARDENING_HTTPS_PORT:-18443}/telegram/webhook"
test "$(http_status "${tls_args[@]}" -X POST "$webhook_url")" = 404
cleanup_compose+=(-f docker-compose.tls.yml -f docker-compose.webhook.yml)
"${cleanup_compose[@]}" config --quiet
"${cleanup_compose[@]}" up -d --wait --wait-timeout 60 nginx
"${cleanup_compose[@]}" run --rm -T --no-deps --entrypoint /bin/sh bot -ec '
  test "$BOT_UPDATE_MODE" = webhook
  test -r "$TELEGRAM_WEBHOOK_SECRET_FILE"
'

webhook_secret=$(cat "$fixture/webhook-token")
cat > "$fixture/webhook-fixture.conf" <<EOF
pid /tmp/nginx.pid;
events {}
http {
    access_log off;
    client_body_temp_path /tmp/client_body;
    proxy_temp_path /tmp/proxy;
    fastcgi_temp_path /tmp/fastcgi;
    uwsgi_temp_path /tmp/uwsgi;
    scgi_temp_path /tmp/scgi;
    server {
        listen 9091;
        location = /telegram/webhook {
            if (\$http_x_telegram_bot_api_secret_token = "$webhook_secret") { return 204; }
            return 401;
        }
        location / { return 418; }
    }
}
EOF
docker run --rm -d --name "$project-webhook-fixture" --network "${project}_webhook" --network-alias bot \
  --entrypoint nginx --user 101:101 --read-only --cap-drop ALL --tmpfs /tmp:size=8m,mode=1777 \
  --mount "type=bind,src=$host_fixture/webhook-fixture.conf,dst=/etc/nginx/webhook-fixture.conf,readonly" \
  nginx:1.25-alpine -c /etc/nginx/webhook-fixture.conf -g 'daemon off;' >/dev/null
webhook_fixture_started=1
status=''
for _ in {1..15}; do
  status=$(http_status "${tls_args[@]}" -X POST -H "X-Telegram-Bot-Api-Secret-Token: $webhook_secret" "$webhook_url")
  if [[ $status == 204 ]]; then break; fi
  sleep 1
done
test "$status" = 204
test "$(http_status "${tls_args[@]}" -X POST "$webhook_url")" = 401
test "$(http_status "${tls_args[@]}" "$webhook_url")" = 405
test "$(http_status "${tls_args[@]}" -X POST "$webhook_url/extra")" = 404
test "$(http_status "${tls_args[@]}" -X POST "${webhook_url%/telegram/webhook}/internal/send-alert")" = 404
echo 'Telegram webhook TLS edge check passed with a fake upstream'
echo "Container hardening smoke test passed"
