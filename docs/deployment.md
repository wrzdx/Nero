# Deployment

[Back to README](../README.md) · [Frontend](frontend.md) · [Development](development.md)

The public deployment uses Nginx with HTTPS and Docker Compose. This is separate
from `make deploy`, which starts the local development container. The production
image in `deploy/Dockerfile` consumes a prebuilt Linux binary and static UI files;
Go, npm and Tailwind run on the development machine, not the VPS.

## Release deployed on 2026-10-05

Active release: `20261005-nero-ui`. Previous release retained for rollback:
`20260907-5d475a0-wsfix`. The update includes the Nero UI, username-based chat
inputs and user search. It required no new migrations; PostgreSQL remains at
version 3 with `dirty=false`.

Checks passed through the public HTTPS domain: pages and asset content hashes,
registration, HTML login cookies, profile/group views, username search,
WebSocket authentication, message create/edit/delete events and read state.
The app reported zero restarts after the update. The release was built from the
working tree; no Git commit or push was performed.

## Existing server

- Domain: `https://nero.wrzdx.tech`.
- SSH: `root@151.241.109.138`, port 22. Confirm the address before a future release.
- Server architecture: `x86_64`; build with `-Architecture amd64`.
- Root: `/srv/messenger`; release directories: `/srv/messenger/releases/<release-id>`.
- Secrets: `/srv/messenger/.env`, retained between releases.
- PostgreSQL data: `/srv/messenger/data`; logs: `/srv/messenger/logs`.
- Compose project: `messenger`; app listens on `127.0.0.1:5051` behind Nginx.

The application module is `github.com/wrzdx/Nero`, its entrypoint is `cmd/nero`,
and new releases use the `nero` image and binary. Existing infrastructure names
(`messenger` Compose project, database/user, and `/srv/messenger` paths) are
retained so the rename reuses the current database, network, and persistent data.
Changing those names requires a separate data migration.

Read the active image and release directory before updating:

```sh
docker inspect messenger-app-1 --format '{{.Config.Image}} {{index .Config.Labels "com.docker.compose.project.working_dir"}}'
docker ps --filter name=messenger
```

Do not use another release's `.env` or reinitialize the server's secrets. The
first-deployment helper `initialize.sh` deliberately preserves an existing `.env`.

## Build a release on Windows

From the repository root in PowerShell, with Go, npm and dependencies installed:

```powershell
npm.cmd ci
./scripts/build-release.ps1 -ReleaseId 20261005-nero -Architecture amd64
```

Choose a unique release ID each time. The script builds templ, Tailwind and asset
hashes, runs `go test ./...` and `go vet ./...`, then cross-compiles Go with
`GOOS=linux`, `CGO_ENABLED=0`. It packages the Linux binary, static files,
migrations, production Dockerfile/Compose, docs and a release manifest under
`out/releases/`. It excludes `.env`, database files, logs and private keys.

The `.tar.gz.sha256` sidecar verifies the transferred archive. `release.json`
records hashes, architecture, the base Git commit, and whether the working tree
contained uncommitted changes. Packaging a working tree does not commit or push it.

## Transfer and start

Record the old release ID and retain its image/directory for rollback. Copy the
archive and checksum to the server, using your chosen ID:

```powershell
scp out/releases/20261005-nero.tar.gz out/releases/20261005-nero.tar.gz.sha256 root@151.241.109.138:/srv/messenger/releases/
```

On the server:

```sh
set -eu
cd /srv/messenger/releases
sha256sum -c 20261005-nero.tar.gz.sha256
mkdir 20261005-nero
tar -xzf 20261005-nero.tar.gz -C 20261005-nero
cd 20261005-nero
export RELEASE_ID=20261005-nero
docker compose --env-file /srv/messenger/.env build app
docker compose --env-file /srv/messenger/.env up -d --no-deps app
```

The unchanged Compose project name keeps the existing database/network. Only the
app is recreated. Nginx continues to proxy to the same loopback port. The release
must include `web/static`; the production image copies it to `/app/web/static`
and sets `STATIC_DIR` accordingly.

This UI/username/search release has no new database migrations. The deployed
database must already have migration version 3 with `dirty=false`. Before future
schema changes, prepare a backup and a compatible rollback plan, then run the
existing migration service deliberately. Starting the application does not run
migrations automatically.

## Check the release

```sh
docker compose --env-file /srv/messenger/.env ps app
curl -fsS https://nero.wrzdx.tech/ >/dev/null
curl -fsS https://nero.wrzdx.tech/login >/dev/null
curl -fsS https://nero.wrzdx.tech/register >/dev/null
curl -i https://nero.wrzdx.tech/api/v1/users/me
```

The final request should return `401 invalid_token` without credentials. Check
that the HTML references `/static/css/app.css?v=<hash>` and that the referenced
CSS/JS return 200. User pages use `Cache-Control: no-store`; static assets use
`no-cache` and content hashes in the application CSS/JS URLs.

From the development machine, the API/WebSocket smoke check is:

```sh
go run ./deploy/smoke https://nero.wrzdx.tech
```

It checks public pages and CSS/JS content hashes, creates two temporary users,
checks HTML login cookies, profile/group pages and username search, then creates
a direct chat and checks WebSocket authentication and create/edit/delete events.
It advances the read marker, removes its message and anonymizes its users. The
empty chat/anonymized rows remain; it is not a zero-write health probe. JSON chat
inputs now use usernames, so run the current smoke code.

Also check the UI in a browser: login, user search, group member selection,
profile, mobile layout and realtime. After deployment, fully reload already-open
tabs: htmx navigation does not replace CSS/JS loaded by the old document.

## Roll back the app

Return to the recorded previous release directory, use its ID and recreate only
the app without rebuilding or pulling:

```sh
cd /srv/messenger/releases/<previous-release-id>
export RELEASE_ID=<previous-release-id>
docker compose --env-file /srv/messenger/.env up -d --no-deps --no-build --pull never app
```

Keep the previous image until checks have passed. This restores application code
and static files, not database data or schema. The current release needs no schema
rollback. Do not run `down -v`, delete `/srv/messenger/data`, print `.env`, or remove
the prior release as part of an ordinary update.
