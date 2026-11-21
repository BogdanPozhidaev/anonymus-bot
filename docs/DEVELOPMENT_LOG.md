# Development Log

Use this file as an append-only record of verified work. Add date, task/issue ID, files changed, behavior, checks actually run, results, and remaining risks. Do not record planned work as completed.

## 2026-11-21 — Baseline audit and documentation pack
- Reviewed uploaded repository archive at Git HEAD `5e63f63` using static code/config inspection.
- Prepared an independent audit addendum and revised development roadmap.
- No application source code, migrations, or deployment configuration were changed as part of that audit.
- Backend/bot tests were blocked by the audit environment's Go version; frontend dependency install timed out; Docker checks were unavailable. See `DEVELOPMENT_ROADMAP.md` for exact scope and limitations.

## 2026-11-21 — P0-1 (backend scope) Internal API authentication

- **Status:** completed for the backend API and its clients.
- **Files changed:** backend API startup, router and middleware, bot configuration
  and HTTP client, Go tests, Compose and Nginx configuration, k6 client and
  deployment documentation.
- **Behavior:** all 12 internal routes require the service token. Compose mounts
  the file into backend and bot and publishes backend port 8080 on loopback only
  in the development override. Nginx rejects the internal proxy path.
- **Checks run:** `go test ./cmd/api ./internal/... -count=1` in backend and
  `go test ./... -count=1` in bot passed. Base and merged Compose
  `config --format json` confirmed port bindings and secret grants. An isolated
  Compose fixture confirmed read access to the synthetic secret from both
  service roles. `nginx -t` passed and HTTP probes confirmed route boundaries.
- **Remaining risks:** the complete application stack, k6 workload, migrations
  and Telegram flows were not run. The service token travels over HTTP within
  the trusted Docker network.

## 2026-11-21 — P0-1 (bot scope) Internal API authentication

- Protected both bot HTTP routes with a separate backend-to-bot token.
  The backend client sends it on both calls and rejects redirects.
- `go test ./cmd/api ./internal/api ./internal/botclient -count=1` in backend
  and `go test ./... -count=1` in bot passed. The bot tests used a fake sender.
- Base and development Compose configuration checks passed. An isolated
  stand-in stack verified both secret mounts in both service roles and
  rechecked the existing Nginx rules. No application deployment or Telegram
  requests were made. The Swagger setting is outside this change.

## 2026-11-21 — P0-2 Session invitations and atomic binding

- Added migration 000004, operator issuance and revocation, a transactional
  redemption flow, token handling in the bot, and invitation controls in the panel.
- `go test ./cmd/api ./internal/... ./tests/repository -count=1` passed in backend.
  Disposable PostgreSQL covered expiry, revocation, reissue, replay, status checks,
  concurrent claims, dual-role rollback and pending message access.
- `go test ./... -count=1` passed in bot, including a local HTTP fixture for
  the backend and Telegram. `npm.cmd run build` passed in webpanel. Full lint
  failed on ambiguous tsconfig roots. Both changed frontend files passed ESLint
  when tsconfigRootDir was explicitly set to the main webpanel.
- No production database, real Telegram service or live deployment was used.

## 2026-11-21 — P0-7 Bound Telegram media downloads

- Photo and voice handlers use a shared downloader with a 20-second request
  budget and a 20 MiB file limit. Failed downloads remove the temporary file.
  Redirects are rejected and errors omit credential-bearing URLs and responses.
- `go test ./internal/handlers -run '^TestMediaDownload' -count=1` and
  `go test ./... -count=1` passed in bot. Tests used local HTTP fixtures
  for deadlines, size limits, cleanup, redirects and safe diagnostics.
- No real Telegram download or live deployment was tested.

## 2026-11-21 — P0-4 Operator authentication throttling

- Added atomic Redis attempt limits for password login and TOTP verification,
  uniform credential errors, and one-time pending-token consumption.
- Configured a dedicated Compose proxy network and a single trusted Nginx
  address. Nginx preserves auth paths and replaces the client IP header.
- `go test ./cmd/api ./tests/repository -run 'Test(ClientIPTrustsOnlyConfiguredProxy|RedisAuthAttemptLimits|AuthThrottleHandlers)$' -count=1`
  and `go test ./cmd/api ./internal/... ./tests/repository -count=1` passed
  in backend with disposable Redis and PostgreSQL.
- Base and development Compose configuration checks, `nginx -t`, and HTTP
  probes for both auth paths and spoofed headers passed on an isolated stack.
