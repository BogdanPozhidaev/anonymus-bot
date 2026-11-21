#!/usr/bin/env bash
set -euo pipefail
umask 077

die() { printf '%s\n' "$*" >&2; exit 1; }

[[ $# == 4 ]] || die "Usage: bash deploy/backup/backup.sh backup|restore PROJECT COMPOSE_ENV_FILE DIRECTORY"
action=$1 project=$2 env_file=$3 directory=$4
[[ $action == backup || $action == restore ]] || die "Expected backup or restore"
[[ $project =~ ^[a-z0-9][a-z0-9_-]*$ ]] || die "Invalid Compose project name"
[[ -f $env_file ]] || die "Compose environment file not found"
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
compose=(docker compose -p "$project" --env-file "$env_file" -f "$root/infra/docker-compose.yml")
archive_image=alpine:3.19

for state in running restarting paused; do
  services=$("${compose[@]}" ps --all --status "$state" --services)
  while IFS= read -r service; do
    case "$service" in
      ''|postgres|redis|nginx|panel) ;;
      *) die "Quiesce $service in project $project before taking or restoring a snapshot" ;;
    esac
  done <<< "$services"
done

for media in photos voice; do
  volume="${project}_bot_${media}"
  label=$(docker volume inspect --format '{{ index .Labels "com.docker.compose.project" }}:{{ index .Labels "com.docker.compose.volume" }}' "$volume")
  [[ $label == "$project:bot_$media" ]] || die "Volume $volume does not belong to project $project"
done

database() {
  "${compose[@]}" exec -T postgres sh -ec '
    export PGPASSWORD="$POSTGRES_PASSWORD"
    tool=$1
    shift
    exec "$tool" -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"
  ' database "$@"
}

volume_command() {
  local media=$1 mode=$2
  shift 2
  local mount="type=volume,src=${project}_bot_${media},dst=/media,volume-nocopy"
  if [[ $mode == ro ]]; then mount+=,readonly; fi
  docker run --rm -i --network none --read-only --mount "$mount" "$archive_image" "$@"
}

database psql -XAtq -v ON_ERROR_STOP=1 -c 'SELECT 1' >/dev/null
files=(database.dump photos.tar.gz voice.tar.gz)

if [[ $action == backup ]]; then
  [[ ! -e $directory ]] || die "Backup directory must not exist"
  mkdir -m 700 -- "$directory"
  database pg_dump --format=custom --no-owner --no-acl > "$directory/database.dump"
  for media in photos voice; do
    volume_command "$media" ro tar -C /media -czf - . > "$directory/$media.tar.gz"
  done
  (cd "$directory" && sha256sum "${files[@]}" > SHA256SUMS)
  printf 'Backup completed in %s\n' "$directory"
  exit 0
fi

[[ -d $directory && -f $directory/SHA256SUMS ]] || die "Complete backup directory required"
(
  cd "$directory"
  sha256sum "${files[@]}" | cmp -s - SHA256SUMS
) || die "Backup checksum mismatch"

objects=$(database psql -XAtq -v ON_ERROR_STOP=1 -c \
  "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'")
[[ $objects == 0 ]] || die "Restore requires an empty database. Start only PostgreSQL and skip migrations on the target"
for media in photos voice; do
  contents=$(volume_command "$media" ro sh -ec 'find /media -mindepth 1 -print -quit')
  [[ -z $contents ]] || die "Restore requires an empty $media volume"
  volume_command "$media" ro tar -tzf - < "$directory/$media.tar.gz" > /dev/null
done

database pg_restore --exit-on-error --single-transaction --no-owner --no-acl < "$directory/database.dump"
for media in photos voice; do
  volume_command "$media" rw tar -C /media -xzf - < "$directory/$media.tar.gz"
done
printf 'Restore completed in project %s\n' "$project"
