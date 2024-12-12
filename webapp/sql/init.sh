#!/usr/bin/env bash

set -eux
cd $(dirname $0)

if [ "${ENV:-}" == "local-dev" ]; then
  exit 0
fi

if test -f /home/ishocon/env.sh; then
	. /home/ishocon/env.sh
fi

ISHOCON_DB_HOST=${ISHOCON_DB_HOST:-127.0.0.1}
ISHOCON_DB_PORT=${ISHOCON_DB_PORT:-3306}
ISHOCON_DB_USER=${ISHOCON_DB_USER:-ishocon}
ISHOCON_DB_PASSWORD=${ISHOCON_DB_PASSWORD:-ishocon}
ISHOCON_DB_NAME=${ISHOCON_DB_NAME:-ishocon3}

## To recreate user, run the following commands:
# mysql -u"$ISHOCON_DB_USER" \
# 		-p"$ISHOCON_DB_PASSWORD" \
# 		--host "$ISHOCON_DB_HOST" \
# 		--port "$ISHOCON_DB_PORT" \
# 		"$ISHOCON_DB_NAME" < 00-init.sql

mysql -u"$ISHOCON_DB_USER" \
		-p"$ISHOCON_DB_PASSWORD" \
		--host "$ISHOCON_DB_HOST" \
		--port "$ISHOCON_DB_PORT" \
		"$ISHOCON_DB_NAME" < 01-schema.sql

mysql -u"$ISHOCON_DB_USER" \
		-p"$ISHOCON_DB_PASSWORD" \
		--host "$ISHOCON_DB_HOST" \
		--port "$ISHOCON_DB_PORT" \
		"$ISHOCON_DB_NAME" < 02-data.sql
