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
It does not probe the Telegram API on every health check. After a successful
migration, the database permissions job provisions and verifies the runtime
login before backend starts. Start the bot and scheduled retention separately
when authorized and configured.

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

## PostgreSQL roles and upgrade to schema 11

Use `POSTGRES_USER` and `POSTGRES_PASSWORD` for the existing database owner.
Keep these values when upgrading an existing volume. Set a different,
random `POSTGRES_RUNTIME_PASSWORD` in the protected deployment environment.
For example, `openssl rand -hex 32` generates a compatible value. The
backend and retention connect as `anonymus_runtime`. Their containers receive
only the runtime password. Migrations, database backups and first-admin
bootstrap use the owner login. Use the dedicated PostgreSQL instance from
the Compose project. Role names and passwords are instance-wide, and
`anonymus_runtime` is reserved for this deployment. The provisioning job
requires the privileges of the owner account created by the PostgreSQL image.

Before upgrading, stop the bot, backend, retention and any other writers and
take a verified backup. Connect to the target database as the owner and check
for historical sessions with the same participant in both roles.

```sql
SELECT id FROM sessions WHERE client_user_id = executor_user_id ORDER BY id;
```

If any rows are returned, have the owner review and correct those sessions
according to their history before applying the migration. Migration 11
rejects conflicts and rolls back its changes. The migration tool can leave a
dirty version marker after failure. Verify the actual schema and recover the
marker to the last confirmed version before retrying. A participant can still
appear in different sessions, and either slot can remain empty.

Use the same Compose project and overrides for the upgrade. Apply migrations
and check the clean schema version before starting the stack. Normal startup
waits for `db-permissions` to complete.

```sh
docker compose run --rm -T --no-deps migrate migrate up
docker compose run --rm -T --no-deps migrate migrate status
docker compose up -d --build --wait nginx
```

The permissions job can run repeatedly and uses the new password each time.
For a password change, stop backend, bot and retention, update the protected
environment, run `docker compose run --rm -T --no-deps db-permissions`, then
recreate backend and retention with the new environment before resuming bot
traffic. The grants are an explicit allowlist in `infra/db-permissions.sql`.
When adding an application table or sequence, update that allowlist and its
tests if the runtime needs access.

Participant, sender and creator references protect history. Deleting an
operator who is only assigned as a session owner clears the assignment.
Deleting an otherwise deletable session clears active pointers and removes
its invites. It remains blocked while messages reference the session.
Deleting a message removes its outbox record. The existing composite
`audit_log(action, timestamp DESC, id DESC)` index covers action filtering.

## Container permissions

The backend, migration command, bot, retention and panel use numeric
UID/GID `10001:10001`. Application root filesystems are read-only.
The media initializer briefly runs as root in a container without
network access. It assigns both media volume roots and their top-level
regular files to the application UID. The backend mounts those volumes read-only.
PostgreSQL drops privileges through its image entrypoint. The edge Nginx
master retains the capabilities needed to read restricted TLS key files,
bind its ports and run non-root workers.

On Linux, grant UID/GID 10001 read access to both internal token files
before recreating backend and bot. For instance, if you manage file
ownership directly, set their group to numeric GID 10001 with `chgrp` and
use mode `0440`. Keep them outside the repository. Compose file-backed
secrets are bind mounts, so the container must be able to read the
underlying files. On Docker Desktop, retain restrictive host ACLs and
verify that the container can read the mounts.

When upgrading an existing deployment, quiesce backend, bot and retention
and take the backup described in [Backup and restore](../deploy/backup/README.md).
Run the media initializer for the same Compose project and environment
before restarting the services. Normal startup also waits for it.

```bash
docker compose -f infra/docker-compose.yml run --rm --no-deps media-init
```

Redis persists its authenticated state in the Compose `redis_data` volume
using an AOF synchronized approximately once per second. Its contents
include authentication and moderation state. Restrict access to the volume.
The first upgrade creates a new Redis volume. Operators need to sign in
again, and the previous moderation counters are cleared.
The PostgreSQL and media backup script excludes Redis. See
[Operations](OPERATIONS.md) for the resource limits.

Run `bash infra/check-hardening.sh` to build and check the stack using a
disposable Compose project with synthetic credentials and certificates.
It uses loopback ports 18180 and 18443 by default.

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
links, and optionally configure the panel name, accent, scheme and localized
sign-in text. Recreate the panel container to apply its runtime settings.
For Telegram greetings, set the optional bot branding values and recreate
the bot container. Both sets of options are applied at container startup.
See [White-label settings](ENV_VARS.md#white-label-settings).
The base Compose image runs Gin in release mode. The dev override uses
debug and publishes application dependency ports only on loopback.