- Production deployment and browser login were not tested. Password policy
  and existing-session revocation are tracked under P1-5 and P0-5.

## 2026-11-21 — P0 completion: access, audit, and retention

- Added migration 000005. Operator role and status changes invalidate existing
  sessions and pending challenges. Audit is admin-only with typed actors,
  validated filters, and bounded JSON and CSV pages.
- Retention rechecks session eligibility, preserves failed media records, and
  purges incoming requests using the PII retention interval. Media operations
  validate identifiers. Backend validates bot and media configuration at startup,
  Swagger defaults to off, and Nginx limits bursts on the auth routes.
- `go test ./... -count=1` and `go vet ./...` passed in backend and bot.
  Focused backend repository and retention tests passed with `-race`.
  The 004 → 005 upgrade and rollback passed with synthetic PostgreSQL data.
  Fake-bot tests covered alert, send-as, and stop. No Telegram API was called.
- `npm.cmd run build` and targeted ESLint passed in webpanel. Chrome with
  a synthetic API verified admin audit pagination and the operator route guard.
  Base and dev Compose validation, Nginx syntax, and isolated HTTP checks
  passed, including auth throttling and the internal route boundary.
- Migration 000005 and the updated retention policy were tested on disposable
  databases. The running stack and its database were not changed.

## 2026-11-21 — P0 local rollout and smoke check

- Built backend and retention images, applied migration 000005 to the local
  Compose database, and recreated the backend. Migration state is version 5
  with `dirty=false`. The health endpoint returned HTTP 200.
- Ran retention with `--entrypoint /retention retention -dry-run`. It reported
  zero eligible users, messages, files, and incoming requests, and no errors.
- Used temporary database accounts to verify live TOTP sign-in through Vite,
  admin-only audit access, and old-session rejection after suspension and
  reactivation. Chrome confirmed operator logout from an already open tab
  when its next request was denied. Temporary accounts and audit rows were removed.
- The scheduled retention container and real Telegram delivery were outside
  this smoke check. Existing operator TOTP settings were preserved.

## 2026-11-21 — P0 closure review

- Reconciled all eight original P0 rows with their acceptance evidence in
  `DEVELOPMENT_ROADMAP.md`. Explicitly carried password policy into P1-5
  under D-006 and recorded the local rollout boundary.
- Re-ran `go test ./cmd/api ./internal/api ./internal/botclient ./tests/repository ./tests/retention -count=1`
  in backend and `go test ./... -count=1` in bot. Both passed.
- Verified base and dev Compose port publication. A disposable backend image
  exited with code 1 when required media and bot settings were missing.
  The live local backend returned 200 for health, 401 for an internal route
  without credentials, and 404 for Swagger. Local database version was 005
  with `dirty=false`, and no temporary P0 operator accounts remained.

## 2026-11-21 — P1-4 First startup and administrator bootstrap

- Added a one-shot Compose migration service, read-only migration status,
  sanitized startup validation, Redis password authentication, and a
  first-admin CLI that reads the password on stdin.
- Migration 000006 and a PostgreSQL table lock make bootstrap one-time and
  safe under concurrent requests. Repeating with the same login preserves the
  existing password and account. Previously initialized databases stay closed.
- `go test ./... -count=1` passed in backend and bot. Follow-up tests
  `go test ./cmd/admin ./internal/config -count=1` and
  `go test ./tests/repository -run '^TestBootstrap' -count=1` passed in backend.
- A separate Compose project verified a clean boot to version 6, idempotent
  migrations, missing Redis configuration, unauthenticated Redis rejection,
  dirty-state rejection, repeat bootstrap, second-login refusal and live
  first-admin API login with TOTP. The fixture containers and volumes were removed.
- The existing stack was not updated. No Telegram request or scheduled
  retention run was performed.

## 2026-11-21 — P1-2 Media lifecycle

- The bot keeps media after immediate and deferred sends. Backend mounts
  both media volumes read-only. Retention removes published files after the
  existing cutoff and keeps rows on failure. It preserves shared references.
- `go test ./... -count=1` passed in bot. The fake Telegram test checked
  all four send branches and verified file contents after delivery.
- `go test ./tests/repository ./tests/retention -count=1` passed in backend.
  Tests covered preview contents and MIME headers, successful cleanup,
  repeat cleanup, shared references, failures and retry.
- `npm.cmd run build` passed. Chrome decoded image and audio previews
  via authenticated synthetic endpoints. Base/dev Compose mounts and a
  disposable container fixture confirmed write, read-only and delete roles.
