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

## 2026-11-22 — P1 acceptance and password change

- The project owner reported all GitHub checks green for the preceding
  P1 changes. Accepted P1-12 and P1-13 on that basis. The exact run
  link was not supplied. Accepted P1-14 with the independent
  documentation-only deployment recorded as debt at the owner's request.
- P1-5. Added operator self-service password change in the panel and
  API. Bootstrap and self-service use a minimum of 12 characters and
  bcrypt's 72-byte UTF-8 ceiling. Existing accounts retain their
  current passwords until they choose to change them.
- `go test ./tests/repository -run '^TestPasswordChangeRevokesSessionsAndPendingChallenges$' -race -count=1`
  passed with temporary PostgreSQL and Redis. It checked authentication,
  invalid and incorrect passwords, stale writes, revocation of two
  sessions and a pending TOTP challenge, sanitized audit, throttling,
  and sign-in with the new password and the same authenticator.
- Full backend `go test ./... -race -count=1` with the CI coverage
  profile passed. Reported coverage was API 75.8%, auth 79.2% and
  moderation 100%. Frontend lint, typecheck and production build
  passed. Isolated Chrome with a synthetic API checked password
  confirmation, an incorrect current password and sign-out after success.
- P1-5 still requires forced change of temporary passwords, an
  administrator reset flow and TOTP reset/recovery codes. The current
  changes used no new migration or working-stand deployment.

## 2026-11-22 — P1-5 Temporary password rotation

- Migration 000008 adds the mandatory-change flag and initializes it
  for existing invited accounts. New panel-created operator accounts
  set the flag. Existing active accounts keep the ordinary sign-in flow.
- A verified temporary-password sign-in receives a five-minute token
  in a separate Redis namespace. It permits only password change and
  logout. Changing the password clears the flag and advances the
  authentication version in the same database write.
- The targeted repository race tests passed on disposable PostgreSQL
  and Redis. They checked the full migration round trip, migration
  from version 7 with active and invited accounts, TOTP enrollment,
  restricted route rejection and TTL, successful change, invalidated
  tokens and a new full sign-in with the same authenticator.
- Panel lint, typecheck and production build passed. Isolated Chrome
  confirmed the redirect after TOTP, direct URL restrictions, password
  validation and sign-out after the change. Swagger was regenerated.
- Full backend `go test ./... -race -count=1` with the CI coverage profile
  passed. Reported coverage was API 75.8%, auth 78.9% and moderation 100%.
  The server wiring test also passed with the new invitation assertion.
  golangci-lint v2.11.4 reported zero issues.
- Administrator password reset and TOTP reset/recovery are still
  open. The migration ran only on test databases.

## 2026-11-22 — P1-5 Administrator password reset

- Added password reset for another active or invited operator. The
  administrator receives a generated password in a no-store response.
  PostgreSQL updates the hash, rotation requirement and authentication
  version together. Existing TOTP enrollment is preserved. The audit
  entry identifies the administrator and target without recording the
  password.
- `go test ./tests/repository -run '^TestAdministratorPasswordResetRequiresRotationAndPreservesTOTP$' -race -count=1`
  passed using temporary PostgreSQL and Redis. It checked authorization,
  self-reset and suspended-account refusals, a stale write, password
  storage and audit, revocation, existing TOTP and required rotation.
- Panel lint, typecheck and production build passed. Isolated Chrome
  verified the available targets, cancellation, one authenticated
  request and removal of the displayed temporary password on close.
  Swagger was regenerated. The feature uses the existing migration 000008.
- Full backend `go test ./... -race -count=1` with the CI coverage profile
  passed. Coverage was API 76.1%, auth 78.9% and moderation 100%.
  golangci-lint v2.11.4 reported zero issues.
- TOTP reset and recovery codes remain open. The working deployment
  was outside these checks.

## 2026-11-22 — P1-5 TOTP recovery

- Migration 000009 adds a table for recovery-code digests. A signed-in
  operator can issue eight codes after verifying the current password
  and TOTP. Replacing codes revokes earlier sessions and challenges.
- Password sign-in and one recovery code issue a five-minute enrollment
  challenge. PostgreSQL consumes each code once, including during
  concurrent attempts. The old factor remains usable until the new
  factor is confirmed. Confirmation replaces the encrypted secret and
  revokes the previous sessions, challenges and remaining codes.
