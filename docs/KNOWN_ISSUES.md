# Known Issues and Verification Status

This is a working index, not a substitute for the detailed `DEVELOPMENT_ROADMAP.md`. Findings refer to the audited archive at `5e63f63` and require revalidation before implementation.

## P0 — security/data-loss/deployment blockers
- Unauthenticated backend `/internal/*` endpoints exposed through published port 8080.
- Guessable session deep links and insufficient participant-bind authorization.
- Backend server fails to assign bot client and media directories.
- Pending messages can be marked delivered before Telegram send; delivery state is inconsistently persisted/read.
- Nginx `/api/` path mismatch; production Compose does not serve panel.
- TOTP/authentication gaps and sensitive staff Telegram access path.
- Retention can anonymize users still active in another session.

## P1 — correctness/privacy/operations
- Incoming request PII/content lacks reviewed retention cleanup.
- Media downloads lack timeout/size bounds; token-bearing URL errors may be logged.
- Redis session revocation gaps on role/status changes; Redis lacks auth/persistence in reviewed Compose.
- Audit-log route role restriction is weaker than expected.
- Files without a message reference can require manual cleanup. P1-1
  added durable delivery retries. Ambiguous Telegram responses can still
  cause duplicates. See [Delivery](API.md#message-delivery).
- Backend startup/shutdown and database lifecycle concerns are listed in roadmap.

## P2 — product/documentation/maintainability
- Operator panel lacks several workflow conveniences and has hardcoded Russian strings/bot username in reviewed source.
- Docs contain a merge-conflict marker, incorrect internal API network-protection statement, missing referenced security checklist, and readiness overclaims.
- `infra/.env.example` omits `OPERATOR_GROUP_CHAT_ID`; retention run hour is hardcoded in Compose despite docs describing it as configurable.
- Empty `webpanel/Dockerfile` and nested duplicate Vite scaffold need a deliberate disposition.
- Scan full Git history for secrets before external handover.

## Verification constraints observed during audit
- Backend module requires Go 1.26.0; bot module requires Go 1.25.1.
- Audit environment had Go 1.23.2; Go tests were not run.
- `npm ci` timed out; frontend lint/build were not run.
- Docker was unavailable; Compose config/build, migrations, and end-to-end tests were not run.
- Apparent Go formatting differences were line-ending normalization from ZIP, not established source-format defects.

Track remediation and actual test results in `DEVELOPMENT_LOG.md`.
