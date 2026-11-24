# Project handover

[Русская версия](PROJECT_HANDOVER.ru.md)

Start with the [documentation index](README.md) and review
[data handling](PRIVACY_AND_DATA_HANDLING.md) before sharing a demo.
Operators can access conversation content. Use synthetic sessions and
fresh fixture credentials on a separate Compose project.

## Demo and acceptance

Follow the [deployment guide](DEPLOYMENT.md) from a clean checkout. Verify
migration status, operator sign-in and TOTP, session invitations with a
fixture bot, and representative media previews. Confirm backup and restore
in an isolated project using the [recovery guide](../deploy/backup/README.md).
Record the commit, environment and observed results. Check the backend,
bot, frontend, image and migration CI jobs for that same commit.

An independent clean-checkout acceptance run is pending. Record any
configuration or instructions that the reviewer needs outside this guide.

## Ownership transfer

Confirm ownership and access for the repository, package and image
registries, hosting, domain and certificate renewal, Telegram bot and
backup storage. Transfer secret locations and recovery responsibilities
through an approved private channel. Verify source, dependency and
contributor rights.

## Source delivery

Create the delivery from a reviewed commit using `git archive`.
The repository's export rules omit internal planning, audit and rollout
notes from that archive. Verify the file list before sharing and keep
customer data, live configuration and credentials outside the package.