- Targeted repository tests with `-race -count=1` passed on temporary
  PostgreSQL and Redis. They checked issuance, replay, concurrency,
  unchanged TOTP before confirmation, revoked access afterward,
  audit without secrets and all migrations up, down and up.
- Panel lint, typecheck and production build passed. Isolated Chrome
  verified code display and cleanup, recovery, new QR enrollment and
  the transition to the panel only after TOTP confirmation.
  The Nginx rate-limit route passed `nginx -t` in a disposable
  container. Swagger was regenerated.
- The full backend `go test ./... -race -count=1` and coverage gate
  passed. Coverage was API 76.4%, auth 81.3% and moderation 100%.
  golangci-lint v2.11.4 reported zero issues. A subsequent targeted
  race run passed after moving the recovery audit to the database
  confirmation point, before issuing the new session.
- Recovery requires the current password and an unused recovery code.
  The feature provides no fallback when both the authenticator and
  all recovery codes are lost. Migration ran only on test databases.

## 2026-11-22 — P1-8 bot language and operator linking

- Private replies and delivery notices use the recipient's stored
  Russian or English language. The bot uses the Telegram language
  hint before a profile exists. Participant preferences take
  precedence when the same account is linked as an operator.
  Shared operator group notices use Russian.
- The panel issues a five-minute challenge. A private `/link` command
  claims it, and the panel confirms the code returned by the bot.
  PostgreSQL stores code digests, limits each Telegram account to one
  operator, and keeps the previous binding until replacement succeeds.
  Expired challenges become unusable immediately and are purged by
  retention. Internal HTTP access logs record route templates.
- Repository tests with `-race -count=1` passed for linking, invitation
  language at bind, outbox retry, and migration round trip. They covered
  service authentication, a second operator's refusal, code replay,
  expiry, suspension, relinking and expired-code cleanup with dry-run.
  `go test ./cmd/api ./internal/api -race -count=1` passed, including
  the follow-up check for redaction on normal responses and panics.
- Full backend and bot test suites passed with the CI race and coverage
  flags. Coverage was API 76.3%, auth 81.3%, moderation 100%, and
  bot handlers 77.6%. Go formatting, build, vet and golangci-lint
  v2.11.4 passed for both modules.
- Panel lint, typecheck and production build passed. Isolated Chrome
  with a fake API verified code issuance and replacement, invalid
  confirmation, status refresh and code cleanup after success.
  Migration 000010 ran on disposable databases. Live Telegram,
  the scheduled retention service and deployment were outside this run.

## 2026-11-22 — P1-7 browser tests

- Added Playwright 1.62.1 coverage for the built panel using synthetic
  API responses. The tests check unsuccessful and successful TOTP,
  history and loaded photo/audio previews, send-as after a recipient
  joins, both admin route guards, and the login flow at 375 px.
- `npm run test:e2e -- --repeat-each=3` passed in local Chromium with
  `CI=true` (nine passes). `npm run lint`, `npm run typecheck` and
  `docker build --tag anonymus-panel-e2e-check:local webpanel` passed.
- Frontend CI installs Chromium and runs `npm run test:e2e`, which
  includes the production build. actionlint 1.7.12 accepted
  `.github/workflows/frontend-ci.yml`. The new workflow awaits a
  GitHub PR run. The suite uses no running backend, Telegram, or
  deployment data.

## 2026-11-22 — P1-7 panel localization

- Added Russian and English for panel navigation, login and recovery,
  sessions, incoming requests, operator management, and audit controls.
  The switcher is available before and after sign-in. The preference
  is stored in the browser and updates document language and date
  formatting. Configured branding retains its supplied text.
- With `CI=true`, `npm run test:e2e -- --repeat-each=2` passed all ten
  runs. It checked the media and reply flow in each language and
  preference persistence and translation of an existing TOTP error.
  `npm run lint` and `npm run typecheck` also passed.
- `docker compose config --quiet` passed. Built the panel with
  `docker build --tag anonymus-panel-i18n-check:local webpanel` and
  verified the runtime configuration with and without a custom product
  name in disposable containers. The running stand was outside this
  check.

## 2026-11-22 — P1-7 dashboard and operator role editor

- Added the dashboard as the start page after sign-in. It shows up to
  five recent sessions and five new requests using the existing API.
  The backend limits ordinary operators to their assigned sessions.
  The dashboard and mobile menu support both panel languages.
