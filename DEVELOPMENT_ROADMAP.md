# Anonymus Bot — Development Roadmap (Phase 0 Audit, revision 3)

> Place at repository root as `DEVELOPMENT_ROADMAP.md`.
> **Revision 3 note (2026-11-21):** This revision includes an independent read-only review of the complete uploaded `anonymus-bot.zip`, with repository paths preserved. It corrects several revision 2 statements that were based on an incomplete/flattened upload. The repository architecture is Gin + PostgreSQL + Redis + `go-telegram-bot-api` + React/Mantine. Findings remain static unless explicitly stated otherwise.

## 0. Scope, method, limits

**Independent review input:** `/mnt/data/anonymus-bot.zip` (273 ZIP entries; 206 non-directory entries), with paths preserved and Git metadata included. The archive contains 83 Go source files, all 6 up/down migration files for migrations 000001–000003, both CI workflows, both Docker Compose files, Nginx config, the full `webpanel/src` tree (including `api/`, `stores/`, and `types/`), package manifests/lockfile, and 8 project documentation files.

**Inspected:** backend and bot source/config/tests; all migrations; `infra/docker-compose.yml`, `infra/docker-compose.dev.yml`, `infra/nginx/nginx.conf`; backend/bot/webpanel Dockerfiles; `.github/workflows/backend-ci.yml` and `bot-ci.yml`; `.env.example` and `infra/.env.example`; all supplied `docs/*`; full `webpanel/src/*`; `webpanel/package.json`, lockfile, Vite/TypeScript/ESLint config; committed coverage artifacts; Git HEAD and working-tree diff metadata.

**Important repository/archive observations:**
- Git HEAD in the archive is `5e63f63` (`Update README.md`). `git diff --ignore-cr-at-eol --quiet` reports no content diff against HEAD; the apparent 170 modified tracked files are line-ending-only (CRLF in the extracted working tree versus LF in the index). Do not interpret those 170 status entries as 170 logical source changes.
- The archive includes `.git/`. Before sharing the archive with a buyer or publishing it, scan the complete Git history for accidentally committed secrets and decide whether history should be included.
- `webpanel/src` is **not incomplete** in this ZIP: the API clients, Zustand auth store, and API types are present. The earlier statement that these paths were absent is superseded.
- `docs/` **is present**. It contains useful operator/setup/migration/retention/API documentation, but some documents are inconsistent or contain merge-conflict markers (see section N).
- `webpanel/Dockerfile` exists but is **zero bytes**. There is also a nested `webpanel/webpanel/` directory containing a second, mostly Vite-template frontend scaffold; its intended use is unclear.
- Two `.env.example` files exist (`.env.example` and `infra/.env.example`) and are inconsistent. The infra example omits `OPERATOR_GROUP_CHAT_ID`, although Compose references it.

**Checks actually attempted in this review:**
- Container environment: Go `1.23.2`, Node `v22.16.0`, npm `10.9.0`; Docker is unavailable.
- Backend `go test ./...` was not runnable: `backend/go.mod` requires Go `1.26.0`.
- Bot `go test ./...` was not runnable: `bot/go.mod` requires Go `1.25.1`.
- `gofmt -l` reports the extracted Go files because the ZIP working-tree files use CRLF line endings. A `gofmt -d` sample showed line-ending normalization; this is not evidence of Go syntax errors. Normalize line endings or check a clean Git checkout before treating it as a formatting defect.
- `npm ci` was attempted but timed out before dependencies were installed. Therefore frontend lint/build were **not** run.
- `docker compose config`, Docker builds, migrations against PostgreSQL, and end-to-end smoke tests were **not** run because Docker is unavailable.
- Compose and workflow YAML files parse as YAML with a local parser; this is not a substitute for `docker compose config`.

Labels: **[CONFIRMED]** = verified by reading the supplied code/config. **[PROBABLE]** = plausible but requires runtime/reproduction checks. **[UNVERIFIED]** = evidence unavailable. All source findings are static unless explicitly identified as an executed check; nothing here is "production ready".

---

## A. Executive summary

Anonymus Bot is an MVP composed of a Telegram bot (long polling, bounded concurrent dispatcher) → Gin backend → PostgreSQL + Redis → React/Mantine operator panel. The repository is more complete than the earlier flattened audit suggested: frontend source and project documentation are present, and there are Go CI workflows plus repository/retention integration tests.

It is **not safe or deployment-ready in its current state**. The most important confirmed blockers are:

1. **Critical: unauthenticated backend `/internal/*` routes are exposed through the published backend port `8080`.** This allows caller-controlled Telegram IDs and access to privileged bot/backend workflows.
2. **Critical: invitation payloads are guessable (`<sequential session id>-c|e`), and bind logic does not enforce that the caller actually acquired the requested participant slot before returning pending messages.**
3. **Critical: `NewServer` receives but fails to assign `botClient`, `photosDir`, and `voiceDir`.** Several flows dereference a nil `botClient`; media serving uses empty directories.
4. **High: the `delivered` flag is not persisted/read consistently.** The INSERT omits the column while the migration default is `true`; the message history query also omits `delivered`, so the UI receives the zero value (`false`) for all messages.
5. **High: pending messages are marked delivered before the bot has successfully sent them to Telegram.** A Telegram send failure can permanently lose a message.
6. **High: Nginx strips `/api/` from proxied paths, while Gin routes include `/api/`; the production Compose stack also does not build or serve the panel.**
7. **High: login/TOTP have no throttling; TOTP secrets are stored in plaintext and enrollment is persisted before confirmation.**
8. **High: retention can anonymize a user who is still participating in another active session; incoming requests retain plaintext identifiers and first-message content.**
9. **High: Telegram file downloads use `http.Get` without a timeout or size limit; wrapped URL errors can expose the bot token in logs.**
10. **Deployment and handover documentation are not reliable:** `docs/README.md` contains merge-conflict markers and overstates readiness; `docs/API.md` incorrectly claims the internal API is protected by the production network boundary.

The earlier `45–50%` sellable-product completion estimate is **not a measured metric** and should not be used as a valuation or delivery commitment. The previous `~325 h` roadmap is also a rough planning estimate, not an engineering estimate validated by implementation.

---

## B. Architecture (as implemented)

```mermaid
flowchart LR
  TGU[Telegram users] <--> TG[Telegram Bot API]
  subgraph BOT["bot container (Go)"]
    POLL[GetUpdatesChan long polling] --> DSP[dispatcher\nsemaphore=10]
    DSP --> H[handlers: start, relay, photo, voice,\nsessions, stop, language, operator, alerts]
    H --> BC[backendclient HTTP]
    ISRV[internal server :9090\n/internal/send-message\n/internal/send-alert\n NO AUTH]
    H -- photo/voice files --> VOL[(shared volumes\nbot_photos, bot_voice)]
  end
  subgraph BE["backend container (Gin :8080)"]
    INT["/internal/* NO AUTH"]
    API["/api/* (cookie-less, Bearer session token)"]
    SW["/swagger/*"]
    MOD[moderation: regex + phrases\ncounters in Redis]
    AUTHS[auth: bcrypt, TOTP, sessions in Redis]
    REPO[repositories pgx]
    BOTC[botclient -> bot:9090]
  end
  RET[retention container\nloop + /retention binary 03:00]
  PG[(PostgreSQL 15)]
  RD[(Redis 7, no password)]
  NGX[nginx :80\n/api/ -> backend (strips prefix!)\n/ -> static text]
  PANEL[React panel\nNOT served by compose]
  BC --> INT
  BOTC --> ISRV
  ISRV --> TG
  H --> TG
  INT & API --> REPO --> PG
  MOD & AUTHS --> RD
  BE --- VOL
  RET --> PG
  RET --> VOL
  PANEL -. dev: vite proxy /api .-> API
  NGX --> API
```

