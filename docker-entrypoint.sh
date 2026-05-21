#!/bin/sh
set -eu
if [ "$(id -u)" = "0" ]; then
	mkdir -p /data /backup
	chown -R app:app /data /backup
	exec su-exec app:app "$@"
fi
exec "$@"