- Administrators can confirm an operator role change in the panel.
  PostgreSQL serializes changes that can alter the active administrator
  set. The API returns HTTP 409 if a role or status update would remove
  the last active administrator. The existing auth-version trigger
  invalidates the affected operator's sessions on a role change.
- PostgreSQL and Redis fixtures verified role revocation, the last-admin
  refusal, concurrent role and status updates, and the session scope of
  an ordinary operator. Playwright checked the role dialog, dashboard
  navigation and empty/error states, mobile access and cache clearing
  after sign-out. With `CI=true`, `npm run test:e2e -- --repeat-each=2`
  passed all 18 runs. `npm run lint`, `npm run typecheck`, the panel
  Docker build, `go vet ./...` and golangci-lint 2.11.4 passed.
- Full backend `go test ./... -race -count=1` passed with the CI
  coverage flags. Coverage was API 76.4%, auth 81.3%, and moderation
  100%. Separate Docker builds of the panel and backend passed.
  No new migration is needed. No working-stand or GitHub PR run was
  part of this verification.

## 2026-11-22 — P2-1 and P2-2 source and repository cleanup

- Translated current source comments and regenerated Swagger with
  swag v1.16.6. Compared Go tokens in 32 comment-only files and checked
  Swagger paths, models, parameters and response codes. Preserved the
  original migration files.
- Removed the unused Redis setting from the bot and its Compose
  service. Removed the unused backend session index and message
  deletion helpers, and stopped reading and writing the legacy media
  path. The existing schema and session-token validation still apply.
- Removed both tracked coverage snapshots, added ignore rules, and
  renamed the retention test helper. CI still generates and checks
  fresh coverage profiles.
- Full `go test ./... -race -count=1` suites passed with the CI coverage
  flags. Coverage was API 76.4%, auth 87.7%, moderation 100%, and bot
  handlers 77.6%. golangci-lint v2.11.4, frontend lint and typecheck,
  both Compose config checks, and separate backend and bot Docker
  builds passed. Test database migrations used disposable databases.

## 2026-11-22 — P2-7 backend shutdown

- Added SIGTERM and interrupt handling. The HTTP listener closes and
  active requests have up to 100 seconds to finish. The delivery
  worker stops claiming items and lets a claimed item finish its send
  and database update. Compose gives the backend two minutes to exit.
- `go test ./cmd/api -run 'Test(Serve|HTTPServer|ClientIP)' -race -count=5`
  passed. Tests exercised an active request during shutdown, forced
  close at the deadline, listener failure, worker timeout and the
  established HTTP timeout and proxy settings.
- Targeted repository tests passed with temporary PostgreSQL and a
  fake bot. One in-flight delivery was written as delivered after
  cancellation, and the next message remained queued and unclaimed.
- Base and dev `docker compose config --quiet` passed. On a disposable
  Compose project, `docker compose stop backend` produced the
  `backend stopped` log and exit code 0. Containers and volumes were
  removed. The working stand and Telegram were outside the check.
- Full backend `go test ./... -race -count=1` passed with CI coverage
  flags. Coverage was API 76.4%, auth 87.7%, and moderation 100%.
  `go vet ./...` and golangci-lint v2.11.4 passed.

## 2026-11-23 — P2-4 container hardening

- Backend, migration, bot, retention and panel now use UID/GID 10001.
  Compose sets read-only roots and CPU, memory and PID limits. Edge
  Nginx retains a restricted master and runs workers as nginx.
  PostgreSQL uses the upstream privilege-dropping entrypoint.
- Media initialization prepares fresh volumes and existing top-level
  files. Verified API reads from read-only mounts, ImageMagick writes
  as the bot UID and fixture removal as the retention UID.
  The same Redis password is still required. Its AOF and data volume
  preserve Redis state across container replacement.
- `bash infra/check-hardening.sh` passed on Docker Desktop. It built
  backend, bot, retention and panel, validated base, dev and TLS
  Compose, fetched HTTP and certificate-verified HTTPS health,
  checked the panel's JS MIME type and runtime branding, ran retention
  in dry-run mode, and verified Redis refused unauthenticated calls
  and restored a test key after recreation.
- The isolated migration round-trip test with `-race -count=1`
  and all nine Playwright scenarios passed. actionlint validated
  the updated infrastructure workflow. GitHub PR execution and
  production certificate, token-file and volume permissions were
  outside the local verification.

