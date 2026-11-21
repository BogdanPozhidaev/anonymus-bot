# Compose deployment

The edge Nginx is the public entry point. The built panel is served at
`/`, including direct links such as `/sessions/42`. API calls use the
same origin and retain their `/api` prefix. Neither panel, backend,
bot, PostgreSQL nor Redis has a host port in the base configuration.

## Start an isolated HTTP stand

Copy `infra/.env.example` to a separate environment file outside the
repository and configure it as described in [ENV_VARS.md](ENV_VARS.md).
Use fixture credentials and a unique Compose project name. For an HTTP
test on the same machine set `PANEL_HTTP_BIND=127.0.0.1` and choose a
free `PANEL_HTTP_PORT`. If another Compose project already uses the
default proxy subnet, choose an unused subnet and set all three
`AUTH_PROXY_SUBNET`, `AUTH_PROXY_IP_RANGE` and `AUTH_PROXY_IP` values.
From the repository root:

```powershell
$compose = @('-p', 'anonymus-acceptance', '--env-file', 'C:/private/acceptance.env', '-f', 'infra/docker-compose.yml')
docker compose @compose config --quiet
docker compose @compose up -d --build --wait nginx
docker compose @compose exec nginx nginx -t
docker compose @compose run --rm -T --no-deps migrate migrate status
curl.exe --fail http://127.0.0.1:18080/api/health
```

The edge waits for both backend and panel. Backend readiness requires
successful PostgreSQL and Redis requests. The panel checks its built
index and the edge checks both the panel and API over their public paths.
The bot has an internal HTTP readiness endpoint after successful startup.
It does not probe the Telegram API on every health check. The migration
service exits after a successful upgrade. Start the bot and scheduled
retention separately when authorized and configured.

The URL above assumes `PANEL_HTTP_PORT=18080`. Look for HTTP 200 and
`"status":"ok"` in the health response, and `dirty=false` in migration
status. Use the same Compose arguments for first-admin bootstrap:

```powershell
$OutputEncoding = [Text.UTF8Encoding]::new($false)
Get-Content -Raw -Encoding UTF8 'C:/private/fixture-admin-password.txt' |
  docker compose @compose run --rm -T --no-deps migrate create-admin --login fixture-admin --name 'Fixture Admin'
```

Sign in at the edge and complete TOTP. Sign out and sign in again with the
same authenticator. Create a synthetic session, verify that it appears
in the session list and open its detail page. For independent handover,
record the tested commit, clean schema version and observed results
without including passwords, TOTP secrets or invitations. This fixture
can be stopped with `docker compose @compose down`.

## TLS with owner-provided certificates

Use Docker Compose 2.24.4 or later for the TLS override. Obtain a
certificate for your DNS name from your certificate authority. Put the
full certificate chain and private key in a restricted directory outside
the repository, using the names `fullchain.pem` and `privkey.pem`. Set
`TLS_CERT_DIR` to that absolute directory and set `PANEL_BASE_URL` to the
public HTTPS origin, including the port if it differs from 443. Docker
and the Nginx master process need read access to the mounted files.

```powershell
docker compose -f infra/docker-compose.yml -f infra/docker-compose.tls.yml config --quiet
docker compose -f infra/docker-compose.yml -f infra/docker-compose.tls.yml up -d --build --wait nginx
docker compose -f infra/docker-compose.yml -f infra/docker-compose.tls.yml exec nginx nginx -t
curl.exe --fail https://panel.example.com/api/health
```

Replace the example host with the name on the certificate. The override
publishes HTTPS only, defaults to port 443, enables TLS 1.2 and 1.3 and
sets HSTS for one day. Port 80 remains available to health checks inside
the edge container. After renewing the files validate Nginx and reload it
with `nginx -s reload` in the edge container.

## Limits and configuration

Nginx accepts bodies up to 24 MiB. Telegram downloads bypass the edge
and retain the bot's 20 MiB download limit. Backend API request bodies
are limited to 64 KiB because they contain JSON and media references.
The edge sends a content
security policy that permits the panel's data-URL QR codes and blob
previews, plus frame, MIME-sniffing and referrer protection. Configure
`PANEL_BASE_URL` with a root origin. That setting changes links produced
by the bot at container startup. The panel uses a same-origin relative
API prefix, so changing domains does not require rebuilding it.
Set `PANEL_BOT_USERNAME` without the leading `@` to enable invitation
links, and optionally set `PANEL_PRODUCT_NAME` for branding. Recreate
the panel container to apply these settings. Its startup script generates
the same-origin runtime configuration.
The base Compose image runs Gin in release mode. The dev override uses
debug and publishes application dependency ports only on loopback.