Flows (verified):
- **Join:** `/start <sessionID>-c|e` → bot parses payload → `POST /internal/sessions/{id}/bind` → backend creates/gets user (username/first_name AES-GCM encrypted), sets `users.active_session_id`, fills `client_user_id`/`executor_user_id` if empty, returns undelivered messages.
- **Relay:** bot `POST /internal/messages/relay {sender_telegram_id, content_type, content|file_id}` → backend resolves active session/role/recipient, moderation (text only), stores message, returns recipient Telegram ID + label → **bot itself** sends the plain-text copy (`Клиент:` / `Менеджер:`) to the recipient with a "reply here" button. No outbox/retry.
- **Media:** bot downloads photo (largest size) / voice to shared volume under a UUID, ImageMagick `convert -strip` for photos, relays `file_id`; after successful delivery the bot **deletes the file**.
- **Operator panel:** login (bcrypt) → `pending_token` (Redis, 5 min) → TOTP → `session_token` (UUID, Redis, 24 h, Bearer header) → `/api/*`. Roles: admin sees all sessions; operator only `owner_operator_id`.
- **Alerts:** backend → bot `:9090` → operator group chat (incoming request, moderation hit, stop request).
- **Retention:** container loops hourly; at `RETENTION_RUN_HOUR` runs `/retention`: anonymize users of sessions closed >3 months (PII encrypted fields → NULL, telegram_id → negative), hash PII keys in `audit_log.payload`, delete messages+media of sessions closed >12 months.

---

## C. Repository map

| Path | Responsibility |
|---|---|
| `backend/cmd/api/main.go` | Wiring: Postgres, Redis, encryptor, repos, Gin |
| `backend/cmd/retention/{main.go,entrypoint.sh}` | Retention job + hourly loop |
| `backend/internal/api` | Gin handlers: auth, sessions, messages (relay/send-as), operators, incoming requests, users, media, audit, health, middleware, router |
| `backend/internal/auth` | bcrypt, TOTP (pquerna/otp), Redis sessions/pending auth |
| `backend/internal/moderation` | regex contact filter, phrase heuristics, Redis violation counter |
| `backend/internal/repository` | SQL (pgx) + integration tests (testcontainers) |
| `backend/internal/retention` | PII anonymizer, message cleaner, audit hasher |
| `backend/internal/crypto/aes.go` | AES-256-GCM for user PII |
| `backend/internal/botclient` | HTTP client → bot :9090 |
| `backend/docs/swagger` | generated (6 of ~30 endpoints) |
| `bot/cmd/bot`, `bot/internal/*` | Telegram handlers, dispatcher, backend client, internal server, ImageMagick strip |
| `migrations/000001–000003` | schema (users, operators, sessions, messages, incoming_requests, audit_log; `delivered`; `closed_at`) |
| `infra/{docker-compose*.yml,nginx/nginx.conf}` | Deployment |
| `webpanel/src/*` | React pages (Mantine, TanStack Query, Zustand) |
| `loadtest/*` | k6 + seed SQL |
| `.github/workflows/{backend,bot}-ci.yml` | CI (Go only) |

---

## D. Feature status

### Telegram bot
| Feature | Status | Evidence |
|---|---|---|
| Registration | PARTIAL | User created only on bind; plain `/start` creates an `incoming_request` (every time, no dedupe, `first_message_text` never sent) |
| Customer/contractor roles | PARTIAL | Two fixed roles via deep link; same user can occupy both |
| Invitation links | BROKEN (security) | `<sequentialID>-c/e`, no secret, no expiry (S-02) |
| Session creation/joining | PARTIAL | Creation via panel only; join races (S-03) |
| Bidirectional messaging | PARTIAL | Works if wiring OK; no outbox; order not guaranteed |
| Multiple sessions / switching | COMPLETE (static) | `/sessions`, `SwitchActiveSessionTx` validates participation |
| Session closure | PARTIAL | `/stop` → `pending_close` + alert; backend alert panics (B-01); no notification to participants on close |
| Text | COMPLETE (static) | |
| Photos | BROKEN | Metadata stripped OK; file deleted after delivery so panel preview fails; captions dropped (B-05) |
| Voice | PARTIAL | Same deletion issue; no validation; no size/timeouts |
| Contact detection | PARTIAL | Text only; false positives (≥6 digits); no Latin phrases (S-09) |
| Error handling | PARTIAL | Generic messages; recipient send failures silently lost |
| Duplicate update handling | n/a / OK | Offset-based polling; no replay risk observed |
| Localization | MISSING | Almost all texts Russian; saved `/language` is never used (only help text uses Telegram `LanguageCode`) |

### Backend
| Feature | Status |
|---|---|
| Authentication (password + TOTP) | PARTIAL — no throttling (S-04), TOTP secret plaintext & stored before confirmation (S-06), no password change/reset |
| Authorization | PARTIAL — session-level checks good; audit log open to all operators (S-07); sessions not revoked on suspend/role change (S-05) |
| 2FA | PARTIAL — no recovery/reset |
| Administrator bootstrap | MISSING |
| Operator management | PARTIAL — create/status/role; no password reset; `telegram_id` cannot be set through API so the bot `/operator` mode is unusable without SQL |
| Session management | PARTIAL — list/get/create/status/owner |
| Message management | BROKEN — `delivered` not persisted (B-02) |
| API validation | PARTIAL — Gin binding; date filters unchecked (500) |
| Audit logging | PARTIAL — 6 actions; actor id mixes operator and Telegram-user ids; no role/IP/UA |
| Rate limiting | MISSING |
| Retention | PARTIAL/BROKEN — S-08 |
| Error handling | PARTIAL |

### Web panel (full source is present in the ZIP; build not run)
| Feature | Status |
|---|---|
| Login + TOTP setup/verify with QR | IMPLEMENTED (static) |
| Dashboard | MISSING |
| Session list / create | PARTIAL — list and create exist; no search/filter/pagination |
| Session details / history | PARTIAL — status update, invitation links, history polling every 5 s |
| Invitation links | BROKEN (security) — hardcoded `test_fastcheck_bot`; predictable session ID/role payload |
| Media previews | UI IMPLEMENTED, runtime BROKEN — React image/audio preview components exist, but backend directory wiring is nil and bot deletes delivered files |
| Operator reply (send-as) | MISSING in UI — backend endpoint exists |
| Operator management | PARTIAL — create with role selection and status changes; no role-edit/password-reset UI |
| Password management | MISSING |
| Audit log + CSV | PARTIAL — action filter and CSV export exist; all authenticated operators can access it; no pagination |
| Incoming requests | PARTIAL — list and create-session action exist; no durable request-to-session relation |
| Loading/error states | PARTIAL — several pages have basic states; review consistency |
| Responsive | UNVERIFIED — source inspection only; no browser test |
| RU/EN localization | MISSING — user-facing strings are hardcoded Russian; no i18n framework |
| Auth token storage | IN-MEMORY — Zustand store has no persistence middleware/localStorage; active-page XSS could still use the token |
| Branding | BROKEN — hardcoded `BOT_USERNAME='test_fastcheck_bot'` and product title |

### Infrastructure
| Item | Status |
|---|---|
| Docker (backend, bot, retention) | PARTIAL — containers run as root; bot/retention lack healthchecks |
| Web panel container | MISSING — `webpanel/Dockerfile` is empty (0 bytes); Compose has no panel service |
| PostgreSQL | PARTIAL — app connects with the role created by `POSTGRES_USER`, which is the PostgreSQL image's initial superuser |
| Redis | PARTIAL — no password, no persistence volume/config; operator sessions and moderation counters are volatile |
| Nginx | BROKEN — `/api/` prefix is stripped; root serves a plain text response |
| Health checks | PARTIAL — backend `/health`; bot/retention healthchecks absent |
| Env vars | PARTIAL — two inconsistent example files; infra example omits `OPERATOR_GROUP_CHAT_ID`; Compose hardcodes `RETENTION_RUN_HOUR` |
| Migrations | MANUAL ONLY — migration files and manual Docker instructions exist, but Compose/backend startup does not apply migrations |
| First admin | MANUAL SQL bootstrap documented in `docs/OPERATOR_SETUP.md`; no automated bootstrap/CLI |
| CI/CD | PARTIAL — backend and bot Go build/vet/lint/race-test workflows exist; no frontend, Docker, Compose, or migration pipeline |
| Backup/recovery | MISSING — no verified backup/restore procedure or restore drill |

---

---

## E. Confirmed bugs

