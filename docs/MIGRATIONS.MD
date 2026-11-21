# Database migrations

From the repository root, configure `infra/.env` using [the template](../infra/.env.example).

```powershell
docker compose -f infra/docker-compose.yml config --quiet
docker compose -f infra/docker-compose.yml up -d --build backend
docker compose -f infra/docker-compose.yml run --rm -T migrate migrate status
```

The one-shot migration service must succeed before Compose starts the backend.
The current schema reports `version=7 dirty=false`. Status only reads the
database. An explicit upgrade uses the same service:

```powershell
docker compose -f infra/docker-compose.yml run --rm --build -T migrate migrate up
```

Reapplying the latest clean schema succeeds without changes. A dirty state
requires inspection and a backup before repair. Migration 000006 marks
existing databases with operator accounts as initialized.

Migration 000007 adds the delivery outbox. Upgrade backend and bot together
after applying it. Avoid mixing older bots that send directly with a backend
that delivers via the outbox. Existing messages are kept and classified as
unverified history. They are not enqueued for automatic replay.

For the local Vite panel, include both `-f infra/docker-compose.yml` and
`-f infra/docker-compose.dev.yml` to publish backend to loopback.
