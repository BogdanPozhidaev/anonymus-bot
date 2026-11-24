# Project documentation

Start with the [product overview](../README.md). [Русская версия](README.ru.md)

| Audience | Guides |
| --- | --- |
| Developers | [Architecture](ARCHITECTURE.md), [API](API.md), [local development](#local-development) |
| Deployment owners | [Deployment](DEPLOYMENT.md), [configuration](ENV_VARS.md), [migrations](MIGRATIONS.MD), [first administrator](OPERATOR_SETUP.md) |
| Operators | [Operator guide](OPERATOR_GUIDE.md), [moderation](MODERATION.md) |
| Maintainers | [Operations](OPERATIONS.md), [retention](RETENTION.md), [backup and restore](../deploy/backup/README.md), [security checklist](SECURITY_CHECKLIST.md) |
| Reviewers and recipients | [Handover](PROJECT_HANDOVER.md), [data handling](PRIVACY_AND_DATA_HANDLING.md) |

## Local development

For the built panel served by Nginx and for TLS, follow the
[deployment guide](DEPLOYMENT.md). To develop the panel through Vite,
prepare fixture settings and both service-token files as described in
[configuration](ENV_VARS.md). From the repository root:

```powershell
$compose = @('-p', 'anonymus-dev', '--env-file', 'C:/private/dev.env', '-f', 'infra/docker-compose.yml', '-f', 'infra/docker-compose.dev.yml')
docker compose @compose config --quiet
docker compose @compose up -d --build --wait backend
docker compose @compose run --rm -T --no-deps migrate migrate status
curl.exe --fail http://127.0.0.1:8080/health
```

Create a fixture administrator using [first administrator setup](OPERATOR_SETUP.md)
with the same Compose arguments. In `webpanel`, run `npm ci` and
`npm run dev`, then open `http://localhost:5173` to sign in. The
development override publishes PostgreSQL, Redis and backend ports on
loopback. Check host port conflicts before startup. Use the same project
name and environment file when operating an existing stack.