| ID | Sev | Location | Description |
|---|---|---|---|
| B-01 | Critical | `backend/internal/api/router.go: NewServer` | Returns `&Server{…}` **without** `botClient`, `photosDir`, `voiceDir` (parameters ignored). `s.botClient` is a nil pointer: `sendModerationAlert`, `sendIncomingRequestAlert`, `sendStopRequestAlert`, `handleSendMessageAsOperator` (`SendMessage`) dereference it → panic → Gin Recovery 500. Consequences: every moderation hit, every plain `/start`, every `/stop`, every send-as returns 500 (send-as even stores the message first). `handleGetMessageMedia` builds paths from empty dir → media never found. *Verified by reading + grep.* |
| B-02 | High | `backend/internal/repository/message_repository.go: Create / ListBySessionID`; migration `000002_add_delivered_to_messages.up.sql` | `Create` omits `delivered`; migration default is `true`. `relayMessage` sets `Delivered=false` when the counterparty is not yet bound, but that value is discarded, so `ListUndelivered` cannot find deferred messages. Separately, `ListBySessionID` omits `delivered` from SELECT/Scan, leaving the Go bool at `false`; the API therefore reports every history message as not delivered. This is two related but distinct defects. |
| B-03 | High | `bot/internal/handlers/{photo,voice,start}.go` | Files are removed right after delivery to the recipient, while the backend/panel serve media from the same volume → history previews fail ("media file no longer available"). Retention `FilesDeleted` will be ~0. |
| B-04 | High | `infra/nginx/nginx.conf` | `location /api/ { proxy_pass http://backend:8080/; }` strips `/api/` but backend routes are `/api/...` → every API call 404s through nginx. `location /` returns a text line; the panel is not served; no `client_max_body_size`; no TLS/headers. |
| B-05 | Medium | `bot/internal/handlers/photo.go` | Photo captions are discarded (never relayed/moderated). |
| B-06 | Medium | `infra/docker-compose.yml`, `docs/MIGRATIONS.MD`, `docs/OPERATOR_SETUP.md` | No automatic migration runner in Compose or backend startup; a clean deployment requires the documented manual `migrate/migrate` Docker command. There is no automated first-admin seed/CLI, but a manual SQL bootstrap is explicitly documented in `docs/OPERATOR_SETUP.md`. This is an operational first-run gap, not an absence of any documented procedure. |
| B-07 | Medium | `backend/internal/api/operators.go: handleUpdateOperator` | Sessions revoked only for `disabled`; `suspended` or role demotion leave existing 24 h tokens fully active (role cached in Redis `SessionData`). `requireAuth` never re-checks the DB. |
| B-08 | Medium | `moderation/service.go`, `api/messages.go` | Operator send-as uses `operatorID` as the moderation key → shares counter namespace with user IDs. Redis failure ⇒ moderation **fails open** (contact info passes). Counter not reset when session resumed → next violation re-pauses; one party's violations pause the whole session. |
| B-09 | Medium | `moderation/regex_filter.go` | `long_digits \d{6,12}` and broad phone pattern block legitimate business text (amounts ≥100000, invoice numbers, "1 000 000 000") and three hits auto-pause the **whole session**. |
| B-10 | Medium | `retention/message_cleaner` (`messages.go`) | On media-file removal error the DB row is deleted anyway → orphan file with personal photo stays forever. `deleteMediaFile` joins `FileID` into a path without validating it (possible `../` via the unauthenticated relay API; limited to `.jpg/.ogg`). |
| B-11 | Medium | `api/auth.go: handleVerifyTOTP` | `last_login_at` is never updated (panel always shows «Никогда»). |
| B-12 | Medium | `api/sessions.go: handleBindSession` | `SetClientUser/SetExecutorUser` are unconditional UPDATEs (no `WHERE … IS NULL`) → concurrent joins overwrite each other. |
| B-13 | Low | `audit` | `session_stop_requested` writes the **Telegram user** id into `actor_id`, which elsewhere holds operator ids; `actor_role/ip/user_agent` never populated; `AuditLogHasher` filters only by `actor_id`, not `target_id` as its comment claims; unsalted SHA-256 of low-entropy values is reversible. |
| B-14 | Low | `api/audit.go` | `date_from/date_to` invalid → DB error → 500; no pagination/limit (entire log returned). |
| B-15 | Low | `bot/internal/handlers/alert_callbacks.go` | "Открыть в панели" is an `answerCallback` toast with a text link, not a URL button; `PANEL_BASE_URL` is `http://localhost` in compose. |
| B-16 | Low | `bot/cmd/bot/main.go` | No signal handling / graceful shutdown; `context.Background()` never cancelled. |
| B-17 | Low | `api/messages.go` | Messages from the bot and from the panel have no per-user ordering guarantee: the dispatcher processes updates of one user concurrently (up to 10). |
| B-18 | Low | `api/health.go` | Returns raw DB/Redis error strings. |
| B-19 | Low | `webpanel/LoginPage.tsx` | `catch (err)` with unused `err` (×2) — likely `@typescript-eslint/no-unused-vars` failure with the configured v8 rules **[PROBABLE]**. |
| B-20 | Low | `retention/pii.go` | Retention run is not idempotent for audit (writes a new `pii_anonymized` entry without actor), fine, but `HashPIIForUser` re-lists all audit rows of that actor for every user (O(n²)). |


| B-21 | High | `backend/internal/api/sessions.go: collectUndeliveredMessagesFor`; `bot/internal/handlers/start.go`; `bot/internal/handlers/message_relay.go`; photo/voice handlers | Pending messages are marked delivered in the database before Telegram send is attempted. If `bot.Send` fails, the bot only logs the error; there is no retry or state rollback. Immediate text/photo/voice delivery also has no durable outbox/retry. A database row can therefore say delivered even when Telegram never accepted the message. |
| B-22 | Medium | `infra/docker-compose.yml`; `backend/internal/auth/session.go`; `backend/internal/moderation/service.go` | Redis has no password, persistence volume, or AOF/RDB configuration. A Redis restart loses all operator sessions and moderation counters. This is both an availability issue and a moderation-policy consistency issue. |
| B-23 | Medium | `docs/README.md`; `docs/API.md`; `docs/RETENTION.md`; `infra/.env.example` | Documentation contains merge-conflict markers (`<<<<<<< HEAD`), claims the MVP is fully implemented, references a missing `docs/SECURITY_CHECKLIST.md`, and says `/internal/*` is network-protected although port 8080 is published. It also says media previews are placeholders although UI preview components exist, and says retention periods are hardcoded although the code reads the month values from env. The infra env example omits `OPERATOR_GROUP_CHAT_ID`, and Compose hardcodes the retention run hour. |


---

## F. Security findings

