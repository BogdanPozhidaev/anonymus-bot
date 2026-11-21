# Anonymus Bot

Anonymus Bot relays text, photos and voice between participants in a
Telegram session. Operators manage sessions and view history in a web panel.

- [First startup, migrations and TLS](docs/DEPLOYMENT.md)
- [Daily operation and recovery](docs/OPERATIONS.md)
- [Architecture](docs/ARCHITECTURE.md) and [API](docs/API.md)
- [Security](SECURITY.md) and [data handling](docs/PRIVACY_AND_DATA_HANDLING.md)
- [Handover](docs/PROJECT_HANDOVER.md) and [development roadmap](DEVELOPMENT_ROADMAP.md)

For local verification use Go as declared in each module, Node.js 22 and
Docker with Compose. Run `go test ./... -race` in `backend` and `bot`.
In `webpanel` run `npm ci`, then `npm run lint`, `npm run typecheck` and
`npm run build`. Backend integration tests use temporary Docker
containers. They do not require the working deployment or a Telegram token.
