# Security Audit — Static Findings

**Baseline:** uploaded archive, Git HEAD `5e63f63`, reviewed 2026-11-21. Static code/config review only; no live system, penetration test, or production environment was accessed. Revalidate each item against the current working tree before remediation.

## Critical
- **Unauthenticated backend `/internal/*` routes exposed via published port 8080.** Reviewed routes permit caller-supplied Telegram IDs and access to relay/bind/stop workflows and selected user/request data. Scope is endpoint-specific; this does not establish unrestricted enumeration of all operators.
- **Guessable session deep links and weak bind authorization.** Payloads use sequential session IDs plus a role suffix; bind logic did not reliably enforce participant-slot ownership before returning pending messages.
- **Backend `NewServer` fails to assign `botClient`, `photosDir`, and `voiceDir`.** Some flows dereference a nil client; media handlers receive empty paths.

## High
- **Message delivery state is inconsistent.** Insert omits `delivered` while DB default is true; history query omits the field, yielding false in Go's zero value.
- **Pending messages marked delivered before Telegram send succeeds.** Failed sends can lose messages; no durable outbox/retry was found.
- **Nginx/Compose panel path is broken.** `/api/` proxy strips a prefix that Gin routes include; Compose does not serve the panel.
- **Authentication hardening gaps.** No login/TOTP throttling; TOTP secret stored plaintext and enrollment persisted before confirmation; password change does not revoke other sessions.
- **Retention can anonymize a user who remains in another active session.** Anonymization changes Telegram ID and can break routing.
- **Incoming requests retain plaintext identifiers/content without the reviewed retention cleanup.**
- **Telegram media download uses `http.Get` without timeout/size cap; URL-bearing errors may expose the bot token in logs.**
- **Staff Telegram-linked flow bypasses web password/TOTP and permits sensitive operations.** Verify intended authorization model.

## Medium / additional confirmed or probable items
- Role demotion/suspension does not revoke cached Redis sessions; middleware trusts cached role/status.
- Audit-log API is available to authenticated operators, not only admins.
- Redis has no password and no persistence volume/AOF in reviewed Compose.
- Bot internal server on port 9090 has unauthenticated send endpoints; it was not published to host in reviewed Compose but is reachable within the container network.
- Contact detection is narrow and text-only; Redis moderation errors fail open.
- Message/media cleanup can leave orphan files when file deletion fails.
- Password reset and other operator-management authorization issues are described in the roadmap's confirmed findings.

## Remediation order
1. Close external access to internal routes and add explicit service authentication/authorization.
2. Fix participant binding and deep-link security.
3. Fix server dependency wiring and restore reliable media/message delivery.
4. Correct auth/TOTP/session revocation and staff access boundaries.
5. Make retention safe across active sessions; define and enforce incoming-request retention.
6. Fix deployment routing, secrets/logging, resource limits, and tests.

Do not claim security certification, GDPR compliance, or production readiness based on this audit.
