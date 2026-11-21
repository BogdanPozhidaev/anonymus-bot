#!/bin/sh
set -e

case "$RETENTION_RUN_HOUR" in
    0[0-9]|1[0-9]|2[0-3]) ;;
    *) echo "RETENTION_RUN_HOUR must be a two-digit UTC hour (00–23)" >&2; exit 1 ;;
esac

echo "Retention cron started, running once per day at configured hour"

while true; do
    current_hour=$(date +%H)

    if [ "$current_hour" = "$RETENTION_RUN_HOUR" ]; then
        echo "$(date -Iseconds) Running retention job..."
        /retention
        echo "$(date -Iseconds) Retention job finished, sleeping until next day"
        sleep 3600
    fi

    sleep 60
done
