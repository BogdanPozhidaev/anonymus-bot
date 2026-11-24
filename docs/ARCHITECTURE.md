# Architecture

## Components
- **Telegram bot (Go):** polling or optional webhook, update dispatcher, handlers, media download, and Telegram message sending.
- **Backend (Go/Gin):** internal bot-facing endpoints and operator API; session/message workflows, moderation, authentication, repositories, and retention-related functionality.
- **PostgreSQL:** users, sessions, messages, operator data, incoming requests, and audit records.
- **Redis:** operator web sessions, authentication state, and moderation counters.
- **Operator panel (React/TypeScript/Mantine):** login/TOTP, session and request views, operator and audit-related screens.
- **Infrastructure:** Compose, Nginx, shared media volumes, optional Prometheus and Grafana, and CI workflows for backend, bot, panel, and infrastructure.

## Main flows
1. A user opens a Telegram deep link containing a single-use invitation token for a participant slot.
2. The bot calls the backend to redeem the invitation and bind the participant.
3. The bot relays user messages to the backend; the backend resolves the session/recipient and applies text moderation.
4. The backend queues delivery in the PostgreSQL outbox. The delivery worker calls the bot to send through Telegram and retries eligible failures. See [message delivery](API.md#message-delivery).
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

Use the [deployment guide](DEPLOYMENT.md) for the edge, TLS, and service
configuration. The [handover guide](PROJECT_HANDOVER.md) lists the checks
required for independent acceptance.
