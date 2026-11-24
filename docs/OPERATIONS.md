# Operations

Use the correct Compose project name for every operation. The
[deployment guide](DEPLOYMENT.md) covers initialization, migration
status, authenticated health checks and external TLS.

- Monitor `/api/health` through Nginx. HTTP 503 indicates a failed
  PostgreSQL or Redis readiness check. Review the relevant service
  status and logs with access restricted to operators.
- Check the panel history for queued, retrying or failed deliveries.
  A failed Telegram response can be ambiguous. Replaying messages
  manually can generate duplicates.
- Rotate the two internal API tokens using separate files outside the
  repository. Coordinate bot and backend restarts. Check application
  health and authorization after both services reload the files.
- Follow [retention](RETENTION.md) for dry-run and execution. Keep the
  writers quiesced when following the
  [database and media snapshot procedure](../deploy/backup/README.md).
- For operational handover, pass configuration locations and access
  ownership securely. Use synthetic accounts for smoke tests.

## Single-organization pilot

Create a fresh Compose project with a protected fixture env file and
loopback panel binding. Follow [isolated deployment](DEPLOYMENT.md#start-an-isolated-http-stand).
Verify HTTP 200 for the panel and `/api/health`, a clean migration
version and healthy Compose services. Create the first administrator,
enroll TOTP, invite an operator, complete their required password change
and verify a fresh sign-in.

Use a separate test bot and authorized synthetic participants for a
Telegram acceptance run. Create a request, claim it, open its linked
session, transfer it and confirm that only its new owner and administrators
can open it. Redeem both invitations, then verify a reply to the
explicitly selected recipient. A local fake bot verifies delivery from the
worker through the internal endpoint. Acceptance by Telegram and reading
by a participant require their own observations.

For diagnosis, use the same Compose project, env file and overrides as
the running deployment.

```powershell
docker compose @compose ps
docker compose @compose run --rm -T --no-deps migrate migrate status
docker compose @compose logs --tail 100 backend
```

Do not share raw service logs. Check the session history first for queued,
retrying and failed messages. With monitoring enabled, inspect backend
scrape health, outbox query health and queued and retrying counts in
[Monitoring](#monitoring). A paused session or missing participant can
hold valid work in the queue. Outbox failure after an ownership change
requires review and a new intentional reply by the current operator.

The delivery worker runs in the backend process. To stop delivery during
a maintenance window, stop incoming traffic and then stop the bot and
backend. This also takes the operator API offline. Stopping the bot alone
leaves the worker running and may consume retries. Stopping the backend
follows its graceful shutdown procedure below. Keep PostgreSQL and media
volumes for [backup and isolated recovery](../deploy/backup/README.md).

Before a release, record the tested commit, image versions, clean schema
version, ownership results and a verified backup. Apply compatible schema
changes before starting the new backend and panel. Keep the new data
when reverting application images. Read the
[schema 13 worker rollback constraints](MIGRATIONS.MD) before restoring
an older backend. Run migrations and database grant provisioning on the
recovered fixture, then test access and one representative media file
before restoring traffic.

## Backend shutdown

On SIGTERM or interrupt, the backend stops accepting HTTP connections
and stops claiming delivery jobs. It gives active HTTP requests up to
100 seconds to drain. An already claimed job keeps its own deadlines of
45 seconds for submission to the bot and 10 seconds for recording the
result in PostgreSQL. A successful stop logs `backend stopped`.

Compose gives the backend two minutes before forcing it to exit. If
the process exits before an outbox update is committed, the job becomes
eligible again after its lease expires. A bot response lost after
Telegram accepts a message can still result in a duplicate on retry.

## Container resource limits

The base Compose file sets CPU, memory and PID limits for each service.
These defaults are for an MVP installation. Adjust them in a Compose
override for measured workloads. PostgreSQL is limited to 1 GiB of
memory, Redis and backend to 512 MiB each, and the bot to 768 MiB.
The panel, edge Nginx and retention have lower limits.
Watch Redis memory use and AOF growth, including disk space for an
AOF rewrite. Keep the Redis volume across container replacements.

## Encryption key rotation

The backend writes AES-GCM ciphertext with the active version and reads using
the version in the authenticated header. Older unversioned participant PII and
encrypted operator TOTP secrets use version 1. The rotation command covers
`users.username`, `users.first_name`, and `operators.totp_secret`, including
older plaintext TOTP secrets. It preserves operator authentication versions and
does not require 2FA re-enrollment.

1. Use the release containing versioned encryption for both backend and retention.
   On the first upgrade, keep the current key in `PII_ENCRYPTION_KEY` and leave
   version 1 active. Schedule a maintenance window for key rotation. Stop bot,
   backend and retention in the target Compose project, and pause any other
   writers. Keep PostgreSQL running. Take a verified backup and make sure recovery
   has access to the key versions used by that backup.
2. Generate 32 random bytes with a trusted generator such as
   `openssl rand -base64 32`. Add the secret to the deployment environment as
   the next unused `version:base64` entry in `PII_ENCRYPTION_KEYS`. Keep the keys
   currently required to read the database, and set
   `PII_ENCRYPTION_ACTIVE_VERSION` to the new version. Protect `infra/.env` or
   the secret store and avoid printing resolved Compose configuration. Use
   `docker compose config --quiet` to validate Compose syntax.
3. In `infra/`, use the same Compose project name, environment file and
   overrides as the existing deployment. Run the check first and review the
   returned user/operator row counts.

   ```sh
   docker compose run --rm -T --no-deps --entrypoint /admin backend rotate-keys
   docker compose run --rm -T --no-deps --entrypoint /admin backend rotate-keys --apply
   docker compose run --rm -T --no-deps --entrypoint /admin backend rotate-keys
   ```

   The check verifies ciphertexts without writing data. Apply performs one
   PostgreSQL transaction and refuses to proceed if it cannot lock the tables.
   An unreadable value or failed update rolls back the transaction. Both modes
   time out after ten minutes. After apply, the check should report
   `users=0 operators=0`.
4. Remove retired keys from the running configuration only after repeating the
   check successfully with those keys omitted. Keep them separately for as long
   as corresponding backups need to be restorable. Recreate backend and
   retention with the same active version, then resume bot traffic and check
   backend health, participant display and operator TOTP sign-in. TOTP setup
   or recovery started before the maintenance window may need to restart if
   its five-minute pending state is still encrypted with a retired key.

To revert to an earlier key version while keeping current data, load both keys
in the version-aware release and repeat the rotation with the earlier version
active. A binary from before versioned encryption requires a compatible
database backup and its keys.

## Audit PII digests and Redis login sessions

When retention anonymizes an eligible participant, it replaces selected
top-level audit payload fields with HMAC-SHA-256 using a random salt per value
and a key derived separately from the active AES key. The participant update
and audit payload updates commit together. Existing salted HMAC values are
stable on reprocessing. Previously processed history is outside the rotation
command, as are audit payload fields outside the existing retention policy.
These digests pseudonymize selected values and serve no audit-log integrity check.

The first upgrade to hashed Redis keys requires operators to sign in again
and restart in-progress sign-ins or password changes. Legacy keys expire using
their existing TTLs of up to 24 hours. Older Redis AOF files and Redis backups
can still contain plaintext keys from before upgrade. Apply the deployment's
backup and storage retention policy to those copies.

## Isolated k6 relay check

With Docker and k6 installed, run from `backend/`:

```powershell
$env:RUN_K6_RELAY = '1'
go test ./tests/repository -run '^TestK6AuthenticatedRelay$' -count=1 -v
Remove-Item Env:RUN_K6_RELAY
```

The test creates temporary PostgreSQL and Redis containers, applies the
migrations, seeds 50 synthetic sessions, and starts the real HTTP router on a
loopback port. It passes a disposable token file to k6. The 250 relay
requests must return valid enqueue results, meet the HTTP thresholds and
leave 250 queued messages across 50 sessions. The delivery worker stays
off during this check so no messages leave the fixture. This measures relay
and outbox enqueue latency. GitHub CI checks statement coverage separately
for API, auth, moderation, the backend service layer and bot handlers,
with a minimum of 70% per area.

## Monitoring

Add the monitoring Compose file to enable the backend scrape endpoint and
run Prometheus and Grafana. The default Compose configuration omits the
monitoring services. Prometheus reads a dedicated Bearer token from
a credential file. Grafana reads its initial administrator password from
a separate file. [Prometheus scrape authentication](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#http_config) ·
[Grafana Docker secrets](https://grafana.com/docs/grafana/latest/setup-grafana/configure-docker/)

1. Generate an independent token with `openssl rand -hex 32`. Create a
   separate strong password for Grafana. Store both in secret files outside
   the repository and set their absolute paths in `infra/.env`.
   On Linux, grant read access to backend UID 10001 and Prometheus UID
   65534 for the token, and to Grafana UID 472 for its password. Use file
   ACLs or groups to keep the files private. Grafana uses the password file
   when its data volume is first initialized. Rotate an existing admin
   password through Grafana.
2. From `infra/`, use your usual Compose project and overrides and add
   `docker-compose.monitoring.yml`. For the base stand:

   ```sh
   docker compose -f docker-compose.yml -f docker-compose.monitoring.yml config --quiet
   docker compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d --build --wait grafana
   ```

3. Open `http://127.0.0.1:3000/d/anonymus-overview` and sign in as `admin`
   using the configured password. If the installation runs on another host,
   use an SSH tunnel to its loopback port. Prometheus has no published port.
   Grafana is published on loopback and uses the provisioned Prometheus data
   source over the monitoring network.

Check **Backend scrape** and **Outbox query health** first. The HTTP panels
show request rate by status class and p95 latency without health probes.
HTTP labels use route templates, so paths with identifiers and query strings
are omitted. Outbox backlog includes queued and retrying messages, including
messages waiting for a participant. An outbox query failure sets its health
metric to zero and omits the backlog sample. Delivery rates count calls to
the bot service by kind and result. Use the operator panel for per-message
delivery status.

Prometheus retains up to seven days or 512 MB of TSDB blocks on its named
volume. Grafana keeps its state on a separate named volume. For local
validation, run `CHECK_MONITORING=1 bash infra/check-hardening.sh` from
the repository root to provision a disposable stand and verify scraping,
dashboard access and the monitoring network boundaries.

## Operator SSO with OpenID Connect

The default sign-in uses a local password and the existing second factor.
The optional SSO override replaces the password step for explicitly linked
operators. They still need an active account, a completed local password
setup and enrolled TOTP. The panel takes roles and permissions from the
local account.

1. Configure HTTPS for the panel using the [deployment guide](DEPLOYMENT.md).
   Register a confidential OpenID Connect web client for Authorization Code
   with PKCE S256. Register the exact callback URL, for example
   `https://panel.example.invalid/api/auth/oidc/callback`. The backend must
   trust the HTTPS certificates for the issuer and its discovery endpoints.
   [OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html)
2. Set the five OIDC variables from the [environment reference](ENV_VARS.md).
   Store the client secret and operator map in separate files outside Git.
   Limit read access to deployment administrators and backend UID 10001.
   Obtain each stable `sub` for this exact issuer and client from the IdP's
   administrative interface. Map it to the existing local operator ID.
   Use a separate entry per operator.

   ```json
   [{"subject":"example-provider-subject","operator_id":42}]
   ```

3. From `infra`, validate the merged configuration and recreate the backend
   behind the TLS edge. Keep every override already used by the deployment.

   ```sh
   docker compose -f docker-compose.yml -f docker-compose.tls.yml -f docker-compose.oidc.yml config --quiet
   docker compose -f docker-compose.yml -f docker-compose.tls.yml -f docker-compose.oidc.yml up -d --build --wait nginx
   ```

4. Confirm that `GET /api/auth/oidc` returns `{"enabled":true}`. Check SSO
   through the browser with a linked operator and confirm the local TOTP
   challenge, role and password fallback. The capability endpoint reports
   configuration. The first SSO attempt contacts the provider.

For secret, issuer or map changes, recreate the backend with the updated
settings. Removing a link stops subsequent SSO after the update. For
immediate access removal, disable the local operator to invalidate existing
sessions and pending challenges. Panel logout revokes the local session.
The IdP may retain its own login. To turn off SSO, recreate the backend
without the OIDC override, keeping TLS and the other active overrides, and
verify that the capability endpoint returns `{"enabled":false}`.

## Telegram webhook

The default mode is long polling. Webhook mode uses the same bot handlers
through a dedicated listener. Nginx exposes only `POST /telegram/webhook`
through TLS when the webhook override is enabled. The bot verifies
`X-Telegram-Bot-Api-Secret-Token` before reading the JSON body, and both
layers cap it at 256 KiB. The bot's internal service stays on its existing
private port. [Telegram webhook API](https://core.telegram.org/bots/api#setwebhook)

1. Configure a public HTTPS origin with a certificate trusted by Telegram.
   Use port 443 or another supported HTTPS port. Set the full endpoint URL
   and the dedicated secret file path in `infra/.env`. Generate this secret
   independently, for example with `openssl rand -hex 32`. Grant bot UID
   10001 read access to the file. [Telegram TLS and port requirements](https://core.telegram.org/bots/webhooks)
2. From `infra/`, use the same project and your current overrides. Add
   the webhook override after TLS. Start the edge before recreating the bot.

   ```sh
   docker compose -f docker-compose.yml -f docker-compose.tls.yml -f docker-compose.webhook.yml config --quiet
   docker compose -f docker-compose.yml -f docker-compose.tls.yml -f docker-compose.webhook.yml up -d --wait nginx
   docker compose -f docker-compose.yml -f docker-compose.tls.yml -f docker-compose.webhook.yml up -d --build --wait bot
   ```

3. Check the bot log for `bot started, listening for webhook updates`, then
   verify a private command with a designated test bot. Keep one running
   bot instance per token. Webhook registration preserves pending updates
   and requests one connection for message and callback updates.

To return to polling, stop the current bot, then recreate it with the
webhook override removed. Recreate the TLS edge with the same remaining
overrides to close the webhook path. Polling startup checks the current
Telegram webhook and removes it if present while preserving pending
updates. A shutdown in webhook mode leaves registration in place so
Telegram can retry delivery when the bot comes back.

The bot accepts one webhook request at a time and acknowledges it after the
handler returns. When busy it returns `503` so Telegram can retry. The
last 4,096 handled IDs are kept in memory for replay suppression. This
cache resets on restart. A process failure or a lost HTTP response around a
side effect can still cause a repeated update. Telegram limits how long
it retains pending updates. [Telegram update delivery](https://core.telegram.org/bots/api#getting-updates)
