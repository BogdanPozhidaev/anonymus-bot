# Architecture (audited baseline)

> Static description of the uploaded repository at Git HEAD `5e63f63`. Verify against the current tree before relying on it.

## Components
- **Telegram bot (Go):** long polling, update dispatcher, handlers, media download, and Telegram message sending.
- **Backend (Go/Gin):** internal bot-facing endpoints and operator API; session/message workflows, moderation, authentication, repositories, and retention-related functionality.
- **PostgreSQL:** users, sessions, messages, operator data, incoming requests, and audit records.
- **Redis:** operator web sessions and moderation counters. The bot's `REDIS_URL` setting appeared unused in the reviewed code.
- **Operator panel (React/TypeScript/Mantine):** login/TOTP, session and request views, operator and audit-related screens.
- **Infrastructure:** Compose, Nginx, shared media volumes, and separate backend/bot CI workflows.

## Main flows
1. A user opens a Telegram deep link containing a session identifier and role suffix.
2. The bot calls the backend bind endpoint.
3. The bot relays user messages to the backend; the backend resolves the session/recipient and applies text moderation.
4. The bot sends the message to the recipient through Telegram.
5. Operator alerts are sent through the bot's internal HTTP service.
6. Retention jobs anonymize selected records and remove older messages/media.

## Trust boundaries to verify
- Telegram user ↔ Telegram Bot API ↔ bot container.
- Bot container ↔ backend internal API.
- Browser/operator ↔ Nginx ↔ backend operator API.
- Backend/bot/retention containers ↔ PostgreSQL, Redis, and shared media volumes.

The backend authenticates bot requests on `/internal/*` using a service token
mounted into both containers. The base Compose stack keeps port 8080 on the
bridge network, and the development override publishes it on loopback.
Nginx rejects `/api/internal` and its descendants before proxying.
The bot calls the backend for data operations. Redis credentials are
configured only for the backend, which uses Redis for operator sessions
and moderation counters.

The audited baseline had unauthenticated internal routes on the backend and a published backend port; see `SECURITY_AUDIT.md`. Do not assume an internal route is protected merely because its path contains `/internal`.

## Deployment caveats found in static review
- Nginx `/api/` proxy path handling appeared inconsistent with Gin's `/api` routes.
- Production Compose did not serve/build the web panel; `webpanel/Dockerfile` was empty.
- Backend server initialization omitted bot client and media directories, breaking dependent flows.

These are audit findings, not a substitute for reproducing them in the current checkout.