## 2026-11-23 — P2-5 encryption rotation and credential storage

- Backend and retention accept a versioned AES-GCM keyring. Existing
  unversioned PII and TOTP ciphertexts remain readable using version 1.
  The `admin rotate-keys` check scans stored values without writing;
  `--apply` rewrites outdated values in one transaction. Tests confirmed
  rollback on corrupted data, refusal during conflicting writes,
  preservation of operator authentication state, and a zero-change
  rerun after removing the retired key from the test configuration.
- Audit retention writes salted HMAC-SHA-256 for the existing PII field
  scope. A failed digest update rolls back participant anonymization,
  which leaves the participant eligible for retry. The existing audit
  history is outside the key rotation command.
- Redis uses SHA-256 keys for regular and password-change sessions,
  pending authentication and TOTP enrollment. Integration tests checked
  TTLs, revocation, one-time challenge consumption, and rejection of
  both a legacy key and an attempted bearer token using the stored digest.
- The focused Go tests for crypto, config, auth, retention and admin passed
  with `-race -count=1`. The PostgreSQL and Redis tests passed in
  disposable containers. The full backend `go test -race -count=1 -p 2 ./...`,
  backend `go vet ./...`, and the full bot `go test -race -count=1 ./...`
  passed.
- `bash infra/check-hardening.sh` passed after correcting the rotation
  fixture invocation. It validated base, dev and TLS Compose and
  exercised check, apply, and check with only the active key from the
  built backend image. Temporary containers and volumes were removed.
