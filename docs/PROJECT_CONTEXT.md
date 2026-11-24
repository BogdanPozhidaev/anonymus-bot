# Project Context — Anonymus Bot

## Purpose
Anonymus Bot is a Telegram-based platform for mediated communication between a customer and a service provider. It is intended to reduce direct exchange of Telegram identifiers and contact details while allowing an operator to moderate conversations through a web panel.

**Positioning:** privacy-preserving communication platform for Telegram. Do not promise absolute anonymity: operators may access conversation content, and Telegram and hosting infrastructure retain their own metadata.

## Product scope observed in the repository
- Telegram bot for joining a session and exchanging text, photos, and voice messages.
- Backend-mediated sessions, message relay, moderation signals, and operator alerts.
- Web operator panel with authentication, TOTP, session views, and audit-related UI.
- PostgreSQL persistence, Redis-backed operator sessions/moderation counters, Docker Compose and Nginx configuration.
- Retention/anonymization jobs.

Use the [handover guide](PROJECT_HANDOVER.md) for current acceptance steps
and the [development log](DEVELOPMENT_LOG.md) for recorded verification.
The [known issues index](KNOWN_ISSUES.md) and [security audit](SECURITY_AUDIT.md)
describe the historical review.

## Commercial objective
Prepare the existing project for a possible whole-project sale, including source code and properly documented intellectual-property rights. A credible handover requires reproducible setup, tests, a working demo, accurate documentation, dependency/license review, contributor/IP confirmation, and secret/history scanning. No valuation or production-readiness claim is established by this context file.

## Technology observed in the audited archive
- Backend: Go, Gin, PostgreSQL/pgx, Redis.
- Telegram bot: Go and `go-telegram-bot-api`.
- Operator UI: React, TypeScript, Mantine, Vite.
- Deployment: Docker Compose, Nginx; CI workflows for backend and bot.

Reconfirm versions and configuration in the current repository before implementation.

## Current baseline and evidence
The independent static review covered the uploaded archive at Git HEAD `5e63f63` (`Update README.md`). It did not access a live deployment or Telegram production environment. Backend and bot tests could not be run in that review environment because installed Go was older than module requirements; Docker was unavailable; `npm ci` timed out. Do not describe those checks as passing.

See the [development roadmap](../DEVELOPMENT_ROADMAP.md) for the detailed
audit and implementation sequence.
