#!/bin/sh
set -e

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