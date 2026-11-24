# Database migrations

Use the Compose project and protected environment file for the target
deployment. See [configuration](ENV_VARS.md) and the
[environment template](../infra/.env.example). The following example
uses an isolated project and runs from the repository root.

```powershell
$compose = @('-p', 'anonymus-acceptance', '--env-file', 'C:/private/acceptance.env', '-f', 'infra/docker-compose.yml')
docker compose @compose config --quiet
docker compose @compose up -d --build --wait backend
docker compose @compose run --rm -T --no-deps migrate migrate status
```

The one-shot migration service must succeed before Compose starts the backend.
The current schema reports `version=13 dirty=false`. Status only reads the
database. An explicit upgrade uses the same service:

```powershell
docker compose @compose run --rm --build -T migrate migrate up
```

Reapplying the latest clean schema succeeds without changes. A dirty state
requires inspection and a backup before repair. Migration 000006 marks
existing databases with operator accounts as initialized.

Migration 000007 adds the delivery outbox. Upgrade backend and bot together
after applying it. Avoid mixing older bots that send directly with a backend
that delivers via the outbox. Existing messages are kept and classified as
unverified history. They are not enqueued for automatic replay.

Migration 000008 records whether an operator must change their password.
Existing invited accounts are marked for a change. Existing active
accounts keep their current sign-in flow. Apply the migration before
updating the backend and panel together.

Migration 000009 adds operator recovery-code digests. Existing operators
can issue their own codes after sign-in using their password and TOTP.

Migration 000010 adds short-lived operator linking challenges. Existing
Telegram bindings remain in place on upgrade or rollback. Apply the
migration before updating the backend, bot and panel together. Retention
purges expired challenges, with a count-only preview in dry-run mode.

Migration 000011 adds participant integrity constraints. Follow the
[schema 11 upgrade procedure](DEPLOYMENT.md#postgresql-roles-and-upgrade-to-schema-11)
to check existing data and provision runtime roles.

Migration 000012 adds assignment and session links to incoming requests.
Existing new and processed requests keep their status. Upgrade the database
before deploying the updated backend. Upgrade the bot before relying on
the new group notifications, then deploy the panel.
The down migration resets assigned requests to new and discards their
assignment and session links. Keep schema 12 during an application-only
rollback unless recovery from a verified backup is part of the plan.

Migration 000013 stores the intended recipient and retry key for operator
replies. Finish or review pending operator sends before upgrading the
backend. The new worker marks older unaddressed operator jobs failed
when it claims them. Stop writers for the schema change and start the
new backend before the updated panel. A binary rollback to an older
worker loses recipient enforcement. Keep the worker stopped during such
a rollback until pending operator jobs can be reviewed.

For the local Vite panel, include both `-f infra/docker-compose.yml` and
`-f infra/docker-compose.dev.yml` to publish backend to loopback.
