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

## Pending decisions

- Internal API authentication and network exposure model.
- Deep-link format, expiry, single-use behavior, and participant binding rules.
- Durable message delivery/outbox and retry semantics.
- TOTP secret encryption, enrollment confirmation, recovery, and session revocation policy.
- Retention periods and treatment of incoming requests, audit records, and users in multiple sessions.
- Production panel packaging and Nginx route layout.
- Whether to retain or remove the nested `webpanel/webpanel/` scaffold and empty Dockerfile.

Add a dated decision entry before implementing a choice that affects security, data model, API behavior, or deployment architecture.
