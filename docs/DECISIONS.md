# Architecture and Project Decisions

Record decisions only after they are explicitly agreed. Each entry should include date, context, decision, alternatives considered, consequences, and status.

## Confirmed project constraints

### D-001 — Improve the existing MVP incrementally
- **Status:** agreed project constraint
- **Decision:** Do not rewrite the project from scratch. Make small, reviewable changes that preserve working behavior unless a change is explicitly required.
- **Reason:** The repository already contains backend, bot, panel, persistence, and deployment code; first establish and repair the actual baseline.

### D-002 — Use privacy-preserving positioning, not absolute anonymity
- **Status:** agreed project constraint
- **Decision:** Describe the product as a privacy-preserving communication platform for Telegram. Do not promise absolute anonymity.
- **Reason:** Operators can access conversation content, and Telegram/infrastructure retain their own metadata.

### D-003 — Evidence-based readiness claims
- **Status:** agreed project constraint
- **Decision:** Distinguish implemented, built, tested, security-reviewed, and production-ready. State exact checks and limitations; do not infer production readiness from static inspection.

### D-004 — Authenticate calls to the backend internal API

- **Date:** 2026-11-21
- **Status:** agreed
- **Decision:** Use separate 32-byte hex-encoded service tokens for bot-to-backend
  and backend-to-bot calls, mounted from files outside the repository. Remove base Compose host
  publication and keep loopback publication in the development override.
- **Alternatives considered:** network isolation alone and reusing operator
  sessions. A dedicated credential fits the bot-to-backend trust boundary.
- **Consequences:** both services need both token files at startup. Rotation can
  require a coordinated recreation with a short interruption.

### D-005 — Issue single-use session invitations

- **Date:** 2026-11-21
- **Status:** agreed
- **Decision:** Issue one bearer invitation per vacant role for 24 hours.
  Store its hash, replace it on reissue, and allow explicit revocation.
  Redeem it atomically in an active session. The intended recipient can
  receive the link from an operator without prior account registration.
- **Alternatives considered:** recipient-bound invitations and a grace period
  for old links.
- **Consequences:** old ID-and-role links are rejected. The migration and
  coordinated backend, bot, and panel update are required. Existing members
  retain their assignments and use `/sessions`.

### D-006 — Limit operator authentication attempts

- **Date:** 2026-11-21
- **Status:** agreed
- **Decision:** Reserve attempts with Redis before password or TOTP checks.
  Use five-minute windows with login limits of 5 per login/IP and 30 per IP,
  and TOTP limits of 5 per pending token and 10 per operator. Atomically
  consume the pending token before issuing an operator session.
- **Alternatives considered:** token-only TOTP limits allow a password holder
  to restart the flow and obtain another attempt budget.
- **Consequences:** login is temporarily unavailable without Redis. Proxy
  deployments must supply a trusted source for the client IP. Password
  policy will be addressed with credential management in P1-5.

### D-007 — Final security and retention rules for P0

- **Date:** 2026-11-21
- **Status:** implementation decisions delegated by project owner
- **Decision:** Compare operator authentication versions against PostgreSQL
  on each authenticated request. An authorization change invalidates prior
  sessions and pending challenges. Limit audit access to admins and classify
  audit actors by entity type.
- **Retention:** Use the existing PII retention period for all incoming
  requests, including unprocessed requests. Keep media records after
  unsuccessful deletion. Check session eligibility again at deletion time.
- **Consequences:** Migration 000005 requires operators to sign in again.
  Run retention in dry-run mode before enabling it against existing data.

### D-009 — Durable delivery with bounded retries

- **Date:** 2026-11-21
- **Status:** implemented for P1-1
- **Decision:** Store each new message and its delivery item atomically.
  Resolve the counterpart from the current session and retry failed sends
  up to five attempts. Queue a separate sender notice on terminal failure.
- **Consequences:** Telegram acceptance is recorded after the bot returns
  success. A lost response can still cause a duplicate. Legacy delivery
  history is shown as unverified and excluded from replay.

## Pending decisions

- TOTP secret encryption, enrollment confirmation, recovery, and session revocation policy.
- Retention periods and treatment of incoming requests, audit records, and users in multiple sessions.
- Production panel packaging and Nginx route layout.
- Whether to retain or remove the nested `webpanel/webpanel/` scaffold and empty Dockerfile.

Add a dated decision entry before implementing a choice that affects security, data model, API behavior, or deployment architecture.
