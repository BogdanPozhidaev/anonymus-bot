#!/bin/sh
set -eu

for dir in /data/photos /data/voice; do
    # The API, bot and retention share this numeric identity for media access.
    # Limit the ownership change to volume roots and their regular files.
    chown 10001:10001 "$dir"
    chmod 0750 "$dir"
    find "$dir" -xdev -maxdepth 1 -type f -exec chown -h 10001:10001 {} +
done
