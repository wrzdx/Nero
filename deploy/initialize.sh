#!/bin/sh
# First deployment only. Generate secrets on the server, never in terminal output.
set -eu
root=/srv/messenger
if [ -e "$root/.env" ]; then
    echo 'Existing environment preserved; initialization skipped.'
    exit 0
fi
install -d -m 700 "$root"
umask 077
set -C
{
    printf 'MESSENGER_ROOT=%s\n' "$root"
    printf 'APP_ORIGIN=https://nero.wrzdx.tech\n'
    printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"
    printf 'JWT_SECRET=%s\n' "$(openssl rand -hex 64)"
} > "$root/.env"
echo 'Server environment initialized (secrets not displayed).'