- No Telegram send or scheduled retention was run on the working stand.

## 2026-11-21 — P1-1 Durable message delivery

- Migration 000007 and a backend worker track each delivery separately
  from its message. The message and outbox item are written atomically.
  Binding makes queued messages eligible without marking them delivered.
- Protected bot delivery handles text, photo and voice, including sends
  on behalf of a participant. Five unsuccessful attempts surface a
  failure in history and enqueue a separate participant notification.
- `go test ./... -count=1` passed in backend and bot. The final targeted
  repository run covered outbox, server wiring and invitation lifecycle.
  The outbox test also passed with `-race`. Tests covered transaction
  rollback, wait-for-bind, retry, terminal failure, notice retry, stale
  leases and duplicate processing after acknowledgement.
- `npm.cmd run build` passed. Chrome with a synthetic API confirmed
  queued, retrying, failed and unverified legacy labels in history.
- Migration was applied only to disposable databases. No real Telegram
  sends or deployment were performed. Legacy messages are not replayed.

## 2026-11-21 — P1-3 Nginx and production panel

- Nginx preserves the API prefix and serves the production panel through
  its separate static image. Added dependency readiness checks, a 24 MiB
  edge body limit, browser security headers and an opt-in TLS override.
- `go test ./internal/api ./tests/repository -run 'Test(ServerWiringWithFakeBot|InternalRoutesRequireServiceToken|InternalTokenAllowsValidRequest)$' -count=1`
  passed in backend. `go test ./... -count=1` passed in bot, and
  `npm.cmd run build` passed in webpanel.
- Base/dev/TLS `docker compose config --format json` passed with synthetic
  variables. On a fresh disposable stack `docker compose up -d --build
  --wait nginx` reached healthy states for PostgreSQL, Redis, backend,
  panel and Nginx. `nginx -t` passed for HTTP and TLS.
- HTTP probes verified /api/health, no duplicate prefix, internal route
  blocking, SPA fallback, asset status and security headers. About 20 MiB
  reached the JSON handler and 25 MiB received 413. Chrome verified TOTP
  sign-in to the built panel through Nginx, and HTTPS health passed with
  explicit verification of a temporary certificate.
- Alternate `PANEL_BASE_URL` values were verified in resolved Compose
  configuration and bot configuration tests. The existing stand, live
  certificates, scheduled retention and real Telegram were outside this run.

## 2026-11-21 — P1-12 Test coverage and P1-13 CI

- Verified the J.1–J.5 scenarios across package and integration tests.
  Added admin/owner/other/anonymous access checks, authenticated media
  previews, request and invitation refusals, moderation relay cases,
  retention file-path failure, bot handler fixtures, same-user update
  ordering, failed sends and redaction. Existing Testcontainers tests
  exercised persistence, concurrent binding and outbox delivery states.
- In backend, `go test ./... -race -count=1 -coverpkg=./internal/api,./internal/auth,./internal/moderation -coverprofile=$p`
  passed. In bot, `go test ./... -race -count=1 -coverpkg=./internal/handlers -coverprofile=$p`
  passed. Profiles were written to temporary paths. The coverage checker
  reported `api` 75.8% (806/1064), `auth` 78.0% (78/100),
  `moderation` 100.0% (46/46) and `handlers` 77.7% (317/408).
  Repeated dispatcher checks passed with `go test ./internal/dispatcher -race -count=20`.
- `go build ./...`, `go vet ./...`, Go formatting and golangci-lint
  `v2.11.4` passed for both modules. Formatting was checked with LF
  line endings, as on the Linux runner.
- `npm run lint`, `npm run typecheck` and `npm run build` passed.
  The production Node 22 image ran `npm ci` before its build. Vite reported
  a size warning for the main bundle.
- Base, dev and TLS `docker compose config --quiet` passed with synthetic
  settings. `docker compose build backend bot retention panel` built all
  four images in an isolated project. The panel image served its
  environment-derived branding config with `Cache-Control: no-store`.
- `go test ./tests/repository -run '^TestAllMigrationsRoundTrip$' -count=1 -v`
  passed against a temporary PostgreSQL instance. It checked all
  migrations up, no application tables after down, and the same clean
  version after applying up again.
- `actionlint` v1.7.12 validated the four workflows. They run on
  pull requests and pushes with coverage, frontend, image and migration
  checks. An actual GitHub Actions run on a PR remains pending.

## 2026-11-21 — P1 functionality and operations