| ID | Sev | Status | Details |
|---|---|---|---|
| S-01 | **Critical** | CONFIRMED | **Unauthenticated `/internal/*` API exposed.** `router.go` registers `/internal/messages/relay`, `/internal/sessions/:id/bind`, `/internal/users/by-telegram/:id`, `/internal/sessions/stop`, `/internal/incoming-requests`, `/internal/operators/by-telegram/:id` etc. with no auth; compose publishes `8080:8080`. Impact: spoof a known Telegram ID for relay/bind/stop operations, read new incoming-request first names and message text, and probe user/operator records and operator-owned sessions by supplied Telegram ID. The route does not provide a general all-operators listing; avoid describing this as unrestricted enumeration. `file_id` path abuse is a separate chained risk (B-10). Fix: shared secret header (`INTERNAL_API_TOKEN`, constant-time compare) on `/internal` group, do not publish 8080 (only nginx), separate listener/network, disable swagger in prod. Same for bot `:9090` (`send-message` can message any chat). Tests: requests without/with wrong token → 401. |
| S-02 | **Critical** | CONFIRMED | **Guessable invites and message theft.** Payload `<id>-<c|e>` with sequential `BIGSERIAL` IDs; no secret/expiry/single-use. (a) `/start N-e` takes an empty executor slot. (b) `handleBindSession` always sets `active_session_id` and calls `collectUndeliveredMessagesFor` **even when the slot is occupied/the user is not a participant**, returning and **marking delivered** another party's pending messages. (c) closed/paused sessions are joinable; a user can hold both roles. Fix: random 128-bit invite tokens (new table `session_invites`: hash, role, session, expires_at, used_at), atomic bind (`UPDATE … WHERE client_user_id IS NULL RETURNING`), deliver pending only after successful bind, reject non-active sessions, forbid dual role. Tests: bind-isolation, concurrent bind, replay. |
| S-03 | High | CONFIRMED | **Bot token in logs.** `photo.go/voice.go downloadTelegramFile` wraps `http.Get(url)` errors (`*url.Error` contains `https://api.telegram.org/file/bot<TOKEN>/…`) and logs them. Also `http.Get` has no timeout (hangs consume the 10-slot dispatcher) and `io.Copy` has no size limit. Fix: dedicated `http.Client{Timeout}`, `io.LimitReader`, redact URL (`errors.Unwrap`/`urlErr.Err`). |
| S-04 | High | CONFIRMED | **No rate limiting / lockout** on login, TOTP (pending token valid 5 min, failures not counted → ≈ up to hundreds of thousands of guesses; with a 3-code window a meaningful success probability), bot, nginx (`limit_req` absent). Password has no policy; account enumeration by status (403 before password check) and timing (no dummy bcrypt). Fix: Redis counters per login+IP, max 5 TOTP attempts per pending token, exponential lockout, nginx `limit_req`, constant-time path. |
| S-05 | High | CONFIRMED | **Stale privileges.** Suspended or demoted operators keep 24 h sessions (B-07); no server-side status/role check per request. |
| S-06 | Medium | CONFIRMED | **TOTP secrets** stored in plaintext in `operators.totp_secret`; secret is generated and persisted on the first password login (before confirmation) and shown only once, so a user who doesn't save it is locked out; no reset/recovery; no password-change endpoint at all (temporary 16-char password is permanent). |
| S-07 | Medium | CONFIRMED | **Audit log readable by every operator** (`authenticated.GET("/audit-log")` is outside `adminOnly`), and the panel nav exposes it. |
| S-08 | High | CONFIRMED | **Retention harms active data and leaves PII.** `PIIAnonymizer` anonymizes every participant of an old closed session even if the user is in another open session (breaks routing: `telegram_id` becomes negative). `incoming_requests` (telegram_id, username, first_name, first_message_text) are never purged and stored **plaintext**; alerts copy first message text to a group chat. Message cleanup issue B-10. |
| S-09 | Medium | CONFIRMED | **Contact detection gaps:** text only (no captions, no voice); Russian-only phrase list; no obfuscation handling ("t g", "собака", spaced digits); fail-open on Redis errors; false positives B-09. |
| S-10 | Medium | CONFIRMED | **Redis without password**, no persistence; `docker-compose.dev.yml` publishes 5432 and 6379 to the host (session tokens stored in plaintext keys). Session tokens themselves are stored unhashed. |
| S-11 | Low | CONFIRMED | Gin runs in debug mode (no `GIN_MODE=release`), `gin.Default()` access logs include Telegram IDs in paths; `/swagger` public; Dockerfiles run as root; ImageMagick present (policy hardening absent); no `http.Server` timeouts; no body size limits. |
| S-12 | Low | CONFIRMED | PII encryption key has no rotation/versioning: a wrong/rotated key makes `GetByTelegramID` fail for every user (bot outage). |
| S-13 | Low | CONFIRMED (limited scope) | The supplied `webpanel/src/stores/authStore.ts` keeps the bearer token in ordinary Zustand in-memory state; no persistence middleware or `localStorage`/`sessionStorage` use was found. This corrects the earlier localStorage hypothesis. An XSS in the active panel could still read/use the in-memory token or make authenticated requests; a full browser/XSS review was not performed. |
| S-14 | Info | — | **Privacy boundary** to document: operators see all messages and can write as either party (`send-as`); platform hides identities between participants only. No claim of full anonymity or GDPR compliance is justified. |

**Checked OK (static):** SQL is parameterized everywhere (no injection found); messages are sent without `ParseMode` (no Telegram HTML injection); forwarded/contact/location/sticker content is not relayed; photos are re-encoded/stripped; user PII encrypted with AES-256-GCM (random nonce); bcrypt cost 12; session switching validated in a locked transaction; operator authorization by `owner_operator_id` (`canAccessSession`) applied to messages/media/status/owner/send-as.


| S-15 | Medium | CONFIRMED | The bot's internal HTTP server (`bot/internal/server/server.go`, `:9090`) exposes `/internal/send-message` and `/internal/send-alert` without authentication. It is not published as a host port in the supplied Compose file, so it is **not equivalent to the host-exposed backend `/internal/*` API**; however, any workload/container able to reach the Compose network can call it. Add service authentication and keep the network boundary explicit. |

---

## G. Missing functionality
Automated first-admin bootstrap and migration step (manual procedures are documented); password change/reset and TOTP reset/recovery; panel i18n (RU/EN) and full bot i18n; operator send-as composer; session search/filters/pagination; dashboard; secure invitations; durable outbox/retry for bot delivery; media retention semantics; runtime branding config (bot username, product name); Telegram-operator linking (set `operators.telegram_id`); incoming-request→session linkage; buyer-ready documentation (current docs exist but are inconsistent and contain merge-conflict markers); broader OpenAPI coverage; backups/restore; rate limiting.

---

## H. Code quality
- **Duplication:** handleBlockedReason ×3 (message/photo/voice), `downloadTelegramFile` ×2, `answerCallback` ×4, `sendMessage` ×7, session-ID parsing/`canAccessSession` boilerplate in every handler, `scan` blocks in repositories (6 message SELECT lists).
- **Dead/unused or incomplete:** `DeleteBySessionID`, `Message.InternalFilePath`, operator `language` (stored but not applied to UI), `last_login_at` (displayed but never updated), several session display/payment/close-reason fields, `REDIS_URL` in bot config, `logoutRequest` struct, `OPERATOR_GROUP_CHAT_ID` passed to backend. Incoming-request fields are used by the API/panel, but the bot's plain `/start` handler does not populate `first_message_text`.
- **Large functions:** `relayMessage` (~120 lines, business logic in transport layer), `handleUpdate` closure in bot `main.go`.
- **No service layer** in backend: handlers talk directly to repositories.
- **Hardcoded config:** 3 violations/24 h, 5-min pending token, 24 h operator session, 10 goroutines, bot username, panel title, Russian texts. Retention month periods are read from env; `RETENTION_RUN_HOUR` is hardcoded to `03` in Compose.
- **Race risks:** B-12, B-17; `ListUndelivered`+`MarkDelivered` non-transactional.
- **Hygiene:** coverage artifacts committed; `testuril_test.go` duplicates `testutil_test.go`; root-level README is absent; `webpanel/README.md` is the Vite template; `docs/README.md` has merge-conflict markers; `.env.example` variants inconsistent; Swagger annotations/docs are in Russian; `webpanel/Dockerfile` is empty; nested `webpanel/webpanel/` is a duplicate scaffold; `docker-compose.dev.yml` uses obsolete `version`.
- **Format/lint:** `gofmt`/`go vet`/`golangci-lint` status **UNVERIFIED** (toolchain unavailable).

---

## I. English comments migration

A path-preserving scan of the supplied ZIP found Russian text in **211 comment-only lines across 38 source/config files** (approximate; it intentionally excludes ordinary user-facing string literals and may miss inline trailing comments). The largest counts were:

`backend/internal/api/sessions.go` 19 · `backend/internal/repository/user_repository.go` 15 · `backend/internal/retention/audit_hash.go` 14 · `backend/internal/api/messages.go` 13 · `backend/internal/auth/session.go` 11 · `backend/internal/api/audit.go` 9 · `backend/internal/api/auth.go` 8 · `backend/internal/retention/config.go` 8 · `backend/internal/retention/pii.go` 8 · `backend/internal/api/operators.go` 7 · `backend/internal/moderation/regex_filter.go` 7 · `bot/internal/fileutil/strip.go` 7 · `bot/internal/handlers/photo.go` 7 · `backend/internal/retention/messages.go` 6 · `bot/internal/dispatcher/dispatcher.go` 6 · `backend/cmd/api/main.go` 5 · `backend/internal/api/middleware.go` 5 · `backend/internal/moderation/service.go` 5 · `bot/internal/handlers/message_relay.go` 5.

The earlier flat-folder overwrite caveat no longer applies to this ZIP because paths are preserved. Still perform a final source-aware scan during implementation, especially for inline comments and Swagger annotations. **Do not translate user-facing Russian strings as part of comment cleanup**; move UI/bot text into localization catalogs in a separate i18n task.

---

## J. Testing gaps

Existing test source (not executed in this review):
- Unit tests: deeplink parsing, blocked-type handling, regex filter, heuristics, password hashing, TOTP, and role/recipient mapping.
- Integration tests using testcontainers: user/session/message/operator/audit/incoming-request repositories, session switching, PII anonymizer, and message cleaner.
- CI workflows exist for backend and bot. Each runs Go build, vet, golangci-lint, and `go test ./... -race -cover` using Go 1.26. There is no frontend or Docker/migration CI job.

