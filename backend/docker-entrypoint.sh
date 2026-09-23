#!/bin/sh
set -eu

# Named Docker volumes are created as root. Grant the unprivileged API process
# access only to its private storage before dropping privileges.
mkdir -p /app/data/uploads
chown -R app:app /app/data

if [ "$1" = "/app/server" ]; then
  exec su-exec app "$@"
fi

exec "$@"
