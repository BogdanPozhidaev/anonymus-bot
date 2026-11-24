# API reference

[Русская версия](API.ru.md)

## Access

- `/api/*` is the operator panel API. Protected routes require
  `Authorization: Bearer <operator-session-token>`.
- `/internal/*` is the bot-facing API. Send exactly one
  `X-Internal-Token` header. Missing, duplicate, or invalid headers
  return HTTP 401. See [environment configuration](ENV_VARS.md).

The base Compose configuration keeps the backend port private. Nginx blocks
internal routes, and the development override publishes the backend on loopback
port 8080. Run Swagger and [k6](OPERATIONS.md#isolated-k6-relay-check) on an
isolated test instance.

## Authentication

| Method | Path | Result |
| --- | --- | --- |
| POST | `/api/auth/login` | Valid credentials return `totp_required` or `totp_setup_required`. |
| POST | `/api/auth/totp/verify` | Verifies a TOTP code and returns `session_token`. |
| POST | `/api/auth/logout` | Revokes the current session. |

Within five minutes, login allows five attempts per login and IP pair and
30 per IP. TOTP allows five checks per pending token during its lifetime
and ten per operator per five minutes. Exceeding a limit returns HTTP 429
with `Retry-After` in seconds. An unavailable rate limiter returns HTTP 503.
An invalid password, unknown login, or inactive account returns HTTP 401
at the password step. Role or status changes invalidate the affected
operator's sessions and pending sign-ins.

### Optional operator sign-in with OpenID Connect

| Method | Path | Result |
| --- | --- | --- |
| GET | `/api/auth/oidc` | Reports whether SSO is enabled. |
| POST | `/api/auth/oidc/start` | Returns `authorization_url` and sets a short-lived browser cookie. |
| GET | `/api/auth/oidc/callback` | Verifies the provider response and redirects to the panel. |
| POST | `/api/auth/oidc/complete` | Consumes the handoff cookie and returns `totp_required` with a `pending_token`. |

The last three routes require SSO to be enabled. Use a single HTTPS origin.
An invalid, expired, or consumed handoff returns HTTP 401. Complete TOTP
using the standard verification route. See [operator SSO operations](OPERATIONS.md#operator-sso-with-openid-connect).

## Readiness

`GET /api/health` is available through Nginx. It returns HTTP 200 with
`"status": "ok"` when PostgreSQL and Redis are ready, or HTTP 503 with
dependency statuses when either is unavailable. `GET /health` is also
available directly on the backend.

```http
GET /api/health HTTP/1.1
Host: panel.example.invalid
```

## Sessions

An administrator can access all sessions. Other operators can access
their assigned sessions.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/sessions` | List accessible sessions. |
| POST | `/api/sessions` | Create a session. |
| GET | `/api/sessions/:id` | Read session details. |
| POST | `/api/sessions/:id/invites` | Issue an invitation for `role` (`client` or `executor`). HTTP 201 includes `token` and `expires_at`. |
| DELETE | `/api/sessions/:id/invites/:role` | Revoke an unused invitation. |
| PATCH | `/api/sessions/:id/status` | Change session status. |
| PATCH | `/api/sessions/:id/owner` | Assign a session owner. |
| GET | `/api/sessions/:id/messages` | List retained messages. |
| POST | `/api/sessions/:id/send-as` | Send on behalf of a participant in an active session. |
| GET | `/api/messages/:id/media` | Read attached media after a session access check. Supports HTTP range requests. |
| GET | `/api/sessions/:id/transcript` | Download an audited CSV or PDF export. |

An invitation is single-use and expires after 24 hours. Reissuing it
revokes the previous invitation for that role. Issue and redeem invitations
for vacant participant slots in active sessions. Share the link privately.
The bot redeems the token via `POST /internal/sessions/bind`. Previously
bound participants can select a session with `/sessions`.

For operator replies, send `recipient_role`, the expected
`recipient_user_id` from the session, and a nonzero UUID `request_key`
along with `impersonated_role`, `content_type`, and `content`. Recipient
and impersonated roles are opposite. Retry an unchanged command with
the same key. The key is scoped to the operator and retained with its
outbox entry. A successful retry returns the original message ID. A
different payload with the same key, a changed recipient, or an
inactive session returns HTTP 409. Revoked access returns HTTP 403 or
401 according to session validity.

The explicit response contains `message_id` and `sender_label`.
Legacy requests without the three new fields retain their request
and response shape. Their recipient is fixed at enqueue time.

## Operators and incoming requests

| Method | Path | Required access |
| --- | --- | --- |
| GET | `/api/operators` | Administrator |
| POST | `/api/operators` | Administrator |
| PATCH | `/api/operators/:id` | Administrator |
| GET | `/api/incoming-requests` | Operator session |
| GET | `/api/incoming-requests/:id` | Operator session. Assigned requests require their owner or an administrator. |
| POST | `/api/incoming-requests/:id/claim` | Operator session. Claims a new request and returns HTTP 204. |
| POST | `/api/incoming-requests/:id/session` | Request owner or administrator. Creates or opens the linked session. |
| POST | `/api/incoming-requests/:id/transfer` | Administrator. Transfers an assigned request and its linked session. |
| PATCH | `/api/incoming-requests/:id/status` | Operator session |

Use `status=in_progress` to list your own assigned requests. Claiming
your own request again succeeds, while a competing claim returns HTTP 409.
Session creation takes `title` and optional `language`. It returns HTTP 201
with `id` on creation or HTTP 200 with the same `id` on repeat if the
caller can access the session. Assignment, linking and their corresponding
audit records are transactional. The legacy status operation rejects changes
to assigned requests with HTTP 409. A linked session still uses the
existing session access rules.

Administrators can add `scope=all` to `status=in_progress` to list
every assigned request. Transfer accepts `expected_owner_id` and
`operator_id`. A successful transfer or a retry to the same current
owner returns HTTP 204. A stale owner, inactive target, or mismatch
between the request and linked session owners returns HTTP 409. Both
ownership changes and their audit event commit together.

Operator roles are `admin` and `operator`. A change that would leave no
active administrator returns HTTP 409.

## Audit log

`GET /api/audit-log` requires an administrator. Filters are `actor_id`,
`action`, `target_type`, `date_from`, and `date_to`. Dates accept
`YYYY-MM-DD` or RFC 3339. `limit` accepts 1–200 and defaults to 100.
`offset` defaults to 0. JSON contains `actor_type` and `has_more`.
With `format=csv`, the response exports the selected page.

## OpenAPI

Set `ENABLE_SWAGGER=true` on a development instance to access
`/swagger/index.html`. Swagger is disabled by default.

## Message delivery

`GET /api/sessions/:id/messages` returns `delivered` and
`delivery_status`. New messages are `queued`, `retrying`, `delivered`, or
`failed`. `legacy_unknown` identifies history recorded before the outbox.
Its historical boolean alone does not prove delivery. The protected relay
and bind calls no longer return a recipient Telegram identifier or pending
message contents to the bot. The delivery worker uses the protected
backend-to-bot endpoint for each attempt.

The backend saves the message and its outbox entry in one transaction.
Work waits for the counterpart to join and is eligible while the session
is active. After a transport or Telegram error, the worker retries at
5, 10, 20 and 40 seconds. After the fifth failed attempt, history shows
`failed`. For participant messages, a separate retryable notice is sent
privately to the sender. Messages sent on behalf of a participant by an
operator report the failure in the panel.

When an operator reply becomes ineligible before the worker claims it,
history reports `failed`. This includes replacement of the recipient,
session closure, and revocation of the operator's applicable access.
Queued operator replies from before the migration have no saved
recipient and fail on claim. A pause delays delivery while the recipient
and operator access remain valid. A network call already in progress
cannot be cancelled retroactively.

Successful Telegram API acceptance is the delivery confirmation. It
does not report whether the recipient read the message. Workers lease
work for 90 seconds and a completed entry is not sent again. If Telegram
accepts a send and its response is lost, or the database cannot record
that success, a later retry may send a duplicate. Telegram does not offer
an idempotency key for these send calls.

## Session transcript download

`GET /api/sessions/:id/transcript?format=csv` or `?format=pdf` downloads
the retained history in chronological order using the same session access
rules as the panel. It requires the operator Bearer token. The assigned
operator and administrators can download from the session detail screen.
The export includes the UTC timestamp, sender role, content type, delivery
status, operator send-as attribution, message text and media captions.
It omits attachment files, file references, participant identifiers and
session titles. Text written in the conversation is included in the export.

CSV uses UTF-8 with a BOM and prefixes spreadsheet formula-like text with
an apostrophe. PDF embeds a font with Cyrillic support and displays
unsupported characters using `[U+XXXX]` notation. Use CSV when the original
Unicode text is needed.

Downloads are attachments with `Cache-Control: no-store`. Each successful
export requires a `session_transcript_exported` audit record with operator,
session, format and message count. If the audit write fails, the endpoint
returns HTTP 503. The limit is 2,000 messages and 1 MiB of source text per
export, plus 250 pages for PDF. Oversized exports return HTTP 422 without
a partial file. If all four exporter slots are in use, the endpoint
returns HTTP 503.

## Request limits and paging

The backend limits API bodies to 64 KiB and returns HTTP 413 when a body
exceeds the limit. Media upload from Telegram uses the separate bot limit.
`GET /api/sessions` and `GET /api/sessions/:id/messages` accept `limit`
(1–200, default 50) and `offset`. Both return `has_more`. Sessions can be
filtered by `status` and searched by title or exact ID with `q`. History
pages start with the newest messages and display each page in chronological
order. Both endpoints apply the existing session access rules.

The same page parameters and `has_more` field apply to public operator
and incoming-request lists and protected internal incoming-request and
session lists. The internal participant list places the selected session
first so `/current` works even when it is older than the newest sessions.
Results use a stable ID tie-breaker. Telegram list menus request ten
rows per page.

An internal incoming-request submission reuses an open request for the
same sender or a request created within the past ten minutes. A new
record returns HTTP 201 and sends an operator alert. A reused record
returns HTTP 200. Both responses include the request `id` and its current
`status` (`new`, `in_progress`, or `processed`). The first private text sent to
the bot without a selected session fills the open request's empty
first-message field. Later text leaves that field intact.

## Password change

`POST /api/auth/password` requires an authenticated operator session,
the current password and a replacement. The replacement must contain
at least 12 characters, fit in 72 UTF-8 bytes, contain no control
characters and differ from the current password. Up to five current
password checks per operator are allowed in five minutes, separately
from login.
HTTP 204 indicates success. Existing sessions and pending sign-ins are
then invalidated. The next sign-in still requires TOTP.

After verifying TOTP for an account with a temporary password,
`POST /api/auth/totp/verify` returns `password_change_required: true`.
The accompanying five-minute bearer token works only for password
change and logout. After a successful change, sign in again with
the permanent password and the same authenticator.

`POST /api/operators/:id/password/reset` requires an administrator
session and resets another active or invited operator's password.
The response returns the generated password once with
`Cache-Control: no-store`. The backend saves the hash, marks the
password for mandatory change and invalidates older sessions and
pending sign-ins in the same update. Existing TOTP enrollment is
preserved. The audit entry records the administrator and the target.

## TOTP recovery

- `POST /api/auth/totp/recovery-codes` requires a full operator session,
  the current password and a valid TOTP. It issues eight codes once in
  a no-store response. PostgreSQL stores only their digests. Reissuing
  codes invalidates earlier codes, sessions and pending sign-ins.
- `POST /api/auth/totp/recover` requires a pending token from the password
  step of sign-in and one recovery code. It consumes the password
  challenge even when the submitted recovery code is invalid. On
  success it consumes the code and returns a fresh five-minute TOTP
  enrollment challenge without granting panel access. The login and
  recovery code submission endpoints use the existing attempt limits.
- Confirm the new authenticator using `POST /api/auth/totp/verify`.
  Successful confirmation replaces the encrypted secret and revokes
  earlier sessions, pending sign-ins and unused recovery codes.
  The previous authenticator remains usable until confirmation.
  An unused code is needed to restart an interrupted recovery.

## Operator Telegram linking

| Method | Path | Required access | Success |
|---|---|---|---|
| GET | `/api/account/telegram-link` | Operator session | 200 with `linked` |
| POST | `/api/account/telegram-link` | Operator session | 201 with `code` |
| POST | `/internal/operators/telegram-link/claim` | Bot service token | 200 with `confirmation_code` |
| POST | `/api/account/telegram-link/confirm` | Initiating operator account | 204 |

The bot sends `code` and the private sender's `telegram_id` in the claim
body. The panel sends the bot's confirmation code as `code`. Codes are
single-use and share the original five-minute expiration. Issuing a new
code replaces the pending challenge. Auth version changes invalidate
pending challenges. A missing or expired challenge returns 404 and an
account already linked to another operator returns 409. Responses
include `Cache-Control: no-store`. The audit event
`operator_telegram_linked` records the operator's internal ID.

The internal user lookup returns `language_code`. The internal operator
lookup returns `language`. The language update endpoint supports
`ru` and `en`. It updates an existing participant profile or a linked
active operator profile and returns 404 when neither exists.

## Operator audit

The audit records completed TOTP sign-in, sign-out, rejected credentials
within the attempt budget, session creation, incoming request status
updates, successful media access, and bot stop or switch actions. Entries
use internal actor and target identifiers. Authentication audit omits
credential input, tokens and TOTP codes. An unknown login has no actor ID.
Media range requests are separate HTTP accesses. Audit writes are
best-effort, with errors logged by the backend.

Session pause and close trigger neutral notices to the connected
participants. Delivery of these notices is best-effort and independent
of the persisted status.