- P1-5. Isolated tests checked that an unconfirmed enrollment never
  changes the PostgreSQL account, that the temporary Redis value and
  confirmed PostgreSQL secret are encrypted, and that a superseded
  enrollment cannot replace the first confirmed secret. Confirmed login
  updates `last_login_at`. Legacy plaintext secrets upgrade on the
  next successful TOTP verification. Password changes, admin reset and
  TOTP recovery remain open.
- P1-6. `go test ./internal/moderation -run '^TestLabelledCorpus$' -count=1 -v`
  measured 94.05% precision and 97.53% recall across 161 synthetic cases.
  Integration tests covered captions, the default notification policy,
  opt-in pause, session-scoped counters, resume reset and Redis failure.
- P1-7 and P1-9. The panel supports runtime branding, paged session
  search and history, send-as, owner reassignment and delivery labels.
  A headless Chrome smoke using the built panel and a synthetic API
  passed login, branding, search, pagination, send-as and closure
  confirmation. API tests covered 64 KiB body boundaries, server
  timeouts, pagination and sanitized health. Panel localization and
  the remaining API list bounds are still open.
- P1-8. The bot polls with cancellation and drains the accepted batch
  before advancing the Telegram offset. Fake-poller tests verified
  draining, offset progression and redaction. User linkage and
  incoming-request flow work remain open.
- P1-10 and P1-11. `go test ./tests/repository -run '^Test(TOTPEnrollmentRequiresConfirmationAndEncryptsSecret|AuthThrottleHandlers|ServerWiringWithFakeBot)$' -race -count=1`
  passed with temporary PostgreSQL and Redis. Fixtures verified the
  audit event counts and neutral pause/close notices to both recipients.
  Browser cancellation sent no request, and confirmation sent one
  authenticated status update.
- P1-14. Added handover, operations, security and data-handling
  documentation. Regenerated Swagger with
  `go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g main.go -d cmd/api,internal/api,internal/models -o docs/swagger`.
  A documentation-only deployment by a new engineer remains open.
- P1-15. Ran `bash deploy/backup/backup.sh backup PROJECT ENV_FILE DIRECTORY`
  and the matching `restore` command in two fixture projects. Compared
  the clean schema version, one session record and both media byte
  streams. A second restore into the populated target and one with
  a corrupted archive were refused. Temporary Docker resources were
  removed. The working deployment and Telegram were outside the run.

## 2026-11-22 — P1 pagination and acceptance

- P1-9. Added SQL-backed page limits and stable ordering to the five
  remaining public and internal list routes. The panel has previous and
  next controls for requests, operators and owner selection. The bot
  menus request ten rows and preserve access to the selected session.
  The targeted `go test ./tests/repository -run '^TestServerWiringWithFakeBot$' -count=1 -race`
  passed. It checked distinct pages for all five routes, invalid page
  parameters and the selected session's position.
- P1-7. Added the panel error boundary. `npm run lint`,
  `npm run typecheck` and `npm run build` passed. Isolated Chrome checked
  both panel list pagers, the status filter's offset reset and the
  error boundary using synthetic API responses.
- P1-8. Direct HTTPS buttons open the authenticated panel. HTTP
  development retains the callback. Private requests reuse existing
  rows, avoid duplicate alerts, and keep the first text message.
  Bot tests used local stubs, and concurrency/cooldown tests used
  temporary PostgreSQL.
- P1-14. Followed the revised isolated Compose instructions with
  a unique project and a separate fixture environment. `docker compose config --quiet`,
  startup through Nginx and `nginx -t` passed. Migration status was
  `version=7 dirty=false`. Created a first administrator, enrolled TOTP,
  signed out and signed back in, then verified session creation, listing,
  details and static panel routing. Only the fixture project was removed.
  Independent acceptance and an actual PR run remain pending.
- Go formatting and golangci-lint v2.11.4 passed for both modules.
  Regenerated Swagger with the existing pinned generator. The working
  deployment and real Telegram were outside these checks.
- The full `go test ./... -race -count=1` runs with the CI coverage
  package lists and temporary coverage profiles passed. The coverage
  gate reported API 76.1% (836/1099), auth 78.0% (78/100), moderation
  100.0% (46/46) and handlers 75.5% (351/465).

## Entry template
### YYYY-MM-DD — [ID] Short title
- **Status:** planned / in progress / completed / blocked
- **Files changed:**
- **Behavior/change:**
- **Checks run:** exact commands and observed outcomes
- **Remaining risks / follow-up:**
