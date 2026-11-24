# PostgreSQL and media snapshots

The Bash script requires Docker with Compose, GNU coreutils, and access to
the explicitly named Compose project. Use Linux or WSL with Docker Desktop
integration for the commands below.

1. Schedule a maintenance window and quiesce all writers. Stop the project’s
   backend, bot and retention containers. Leave PostgreSQL running. Ensure
   external writers and any one-off migration commands have finished.
2. Pass the same Compose project name and environment file used to start it.
   Use a new snapshot directory on protected storage.

   ```sh
   bash deploy/backup/backup.sh backup PROJECT /secure/compose.env /secure/snapshot
   ```

3. Re-enable the writers after the snapshot succeeds. Protect the database
   dump, media archives and their checksum manifest using access controls and
   storage encryption. Preserve the encryption key separately through the
   deployment’s secret storage. Retention of the application database does
   not expire copies in snapshot storage.

Restore only a trusted snapshot, using a **new isolated Compose project**.
The database needs to be empty. Start only the target PostgreSQL service
without running the migration service. Create empty Compose-labelled media
volumes for the target, using the same project name as the restore command.

```sh
project=restore-drill
env_file=/secure/restore.env
docker compose -p "$project" --env-file "$env_file" -f infra/docker-compose.yml up -d --wait postgres
for media in photos voice; do
  docker volume create --label "com.docker.compose.project=$project" \
    --label "com.docker.compose.volume=bot_$media" "${project}_bot_${media}"
done
bash deploy/backup/backup.sh restore "$project" "$env_file" /secure/snapshot
```

Verify the restored schema version. The database archive excludes role
grants. On the restored Compose project, apply any pending migrations and run
the `db-permissions` service before starting backend or retention. Then verify
operator access, session history and representative media files before
allowing traffic. The restore is
transactional for PostgreSQL. If media extraction or Docker fails after
the database restore, discard the isolated target and restore again into an
empty one. The script does not restart services or execute destructive
PostgreSQL commands.

The isolated drill created two PostgreSQL Compose projects, migrated the
source to schema version 7, and seeded a session and one fixture for each
media volume. After restoration, the target returned the same migration
state, session title and file bytes. A second restore into that target and
a restore with a modified archive both failed as expected. Repeat the
drill with the schema and build being delivered.