**Committed coverage artifacts (not freshly generated):** the supplied `backend/coverage` artifact indicates approximately `0.6%` statement coverage for `internal/api`, `37.5%` for `internal/moderation`, and `15.3%` for `internal/auth`. The supplied `bot/coverage` indicates approximately `6.3%` for `internal/handlers` and `0%` for several other listed packages. Treat these as snapshots only; timestamps/provenance were not independently established. This corrects revision 2's blanket statement that the entire API and moderation service had 0% coverage.

**Execution status in this review:**
- Backend tests blocked by local Go 1.23.2; module requires Go 1.26.0.
- Bot tests blocked by local Go 1.23.2; module requires Go 1.25.1.
- `npm ci` timed out; frontend lint/build not run.
- Docker unavailable; Testcontainers, Compose config, image builds, nginx test, migrations, and end-to-end checks not run.
- `gofmt -l` flags CRLF files from the extracted ZIP. Git confirms the working-tree/index difference disappears when CRLF is ignored; do not interpret this alone as source-formatting drift.

Missing tests (priority):
1. API tests (httptest + Postgres + Redis/miniredis): login/TOTP/pending token, throttling, authorization matrix (admin/operator/other-operator), audit-log access, `NewServer` wiring smoke test, bind security, relay with moderation, send-as, media path, stop flow.
2. Repository: `delivered` persistence and readback; concurrent bind; claim/mark semantics.
3. Delivery integration: Telegram send failure, retry/idempotency, no message loss/duplicates, outbox state transitions.
4. Retention: user with another open session, incoming_requests retention, file-removal failure, path validation.
5. Bot handlers with a fake Telegram API + fake backend: photo/voice, blocked reasons, per-user ordering, token redaction, delivery failures.
6. Moderation corpus (false positives/negatives, RU/EN, obfuscation, business numbers, captions/voice policy).
7. Frontend: unit tests (RTL) + Playwright e2e login→session→messages/media; verify responsive layout and role-gated navigation.
8. Infra: Compose smoke (`up`, migrate, health, `/api` through nginx), `nginx -t`, migration up/down, backup/restore drill.
9. CI must run backend, bot, frontend, image build, compose validation, and migration tests.

---

## K. Commercial readiness

**Not ready for a commercial handover or production deployment.** Buyer blockers:
- Critical internal API exposure and guessable invitation/bind flow (S-01/S-02).
- `NewServer` nil dependencies and broken message-delivery semantics (B-01/B-02/B-21).
- Nginx route mismatch, panel not deployed, empty panel Dockerfile (B-04).
- No automated migrations/first-admin path (manual instructions exist, but they are not a repeatable deployment pipeline).
- No password change/reset or TOTP recovery; no brute-force throttling.
- Retention can break active users and does not clean incoming-request PII.
- No durable delivery outbox/retry.
- Documentation is present but inconsistent, contains merge-conflict markers, and lacks security/privacy/handover/operations documents.
- Frontend source exists, but build/lint/e2e are not verified; panel lacks operator send-as UI, search/pagination, i18n, and runtime branding config.
- No verified backup/restore process; Redis is volatile and unauthenticated.
- No evidence of production operation, revenue, customers, or repeatable installation.

Strengths to preserve:
- Clear bot/backend/panel separation.
- Parameterized SQL in reviewed repository queries.
- bcrypt password hashing (cost 12 in the reviewed implementation).
- AES-GCM encryption for selected user PII fields.
- Session ownership checks in panel API handlers.
- Transactional active-session switching.
- Testcontainers-based repository/retention test scaffolding.
- Backend and bot Go CI workflows.
- Retention dry-run support (must be used before enabling production retention).

Do not market the product as fully anonymous: participants are hidden from one another, but operators/admins can read messages and send messages as either side. Do not claim GDPR compliance or production security without a separate legal/privacy/security assessment.

---

## L. Implementation roadmap

### P0 — Critical security and data integrity (≈ 50 h)