- See [Operations](OPERATIONS.md#encryption-key-rotation) for maintenance,
  key retirement, backups and the one-time operator re-login. The GitHub
  workflow run and rotation of production data were outside this check.

## 2026-11-23 — P2-6 PostgreSQL integrity and runtime privileges

- Migration `000011_participant_integrity` rejects the same participant
  in both slots of one session while allowing empty slots and reuse
  across sessions. It protects participant, sender and creator references,
  clears a deleted operator's assignment where otherwise allowed,
  and adds five indexes for foreign-key lookups. The existing
  `idx_audit_log_action_order` already covers the action filter.
- Compose now gives backend and retention the separate `anonymus_runtime`
  login. The one-shot `db-permissions` job waits for a successful
  migration, provisions explicit grants and verifies runtime login.
  Schema migrations, first-admin bootstrap and backups use the owner.
- `go test ./tests/repository -run '^Test(ParticipantIntegrityMigration|AllMigrationsRoundTrip)$' -race -count=1`
  passed on disposable PostgreSQL. Tests verified legacy conflict
  rejection without row edits, migration state recovery and retry,
  allowed cross-session membership, FK actions and reversible migration.
- `bash infra/check-hardening.sh` passed. The disposable Compose project
  verified base, dev and TLS configurations, repeated role provisioning,
  allowed runtime operations and denied schema, audit and bootstrap
  operations. Backend and retention used the runtime login. The project
  removed its temporary containers and volumes. `actionlint` v1.7.12
  validated the infrastructure workflow.
- Full backend `go test -race -count=1 -p 2 ./...`, bot
  `go test -race -count=1 ./...`, and backend `go vet ./...` passed.
  Shell syntax, Go formatting and `git diff --check` passed.
- [Deployment](DEPLOYMENT.md#postgresql-roles-and-upgrade-to-schema-11)
  documents the password and existing-data check. Production migration,
  restore and GitHub CI execution were outside this validation.

## 2026-11-23 — P2-3 service layer and P2-8 coverage and k6

- Extracted participant relay, invitation binding and session status
  transitions into `backend/internal/service`. Gin keeps request
  validation, authorization and response mapping. The API uses a common
  session ID parser and the session model resolves participant roles.
- Focused router integration tests passed for authorization, invite
  lifecycle, relay decisions, media, moderation and status changes.
  The relay tests distinguish rejected messages from messages that must
  stay queued while the counterpart is absent.
- Backend CI now measures the service package separately at the existing
  70% floor. A synthetic coverage profile confirmed that the checker
  rejects 69% and accepts 70%. Backend `go vet ./...` and the workflow
  syntax check passed.
- Full race tests and coverage gates passed with the CI package lists.
  The backend ran `go test ./... -race -count=1 -p 2 -coverpkg=./internal/api,./internal/auth,./internal/moderation,./internal/service -coverprofile=<temporary-profile>`
  directly under Go 1.26.0 after a local toolchain mismatch. Coverage
  was API 76.4%, auth 88.4%, moderation 100%, and service 89.1%.
  Bot `go test ./... -race -count=1 -coverpkg=./internal/handlers -coverprofile=<temporary-profile>`
  yielded 77.6% for handlers.
- `RUN_K6_RELAY=1 go test ./tests/repository -run '^TestK6AuthenticatedRelay$' -count=1 -v`
  passed with k6 2.2.0 on isolated PostgreSQL and Redis. All 250 HTTP
  requests succeeded and 250 messages remained queued in 50 sessions,
  with no sender mismatches. Local HTTP p95 was 56.9 ms. Initialization
  rejected a non-loopback base URL. The delivery worker stayed off and
  no Telegram requests were made.
- [Operations](OPERATIONS.md#isolated-k6-relay-check) documents the
  repeatable test. GitHub CI execution and production load were outside
  local validation.

## 2026-11-23 — P3 white-label

- The panel loads an accent palette, default light/dark/auto scheme and
  localized sign-in subtitles from runtime configuration alongside the
  existing name and bot username. Container startup validates theme settings.
  The browser renders branding as plain text and falls back on malformed
  optional fields. The bot supports an optional organization name and
  independent Russian/English `/start` greetings for joins and requests.
- Panel `npm.cmd run lint` and `npm.cmd run test:e2e` passed. The latter
  built the production bundle and passed 12 Chromium tests, including
  runtime branding, malformed optional values and system theme switching.
- Bot `go test -race -count=1 ./...` passed with local Telegram and backend
  stubs for customized and default replies. The CI coverage check passed
  at 78.1% for handlers.
- `bash infra/check-hardening.sh` passed in the disposable Compose project.
  It checked base, dev and TLS configuration, the production panel's
  runtime JSON, invalid-accent rejection and delivery of the branding
  environment to the bot container. The bot process did not start in
  that Compose check.
- See [White-label settings](ENV_VARS.md#white-label-settings) for variables
  and example values. Production rollout, GitHub CI execution and delivery
  through the real Telegram API were outside this validation.

## 2026-11-23 — P3 monitoring

- Backend optionally exports Prometheus metrics with a dedicated Bearer
  token. HTTP metrics use route templates and status classes. The outbox
  collector reports queued/retrying counts or a collection failure within
  its query deadline. The delivery counters track bot service calls and
  their outcomes separately from persisted delivery status.
- The Compose override provisions Prometheus and Grafana OSS with bounded
  resources and separate data volumes. Grafana is bound to loopback and
  requires authentication. Prometheus has no host port and Nginx blocks
  the scrape path. The verified image versions are Prometheus 3.13.5 and
  Grafana OSS 13.2.4. The Go client is pinned at 1.23.2.
- `go test -race -count=1 ./internal/metrics ./cmd/api` passed, including
  exporter authorization, privacy-safe route labels and outbox read
  failure. The isolated outbox integration test verified pending counts
  and attempt results against a fake bot. `go vet ./...` and
  `actionlint` v1.7.12 passed.
- The full backend race suite passed using the CI coverage flags.
  Coverage was API 76.4%, auth 88.4%, moderation 100% and service 89.1%.
- `CHECK_MONITORING=1 bash infra/check-hardening.sh` passed after fixing the
  Windows HTTP status probe and disabling Grafana plugin preinstallation.
  The disposable project verified the monitored backend scrape, rejection
  of an internal API token at the scrape endpoint, edge 404 responses,
  authenticated dashboard provisioning and a live outbox sample queried
  through Grafana. It validated the monitoring and TLS overrides together
  and removed the temporary project and volumes.
- [Monitoring](OPERATIONS.md#monitoring) documents deployment and access.
  The working stand, GitHub CI execution and live Telegram delivery
  were outside this validation.

## 2026-11-23 — P3 Telegram webhook

- The optional Compose override enables a separate bot listener behind one
  HTTPS Nginx route. It validates a dedicated secret and limits request
  bodies to 256 KiB. The bot acknowledges updates after the handler returns
  and remembers the last 4,096 accepted IDs in memory. The cache is local
  to the process and updates can be repeated across restart or lost
  acknowledgements.
- Polling remains the default. Switching to polling removes a previous
  webhook without dropping pending updates. Webhook shutdown drains the
  accepted request and leaves registration in place.
- `go test -race -count=1 ./cmd/bot ./internal/config` passed. The full bot
  race suite with CI coverage flags and `go vet ./...` passed. Handler
  coverage was 78.1%. Fake Telegram responses covered registration,
  switching modes, sanitized failure, authentication, size and JSON
  limits, replay, backpressure and graceful shutdown.
- `CHECK_MONITORING=1 bash infra/check-hardening.sh` passed. The disposable
  project validated the Compose overrides together, checked the TLS route
  before and after enabling the webhook, and forwarded the credential to
  a fake upstream. It confirmed method and private route restrictions and
  removed its containers and volumes.
- [Telegram webhook operations](OPERATIONS.md#telegram-webhook) documents
  setup and rollback. The working stand, actual Telegram registration and
  GitHub CI execution were outside this validation.

## 2026-11-23 — P3 transcript export

- Added role-gated CSV and PDF downloads to session details. The server
  exports retained messages in chronological order with delivery status
  and sender attribution. The export includes captions and omits
  attachment files, file references, participant IDs and session titles.
- The backend caps export rows, source text bytes, PDF pages and concurrent
  exports. CSV protects formula-like cells and PDF embeds a Cyrillic-capable
  font. An audit event must be recorded before returning an attachment.
- `go test -race -count=1 ./internal/transcript ./internal/api` and the
  isolated `TestSessionTranscriptAccessContentAndLimits` passed. They
  covered role boundaries, session isolation, chronological ordering and
  combined size limits. Poppler independently extracted the expected
  Russian and English text from a generated fixture PDF. `go vet ./...`
  passed. Swagger was regenerated for the new endpoint.
- `npm.cmd run lint` and `npm.cmd run test:e2e` passed. The browser suite
  passed all 14 scenarios after correcting the test navigation to preserve
  the in-memory session. It verified both downloads and the limit message.
- `bash infra/check-hardening.sh` built the backend and panel images and
  passed the isolated Compose checks.
- The full backend race suite and the existing CI coverage thresholds
  passed. Coverage was API 76.6%, auth 88.4%, moderation 100% and service 89.1%.
- The working stand and GitHub CI execution were outside this validation.

## 2026-11-23 — P3 operator SSO

- Added opt-in OpenID Connect sign-in in the backend and panel, an
  OIDC Compose override with two backend-only secrets, and a TLS
  callback route. The browser shows the SSO action when the backend
  reports it enabled.
- The backend uses Authorization Code with PKCE S256, a browser-bound
  state, a nonce and a short-lived Redis handoff. It verifies the signed
  ID token and maps its subject to an existing operator. The operator's
  current local role, account status and second factor still apply.
- `go test -race -count=1 ./internal/config ./tests/repository -run '^(TestLoadOIDC|TestOIDCWithSignedProviderAndLocalTOTP)$'`
  passed with a signed HTTPS provider fixture and temporary PostgreSQL and
  Redis. It covered invalid settings and claims, one-use handoff, account
  restrictions, auth-version changes and successful local second factor.
  `go vet ./...` passed.
- The full backend run passed with `go test ./... -race -count=1 -p 2`,
  using the CI coverage package list and the coverage gate. Statement
  coverage was API 77.0%, auth 86.2%, moderation 100% and service 89.1%.
- `npm.cmd run lint`, `npm.cmd run typecheck` and `npm.cmd run test:e2e`
  passed. All 17 browser scenarios passed, including provider return,
  required TOTP, and password sign-in when SSO is disabled or fails.
- `bash -n infra/check-hardening.sh` and `bash infra/check-hardening.sh`
  passed. The isolated stack validated base and merged Compose
  configuration, built the images, ran `nginx -t`, loaded both synthetic
  OIDC files and confirmed callback query values were absent from the
  edge and backend logs. The test removed its containers and volumes.
- The same smoke test passed with `CHECK_MONITORING=1`, as used by the
  infrastructure CI job.
- The [SSO operations guide](OPERATIONS.md#operator-sso-with-openid-connect)
  covers enabling SSO, rotating credentials and revoking access. GitHub CI
  and sign-in through the deployment's real IdP remain unverified. This
  work used synthetic data and isolated services.

## Entry template
### YYYY-MM-DD — [ID] Short title
- **Status:** planned / in progress / completed / blocked
- **Files changed:**
- **Behavior/change:**
- **Checks run:** exact commands and observed outcomes
- **Remaining risks / follow-up:**