| ID | Task | Files / functions | Acceptance & tests | h |
|---|---|---|---|---|
| P0-1 | **Authenticate internal APIs and stop exposing them** | `router.go` (`/internal` group middleware), `botclient/client.go`, `bot/internal/backendclient/client.go`, `bot/internal/server/server.go`, `docker-compose.yml` (remove `8080:8080`), swagger gated by env | Missing/invalid `X-Internal-Token` → 401 on backend and bot; port 8080 unreachable from host; tests for both servers | 6 |
| P0-2 | **Secure invitations and atomic bind** (S-02, B-12) | new migration `000004_session_invites`, `sessions.go handleBindSession`, `session_repository.go` (conditional updates in a tx), `bot/handlers/deeplink.go`, panel deeplink generation, new `POST /api/sessions/{id}/invites` | Token single-use, expiring; non-participant never receives messages; concurrent bind test; closed/paused rejected; no dual role | 14 |
| P0-3 | **Fix `NewServer` wiring** (B-01) + startup validation of required env | `router.go`, `main.go`, `config` | Constructor test asserting non-nil deps; alert/send-as/stop endpoints return 200 against fake bot | 2 |
| P0-4 | **Brute-force protection** (S-04) | `auth.go`, `auth/throttle.go` (Redis), Nginx `limit_req`; password policy moved to P1-5 under [D-006](docs/DECISIONS.md#d-006--limit-operator-authentication-attempts) | At most 5 TOTP checks per pending token; login throttling with retry guidance; dummy bcrypt path; tests | 8 |
| P0-5 | **Session/privilege freshness** (S-05, B-07) | `middleware.go requireAuth` (cache status+role ≤30 s or revoke on change), `operators.go` revoke on suspend/role change | Suspended operator gets 401 immediately; demoted admin loses admin access | 5 |
| P0-6 | **Audit access and integrity** (S-07, B-13, B-14) | `router.go` (adminOnly), `audit.go`, migration `000005` (`actor_type`, indexes), `audit_log_repository.go` (limit/offset, validate dates → 400) | Operator gets 403; actor type correct; pagination tested | 4 |
| P0-7 | **Stop token leak and unbounded downloads** (S-03) | `bot/handlers/photo.go`, `voice.go` → shared `downloader` with timeout, size cap, redacted errors | Test injecting failing server asserts logs contain no token | 3 |
| P0-8 | **Retention safety** (S-08, B-10) | `retention/pii.go` (skip users with any non-closed session), purge `incoming_requests` by age, `messages.go` (don't delete row on file error; validate `file_id` as UUID), `api/messages.go` validate `file_id` | Integration tests for multi-session user, incoming_requests, failing file removal | 8 |

#### P0 closure evidence — 2026-11-21

**Status: closed for the agreed P0 scope and verified on the local stand.** See the
[development log](docs/DEVELOPMENT_LOG.md) for the isolated checks and the local rollout.

| ID | Evidence reviewed |
|---|---|
| P0-1 | [Backend route tests](backend/internal/api/internal_auth_test.go) cover all 12 routes without credentials and invalid/duplicate credentials on bind. [Bot route tests](bot/internal/server/server_test.go) cover both endpoints and the authenticated send path. Base Compose publishes neither service port; the dev override binds backend only to loopback. The recorded Nginx probe blocked the internal path. The local backend returned 401 without credentials and 404 for Swagger with the default configuration. |
| P0-2 | [Invitation lifecycle test](backend/tests/repository/session_invites_test.go) covers expiry, revoke/reissue, replay, concurrent claims, rejected statuses, dual-role rollback, and access to pending messages after binding. [Bot fixture](bot/internal/handlers/start_invite_test.go) covers the new private-chat flow and rejection of old links. Migration 004 was applied locally and issuance/revocation were confirmed by the project owner. |
| P0-3 | [Constructor test](backend/internal/api/router_test.go) verifies the client and both media directories. The backend image rejected missing startup settings with exit 1. [Fake-bot integration](backend/tests/repository/server_wiring_test.go) observes alert, send-as, and stop calls. Incoming creation returns 201, send-as and stop return 200. |
| P0-4 | [Throttle tests](backend/tests/repository/auth_throttle_test.go) cover atomic login/IP and TOTP limits, 429 with retry guidance, failed Redis access, credential-response parity, and one-time token consumption. Code uses dummy bcrypt for an unknown login. [Trusted proxy test](backend/cmd/api/client_ip_test.go) and the recorded Nginx burst probe verify header isolation and edge 429 responses. Password policy is tracked in P1-5. |
| P0-5 | [Session revocation tests](backend/tests/repository/operator_security_test.go) cover demotion, suspension, reactivation and stale TOTP challenges. The local rollout verified an already open Chrome tab returns to login after suspension on its next request and reactivation does not restore its old token. |
| P0-6 | [Audit API tests](backend/tests/repository/operator_security_test.go) cover operator 403, invalid-date 400, pagination and bounded CSV. [Migration test](backend/tests/repository/p0_migration_test.go) verifies actor backfill and rollback with existing rows. Chrome verified role-gated audit access. |
| P0-7 | [Media download tests](bot/internal/handlers/media_download_test.go) cover the exact size boundary, truncated responses, temporary file cleanup, deadlines, redirects and safe logging. Production limits are 20 seconds across both requests and 20 MiB per file. |
| P0-8 | [Retention integration tests](backend/tests/retention/safety_test.go) preserve a participant in a non-closed session and records whose files cannot be removed, and verify incoming request expiry and dry-run. [Relay integration](backend/tests/repository/server_wiring_test.go) covers invalid media ID rejection. Local migration state is clean at 005; retention dry-run reported zero eligible records and no errors. |

The scheduled retention job remains off on the local stand. Real Telegram
delivery was not repeated after the 005 rollout. Production deployment and the
scheduled run are separate operational checks.

### P1 — Essential functionality and production blockers (≈ 186 h)

| ID | Task | Files | Acceptance / tests | h |
|---|---|---|---|---|
| P1-1 | **Closed in isolated validation.** Persist/read delivery status and use a durable outbox | Migration 000007, backend worker and repository, bot delivery endpoint, message history | Verified delivery after bind, retry, terminal failure and sender notice, lease recovery, and text/photo/voice. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-21--p1-1-durable-message-delivery) | 10 |
| P1-2 | **Closed in isolated validation.** Retain media after delivery until retention | Bot handlers, backend message repository, Compose media mounts | Four send paths retain files. Preview API, browser, shared references, and retention failure/retry verified. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-21--p1-2-media-lifecycle) | 4 |
| P1-3 | **Closed in isolated validation.** Nginx routing and production panel, health checks, security headers, body limit and optional TLS | Nginx, Compose, panel Dockerfile/Vite, backend and bot readiness | Clean Compose health and Chrome TOTP sign-in through Nginx verified. TLS override and body limits tested. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-21--p1-3-nginx-and-production-panel) | 10 |
| P1-4 | **Closed in isolated validation.** Migrations, first admin and environment validation | `docker-compose.yml`, `backend/cmd/admin`, configuration packages, migration 000006 | Clean Compose startup, repeat and concurrent bootstrap checks, dirty-state refusal and first administrator TOTP login. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-21--p1-4-first-startup-and-administrator-bootstrap) | 8 |
| P1-5 | **Closed in isolated validation.** Confirmed and encrypted TOTP enrollment, password policy, self-service change, administrator reset, mandatory temporary-password rotation, and TOTP recovery with one-time codes. | Auth API and repository, migrations 000008–000009, admin bootstrap, panel | Verified password and TOTP flows, session revocation, one-time code consumption and confirmation before persisting the new factor. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-22--p1-5-totp-recovery) | 14 |
| P1-6 | **Closed in isolated validation.** Configurable moderation, captions, obfuscation, RU/EN phrases, per-session counters, reset on resume, and an opt-in pause policy. | `moderation/*`, relay handlers, Compose | 161 labelled cases, precision 94.05%, recall 97.53%. Default and explicit pause policies, caption rejection and Redis failure covered. [Moderation](docs/MODERATION.md) | 12 |
| P1-7 | **Closed in isolated validation.** Runtime branding, send-as, search/filter/paging, owner reassignment, history labels, a panel error boundary, RU/EN localization, dashboard, role editor and a Playwright suite. | Panel, frontend workflow, operator API and repository | Verified assigned session visibility, role revocation, concurrent last-admin protection, and 18 local Chromium passes. The new frontend CI step awaits a PR run. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-22--p1-7-dashboard-and-operator-role-editor) | 30 |
| P1-8 | **Closed in isolated validation.** Ordered processing and shutdown, sanitized logs, deduplicated requests, first private message, HTTPS panel buttons, stored RU/EN for direct replies, and self-service operator linking. | Bot handlers and delivery, operator API and panel, migration 000010 | Fake Telegram, PostgreSQL, Redis and Chrome checks cover language selection, delivery notices, two-step linking and access restrictions. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-22--p1-8-bot-language-and-operator-linking) | 16 |
| P1-9 | **Closed in isolated validation.** HTTP timeouts, 64 KiB body limit, stable error responses and bounded public and internal lists. | API, repositories, clients and OpenAPI | Local tests verified body boundaries, sanitized health, access, page navigation and invalid pagination. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-22--p1-pagination-and-acceptance) | 6 |
| P1-10 | **Closed in isolated validation.** Audit covers verified login, rejected credentials, logout, session creation, incoming request status, media access and bot stop/switch. | `audit.go`, API handlers and repository tests | Fixture tests verify one entry per audited HTTP action and the absence of authentication input from payloads. | 6 |
| P1-11 | **Closed in isolated validation.** Neutral pause/close notices and explicit confirmation in the panel. | API, repository, panel session detail | Both participants received notices through the fake bot. Chrome verified cancel and confirm. Notice delivery is best-effort. | 4 |
| P1-12 | **Accepted.** Test suite (J.1–J.5). | Go tests and coverage gate | All four local coverage areas meet 70%. The project owner reported a green GitHub run. | 36 |
| P1-13 | **Accepted.** Frontend, Docker/Compose, migration round trip, formatting and pinned golangci-lint jobs. | `.github/workflows/*` | Local checks and workflow syntax validation passed. The project owner reported all GitHub checks green. | 6 |
| P1-14 | **Accepted with documented debt.** README, security and privacy boundaries, checklist, handover, operations and regenerated Swagger. | Root documentation, `docs/*`, `backend/docs/swagger/*` | Isolated deployment and first sign-in verified. The project owner deferred an independent documentation-only deployment by a new engineer. [Handover debt](docs/PROJECT_HANDOVER.md#deferred-independent-acceptance) | 18 |
| P1-15 | **Closed in isolated validation.** PostgreSQL and media snapshot and restore. | `deploy/backup/*` | Verified schema version, fixture row and both media files after a two-project restore. Nonempty targets and damaged archives refused. [Instructions](deploy/backup/README.md) | 6 |

### P2 — Important quality improvements (≈ 44 h)

| ID | Task | h |
|---|---|---|
| P2-1 | **Closed in local validation.** Translate current source comments and Swagger annotations to English and correct obsolete descriptions. Original migration text is preserved. | 6 |
| P2-2 | **Closed in local validation.** Drop the unused bot Redis setting, legacy message-path reads and writes, unused deletion helpers and session index. Remove tracked coverage snapshots and rename the retention test helper. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-22--p2-1-and-p2-2-source-and-repository-cleanup) | 3 |
| P2-3 | **Closed in local validation.** Extract participant relay, invitation binding and session status transitions into services, with shared session ID and participant-role helpers. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p2-3-service-layer-and-p2-8-coverage-and-k6) | 8 |
| P2-4 | **Closed in isolated validation.** Non-root application containers, read-only roots, media ownership preparation, authenticated Redis persistence, resource limits and verified HTTP/TLS, healthchecks and loopback dev ports. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p2-4-container-hardening) | 6 |
| P2-5 | **Closed in isolated validation.** Versioned AES-GCM and transactional PII/TOTP rotation, salted HMAC for retained audit PII, and hashed Redis session keys. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p2-5-encryption-rotation-and-credential-storage) | 8 |
| P2-6 | **Closed in isolated validation.** Distinct participant slots, explicit FK actions, supporting indexes and separate database runtime and owner privileges. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p2-6-postgresql-integrity-and-runtime-privileges) | 6 |
| P2-7 | **Closed in local validation.** Backend handles SIGTERM, drains HTTP and settles in-flight delivery. Existing HTTP timeouts and trusted proxy settings were reverified. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-22--p2-7-backend-shutdown) | 4 |
| P2-8 | **Closed in isolated validation.** CI coverage gate includes the service package. Authenticated k6 relay passed with 250 queued messages across 50 synthetic sessions. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p2-3-service-layer-and-p2-8-coverage-and-k6) | 3 |

### P3 — Optional commercial enhancements (≈ 45 h)

| Task | Status | h |
|---|---|---:|
| White-label configuration | **Closed in isolated validation.** Runtime names, localized sign-in and bot greeting texts, panel accent and light/dark/auto scheme. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p3-white-label) | 16 |
| Metrics endpoint and Grafana dashboard | **Closed in isolated validation.** Authenticated Prometheus scrape, HTTP and delivery signals, and a provisioned loopback Grafana dashboard. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p3-monitoring) | 8 |
| Telegram webhook mode | **Closed in isolated validation.** Opt-in TLS route, dedicated secret, replay suppression, graceful drain and switch back to polling. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p3-telegram-webhook) | 6 |
| CSV/PDF transcript export | **Closed in isolated validation.** Role-gated session exports, Cyrillic PDF, spreadsheet-safe CSV and required audit records. [Verification](docs/DEVELOPMENT_LOG.md#2026-11-23--p3-transcript-export) | 8 |
| SSO/OIDC for operators | Planned | 7 |

**Total ≈ 325 h** (≈ 280 h without P3).

### Order
P0-3 → P0-1 → P0-2 → P0-7 → P0-4 → P0-5 → P0-6 → P0-8 → P1-4 → P1-1/P1-2 → P1-3 → P1-12/13 (grow alongside) → P1-5 → P1-6 → P1-8 → P1-7 → P1-9…11 → P1-15 → P1-14 → P2 → P3.

### Do not implement
Multi-tenancy, payments, other messengers, E2E encryption, AI moderation, microservice split, WebSockets before polling is proven insufficient, GDPR-certification claims.

---

## M. Inputs needed to continue

The full ZIP with preserved paths has now been supplied and reviewed. A second upload is not currently required.

Decisions needed before implementation:
1. **Invitation model:** single-use, expiring token per participant role; decide whether invitations can be regenerated/revoked.
2. **Moderation policy:** should repeated violations automatically pause the entire session, or should the system first alert an operator? Define an approved RU/EN test corpus.
3. **Retention policy:** confirm the 3-month PII and 12-month message periods, including treatment of users who remain active in other sessions and incoming requests.
4. **Media policy:** define how long media remains available to operators after Telegram delivery and how failed sends are retried.
5. **Operator impersonation:** confirm which roles may send as either participant, how the UI identifies such messages, and what audit fields are mandatory.
6. **Deployment target:** decide whether the web panel must be served by the same Compose stack (recommended for a turnkey handover) and whether a single backend/bot instance is acceptable.
7. **Git history:** decide whether `.git` history should be included in a buyer handover after a full secret scan.

Required execution environment for the next validation phase:
- Go 1.26 for backend and bot (CI already targets 1.26).
- Node version compatible with the lockfile/Vite toolchain, with package installation available.
- Docker + Docker Compose and a disposable PostgreSQL/Redis environment.
- A test Telegram bot/token only in a secure local environment; never put real secrets in the audit ZIP or documentation.


---

## N. Independent verification addendum — revision 3

This section records direct checks against the supplied ZIP. It supersedes conflicting revision 2 statements where explicitly noted.

### N.1 Confirmed critical implementation defects

| Finding | Direct evidence in supplied source | Assessment |
|---|---|---|
| `NewServer` drops required dependencies | `backend/internal/api/router.go`, `NewServer` (lines 39–70): parameters `botClient`, `photosDir`, `voiceDir` are accepted but omitted from the returned `Server` literal. | **CONFIRMED / Critical.** Alert paths and send-as dereference `s.botClient`; media handler reads `s.photosDir`/`s.voiceDir`. The constructor bug is not speculative. |
| Backend internal API has no auth middleware | `backend/internal/api/router.go`, `RegisterRoutes` (lines 80–98): all `/internal/*` routes are registered directly; `requireAuth` is only attached to `/api/*`. | **CONFIRMED.** |
| Backend port is published to host | `infra/docker-compose.yml`, backend service: `ports: - "8080:8080"`. | **CONFIRMED.** The API is reachable directly on the host interface unless deployment/network policy outside this file blocks it. |
| Predictable invite payload | `bot/internal/handlers/deeplink.go`: parser accepts `<integer>-c` or `<integer>-e`; `webpanel/src/pages/SessionDetailPage.tsx` constructs links from `sessionId` and hardcoded bot username. | **CONFIRMED.** No invite secret, expiry, or one-time token exists in the supplied flow. |
| Bind does not enforce successful slot acquisition | `backend/internal/api/sessions.go`, `handleBindSession`: it calls `SetActiveSession` before checking whether the requested participant slot is empty; it only sets the slot if nil, then calls `collectUndeliveredMessagesFor` regardless. | **CONFIRMED.** A caller who is not a participant can still be given undelivered messages from that session and cause them to be marked delivered. Concurrent binds can overwrite participant slots because repository setters are unconditional updates. |
| Delivered flag is inconsistent | `backend/internal/api/messages.go`, `relayMessage` sets `Delivered: recipientUserID != nil`; `backend/internal/repository/message_repository.go`, `Create` omits the column; migration 000002 defines `DEFAULT true`. `ListBySessionID` also omits the column from SELECT/Scan. | **CONFIRMED.** Pending messages are stored with DB default `true`, so `ListUndelivered` will not find them. Separately, the history API returns Go's zero value `false`, causing the panel to show “не доставлено” for all messages. |
| Pending messages are marked delivered before Telegram accepts them | `backend/internal/api/sessions.go`, `collectUndeliveredMessagesFor` calls `MarkDelivered` before returning messages to the bot; `bot/internal/handlers/start.go` then calls `bot.Send` and only logs failures. | **CONFIRMED / High.** Failed Telegram sends are not retried and the DB state already says delivered. |
| Media is deleted after successful Telegram send | `bot/internal/handlers/photo.go`, `voice.go`, and `start.go`: delivery functions call `fileutil.RemoveFile` after successful send. | **CONFIRMED.** The panel's media components exist, but files may already be gone when an operator opens history. This compounds the nil media-directory wiring defect. |
| Nginx strips `/api/` | `infra/nginx/nginx.conf`: `location /api/` uses `proxy_pass http://backend:8080/;`, replacing the matched prefix with `/`; Gin routes are registered under `/api`. | **CONFIRMED.** Requests such as `/api/auth/login` are forwarded as `/auth/login` and will not match the Gin route. |
| Panel is not deployed by Compose | `infra/docker-compose.yml` has no webpanel service; Nginx `location /` returns a text response; `webpanel/Dockerfile` is zero bytes. | **CONFIRMED.** The panel source is present, but the production stack does not build/serve it. |
| Retention anonymizes users based on an old closed session without checking other sessions | `backend/internal/retention/pii.go`, `PIIAnonymizer.Run`: it collects participant IDs from old closed sessions and calls `AnonymizeUser`; no query checks whether that user has another active/non-closed session. | **CONFIRMED / High.** `AnonymizeUser` sets `telegram_id` to a negative value, breaking Telegram lookup/routing for a user still active elsewhere. |
| Retention can delete DB rows after file deletion errors | `backend/internal/retention/messages.go`: file deletion errors are appended/logged, but message IDs are still added to `idsToDelete` and DB rows are deleted after the loop. | **CONFIRMED.** A file that could not be removed can become an orphan without its message row. |
| Telegram file download has no timeout/size limit | `bot/internal/handlers/photo.go` and `voice.go`: both use `http.Get(url)` and `io.Copy(out, resp.Body)` with no explicit timeout or byte cap. | **CONFIRMED.** On transport errors, wrapped `*url.Error` can include the Telegram file URL containing the bot token; the error is logged. |
| TOTP brute-force protection is absent | `backend/internal/api/auth.go`: pending token has a 5-minute TTL; failed TOTP returns 401 without incrementing/limiting attempts. No rate-limit middleware was found in the supplied routes. | **CONFIRMED.** |
| TOTP secret is plaintext and enrollment is premature | `backend/internal/api/auth.go`: first password login generates and saves `TOTPSecret` before successful code confirmation; `operators.totp_secret` is stored as a string field and no encryption is applied in the repository. | **CONFIRMED.** If setup is abandoned after secret persistence, the next login can require a code for a secret the operator may not have saved. |
| Suspended/demoted operators keep existing privileges | `backend/internal/api/operators.go`, `handleUpdateOperator`: revocation is performed only when status becomes `disabled`; role changes and `suspended` do not revoke. `requireAuth` trusts role/status cached in Redis session data. | **CONFIRMED.** |
| Audit log is readable by all authenticated operators | `backend/internal/api/router.go`: `/audit-log` is registered on `authenticated`, outside the `adminOnly` group. | **CONFIRMED.** |
| Incoming request PII is retained in plaintext | `backend/internal/api/incoming_requests.go` and `backend/internal/repository/incoming_request_repository.go`: Telegram ID, username, first name, first message are stored directly; no incoming-request cleanup was found in retention. Alert text also copies `first_message_text` into the operator group. | **CONFIRMED.** |
| Redis is volatile and unauthenticated in Compose | `infra/docker-compose.yml`: Redis has no password, no persistent volume, and no persistence command. | **CONFIRMED.** Restart loses operator sessions and moderation counters. |
| Frontend token is not persisted to localStorage | `webpanel/src/stores/authStore.ts` stores token in plain Zustand state and does not use `persist`; no localStorage/sessionStorage references were found under `webpanel/src`. | **CONFIRMED CORRECTION.** The previous localStorage concern is not supported by the supplied source. Active XSS remains relevant. |

### N.2 Important corrections to revision 2

1. **Frontend completeness:** revision 2 said `webpanel/src/api/*`, `stores/authStore`, and `types/api` were unavailable. They are present in this ZIP. The panel can now be statically reviewed; its build still has not been run.
2. **Documentation availability:** revision 2 said there was no `docs/` directory. It is present and includes API, architecture, environment, migrations, operator setup/guide, README, and retention docs. The issue is quality/consistency, not absence.
3. **First admin and migrations:** there is no automated bootstrap or Compose migration service, but `docs/OPERATOR_SETUP.md` documents a manual SQL first-admin creation, and `docs/MIGRATIONS.MD` documents running `migrate/migrate` via Docker. Describe these as **manual first-run procedures**, not as undocumented/impossible.
4. **Coverage:** committed artifacts do not support a blanket 0% statement for the entire API/moderation/auth. Approximate figures from the supplied artifacts are backend API 0.6%, moderation 37.5%, auth 15.3%; bot handlers 6.3%. These artifacts may be stale and were not regenerated.
5. **Auth token storage:** the supplied Zustand store is in-memory only. Do not claim localStorage persistence.
6. **Media UI:** the React panel has photo/audio preview components. The problem is runtime availability: nil media directories due to `NewServer` wiring and bot deletion of files after Telegram delivery.
7. **Migration and admin bootstrap:** both have manual documentation. They are still production/deployment blockers because the procedures are not automated, repeatable, or integrated into the Compose startup.
8. **Retention env configuration:** the code reads `RETENTION_PII_MONTHS` and `RETENTION_MESSAGE_MONTHS`; those are not hardcoded-only. However, Compose hardcodes `RETENTION_RUN_HOUR: "03"` rather than interpolating the `.env` value.
9. **Redis in bot:** the bot config includes `REDIS_URL`, but the reviewed bot code does not use Redis; backend uses Redis for operator sessions and moderation counters.
10. **Stop notification:** the bot's `/stop` handler sends a neutral notice to the counterpart after the backend response. The backend nil `botClient` bug currently panics before the response is returned, so the flow is broken for that reason; do not describe the notification feature as entirely absent.

### N.3 Additional documentation and packaging defects

| ID | Severity | Location | Finding |
|---|---|---|---|
| DOC-01 | Medium | `docs/README.md` | Contains literal merge-conflict markers (`<<<<<<< HEAD`) and conflicting content. It claims “MVP fully implemented” despite the confirmed blockers. |
| DOC-02 | High | `docs/API.md` | Says `/internal/*` is protected by network isolation and inaccessible externally through production Nginx. The same repository publishes backend port `8080:8080`, so the statement is false for the supplied Compose deployment. |
| DOC-03 | Medium | `docs/README.md` | References `docs/SECURITY_CHECKLIST.md`, which is absent from the ZIP. |
| DOC-04 | Medium | `docs/README.md` | Says photo/voice are placeholders without previews; the React page contains preview components, though backend/file lifecycle bugs prevent reliable previews. |
| DOC-05 | Medium | `docs/README.md` | Says retention periods are hardcoded; code supports `RETENTION_PII_MONTHS` and `RETENTION_MESSAGE_MONTHS`. It also overstates the implementation status and describes send-as as an operator feature even though the panel has no send-as composer. |
| PKG-01 | Medium | `webpanel/Dockerfile`; `webpanel/webpanel/` | Dockerfile is empty; nested folder contains a duplicate Vite scaffold. Determine whether nested scaffold is accidental, remove only after confirming, and implement a real panel image. |
| CFG-01 | Medium | `.env.example`; `infra/.env.example`; `infra/docker-compose.yml` | Two templates are inconsistent; infra template omits `OPERATOR_GROUP_CHAT_ID`; Compose hardcodes retention run hour. Consolidate to one documented source of truth. |

### N.4 Recommended revised implementation order

The original P0/P1 task list remains a useful backlog, but the sequence should be adjusted to avoid building features on broken foundations:

1. **P0 — Restore core backend wiring:** assign `botClient`, `photosDir`, `voiceDir` in `NewServer`; add constructor/wiring tests. This restores the ability to test real flows, but does not make the service safe to expose.
2. **P0 — Close the unauthenticated internal API exposure:** remove host publication of backend `8080`; add service-to-service authentication to backend `/internal/*` and bot `:9090`; verify network reachability from host and sibling containers. Do not rely on Nginx not proxying a route while the backend port is published directly.
3. **P0 — Fix invite and bind authorization atomically:** unpredictable, expiring, single-use invite tokens; transactionally claim a vacant role slot; reject closed/paused sessions and duplicate/dual-role binds; only return messages to the participant whose bind succeeded.
4. **P0 — Fix authentication:** rate-limit login and TOTP attempts; encrypt TOTP secrets; persist enrollment only after successful confirmation; implement recovery/reset and password change; revoke sessions on suspension/role changes.
5. **P0 — Fix retention safety and PII lifecycle:** never anonymize a user still attached to any active/non-closed session; decide retention for incoming requests; validate file IDs; do not delete message rows when media deletion fails.
6. **P0/P1 — Correct delivery semantics:** persist/read `delivered` consistently, then implement an outbox with retries and idempotency. Only mark a message delivered after Telegram confirms success. Cover immediate and deferred text/photo/voice sends.
7. **P1 — Make the application deployable:** fix Nginx URI handling; build/serve panel; add a migration service or explicit release step; automate first-admin bootstrap; correct `.env` templates; add TLS/security headers/body limits/timeouts/healthchecks.
8. **P1 — Complete operator panel:** add send-as composer with explicit impersonated-role choice, audit pagination/access control, search/filter/pagination, runtime branding and localization; test media preview after fixing storage lifecycle.
9. **P1 — Add test coverage and CI:** API/auth/moderation/bot delivery tests first; then frontend build/lint/e2e, Docker/Compose/migration smoke, backup/restore drill.
10. **P1/P2 — Documentation and handover:** remove conflict markers, reconcile all docs with actual routes/behavior, add SECURITY, PRIVACY_AND_DATA_HANDLING, OPERATIONS, SECURITY_CHECKLIST, PROJECT_HANDOVER, and a reproducible clean-install guide.
11. **P2 — Hardening/cleanup:** non-root containers, Redis authentication/persistence, least-privilege DB roles, key rotation, audit integrity, code deduplication, remove unused nested Vite scaffold after owner confirmation.
12. **P3 — Commercial enhancements:** white-label configuration, metrics, webhook mode, transcript export, SSO/OIDC only after core reliability/security is complete.

### N.5 Scope boundary

This revision is a **read-only static review plus local tool availability checks**. No source code, migrations, Docker files, frontend files, or documentation inside the uploaded ZIP were modified. No live Telegram API, production database, deployed host, browser session, or external service was accessed. Findings requiring runtime proof remain open until the next validation phase.
