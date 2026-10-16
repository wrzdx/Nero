# Deployment

[Back to README](../README.md) · [Development](development.md) · [Frontend](frontend.md)

Nero uses the root multi-stage `Dockerfile` to build UI assets, templ views and
the Go binary. GitHub Actions publishes the resulting Linux/amd64 image to
`ghcr.io/wrzdx/nero`. The VPS pulls the published image by digest.
`make deploy` is a separate local development command.

## What is implemented

- PRs targeting `main` run tests, migrations, vet and a Docker build.
- Pushes to `main` and `12factor` run checks, then publish the image and a release
  archive. Updating an open PR produces both a PR run and a push run; only the
  push run publishes.
- The image carries the source commit in `org.opencontainers.image.revision`.
- The packager takes deployment files from that Git commit and writes build
  metadata and checksums. Server secrets are outside the archive.
- Server configuration snapshots, activation and rollback are manual procedures
  below. They have not yet been verified by an end-to-end server rehearsal.

The first confirmed GHCR publication was commit
`e0408fde07ddadb1035869791aa02f358439dcd7`, image digest
`sha256:0cc9d507ef6525f0b846ab47100cff3ba956d06990e06f37ad95ff61e8f7dbe6`,
in [CI run 38937195361](https://github.com/wrzdx/Nero/actions/runs/38937195361).
That commit predates the new configuration template and release packager.
Use a subsequent successful publication for the archive procedure below.
Publishing an image does not deploy it to the VPS.

## Obtain a release archive

After a successful push run, download the `nero-release-<commit>-<attempt>`
artifact from its Actions page. It contains `<release-id>.tar.gz` and its
`.tar.gz.sha256` sidecar. The Docker action's `.dockerbuild` artifact is a build
record for inspection in Docker Desktop.

The release archive contains:

```text
compose.yaml
migrations/*.up.sql
migrations/*.down.sql
runtime.example.env
build.json
checksums.sha256
```

`build.json` records the release ID, full source commit, image reference pinned
by digest, packaging time and CI run URL. `checksums.sha256` covers every other
file in the archive, including `build.json`. The sidecar covers the archive.
Retain the archive, checksum and referenced images for releases you may restore;
Actions artifact retention is finite.

### Package manually

Requires Git and Python 3.10+, with no third-party Python packages. Run from the
repository checkout. Copy the commit and full image reference from the **same**
successful CI run, and choose an unused release ID:

```sh
python3 scripts/package-release.py \
  --commit "$SOURCE_COMMIT" \
  --image "$APP_IMAGE" \
  --release-id "$RELEASE_ID" \
  --build-url "$BUILD_URL"
```

In PowerShell, use `python scripts/package-release.py` and PowerShell variables,
for example `--commit $SourceCommit --image $Image --release-id $ReleaseId`.
The former `scripts/build-release.ps1` local compiler has been replaced by this
packager. Go, Node and Docker are not required for manual packaging.

`--commit` and `--image` are required. The image must contain `@sha256:` followed
by 64 lowercase hexadecimal digits. Optional arguments are `--build-url`,
`--release-id` (defaults to UTC timestamp plus commit), and `--output-dir`
(defaults to `out/releases/`). Existing archive/checksum files are never replaced.

The script reads committed Git blobs rather than working-tree files. Local
edits, untracked files and `.env` are excluded. CI supplies its own commit and
build output digest automatically. A manual caller is responsible for using
the matching pair: the packager does not query the registry to verify image
labels or signatures. Checksums detect changed bytes; they are not signatures.

## Production configuration

The application reads process environment variables; it does not load env files.
Compose maps `runtime.env` inputs to the application, PostgreSQL and migrator.
Pass the selected file explicitly with `--env-file ./runtime.env`.
The committed `runtime.example.env` is a template, not a secret file.

### Required values

| Input | Meaning |
| --- | --- |
| `APP_IMAGE` | Exact `image` from this archive's `build.json`, including digest |
| `APP_ORIGIN` | Public origin, for example `https://nero.wrzdx.tech` |
| `APP_PORT` | Host loopback port forwarded to container port 5050; existing Nginx uses 5051 |
| `MESSENGER_ROOT` | Absolute persistent-data root; existing deployment uses `/srv/messenger` |
| `JWT_SECRET` | Existing signing key, or a generated key for a fresh deployment |
| `POSTGRES_PASSWORD` | Password matching the actual database role |

Do not use `example.env` development credentials in production. On an existing
server, preserve the signing key and actual database credentials. Changing
`POSTGRES_PASSWORD`, `POSTGRES_USER` or `POSTGRES_DB` in an env file does not
change roles, passwords or databases in an initialized PostgreSQL data directory.

### Optional overrides and production defaults

| Input | Compose default | Meaning / constraint |
| --- | --- | --- |
| `POSTGRES_HOST` | `db` | Database host, shared by app and migrator |
| `POSTGRES_PORT` | `5432` | Client connection port; does not change the embedded server's listen port |
| `POSTGRES_USER` | `messenger` | App/migrator role and fresh database initialization user |
| `POSTGRES_DB` | `messenger` | App/migrator database and fresh database initialization name |
| `POSTGRES_TIMEOUT` | `10s` | Positive startup connection/ping timeout for Nero |
| `POSTGRES_SSLMODE` | `disable` | Nero accepts `disable` and `verify-full` |
| `POSTGRES_SSLROOTCERT` | empty | CA path **inside** each client container; required by Nero for `verify-full` |
| `TIME_ZONE` | `UTC` | IANA time zone, e.g. `Europe/Moscow` |
| `LOGGER_LEVEL` | `INFO` | Log level; logs go to stdout |
| `AUTH_ACCESS_TOKEN_TTL` | `15m` | Access token lifetime |
| `AUTH_SESSION_TTL` | `24h` | Refresh-session lifetime |
| `HTTP_SHUTDOWN_TIMEOUT` | `30s` | Application shutdown deadline |
| `APP_STOP_GRACE_PERIOD` | `35s` | Docker shutdown grace; keep above the application deadline |
| `APP_MEMORY_LIMIT` | `256m` | App container memory limit |
| `APP_CPU_LIMIT` | `1.0` | App CPU quota |
| `GOMEMLIMIT` | `192MiB` | Soft Go memory limit; leave headroom below the container limit |
| `DB_MEMORY_LIMIT` | `384m` | PostgreSQL container memory limit |
| `DB_CPU_LIMIT` | `1.0` | PostgreSQL CPU quota |

These defaults live in Compose. Commented entries in the template document
overrides; uncommenting an entry makes it an explicit setting. Go also has
defaults for standalone execution, including logger level `DEBUG`.

Production Compose explicitly sets `ENVIRONMENT=production`, `HTTP_ADDR=:5050`
and `STATIC_DIR=/app/web/static`, and binds the host port to `127.0.0.1`.
Go selects `GOMAXPROCS` automatically. The migrator receives standard `PG*`
variables and uses `postgres://` to select the PostgreSQL driver, avoiding manual
URL construction with credentials.

The base Compose launches the embedded PostgreSQL service and does not configure
server TLS or mount a CA file. `verify-full` requires a TLS-capable database and
a release-specific Compose override mounting the CA read-only into both `app`
and `migrate` at the configured path. An external-only database topology also
needs a Compose override for the embedded `db` dependency. These paths have not
yet been deployment-tested; setting a path variable alone is insufficient.

## Server layout and configuration version

Last recorded VPS layout (2026-10-05): domain `nero.wrzdx.tech`, SSH
`root@151.241.109.138`, root `/srv/messenger`, PostgreSQL data under
`/srv/messenger/data`, Compose project `messenger`, Nginx upstream
`127.0.0.1:5051`. Confirm the current server before an update:

```sh
docker inspect messenger-app-1 --format '{{.Config.Image}} {{index .Config.Labels "com.docker.compose.project.working_dir"}}'
docker ps --filter name=messenger
```

Retain the project name, data path and existing database identity. A unique
release directory contains the extracted archive, `runtime.env`, and a final
`release.json`. Its directory ID is also the configuration snapshot version.
After activation, do not edit its files. A configuration-only change creates a
new release ID, even if the commit and image digest stay the same.

Transfer the archive and sidecar to `/srv/messenger/releases/`. Set `RELEASE_ID`
to the downloaded archive's ID, then prepare a new directory on the server:

```sh
set -eu
umask 077
cd /srv/messenger/releases
sha256sum -c "$RELEASE_ID.tar.gz.sha256"
mkdir "$RELEASE_ID"
tar -xzf "$RELEASE_ID.tar.gz" -C "$RELEASE_ID"
cd "$RELEASE_ID"
sha256sum -c checksums.sha256
```

### Prepare the configuration manually

No interactive configuration script is needed for this step. Start with a copy
of the previous settings and edit the new file. Values that remain valid, such
as the database password and signing key, can be reused between releases.

From the new release directory, choose exactly one source:

| Situation | `CONFIG_SOURCE` |
| --- | --- |
| Update from a release using this layout | `/srv/messenger/releases/<previous-release-id>/runtime.env` |
| First transition from the older deployment | `/srv/messenger/.env` |
| Fresh deployment with no previous settings | `./runtime.example.env` |

Set `CONFIG_SOURCE` to the selected file, then copy it. The guard deliberately
refuses to replace an existing `runtime.env`, including a symbolic link:

```sh
set -eu
umask 077
CONFIG_SOURCE='/srv/messenger/releases/<previous-release-id>/runtime.env'
test -f "$CONFIG_SOURCE"
test ! -e ./runtime.env
test ! -L ./runtime.env
cp -- "$CONFIG_SOURCE" ./runtime.env
chmod 600 ./runtime.env
```

Open **the new** `runtime.env` in your editor. For each setting, either keep the
copied value or deliberately enter its replacement:

- Set `APP_IMAGE` to the exact `image` in the new archive's `build.json`.
- Preserve `MESSENGER_ROOT` and the existing database identity and credentials.
  Set `APP_PORT=5051` when retaining the existing Nginx upstream.
- Preserve `JWT_SECRET` unless signing-key rotation is an intentional part of
  the update.
- Review the origin and optional settings. For a fresh deployment, fill every
  required value in the template before validation.

Do not print or commit `runtime.env`. After the release is activated, retain it
unchanged; subsequent configuration changes belong in a new release directory.
Restarting or rolling back uses the saved file rather than creating it again.

The legacy `deploy/initialize.sh` preserves or creates the old server `.env`;
it does not assemble the new runtime snapshot or set `APP_IMAGE`/`APP_PORT`.

### Record the configured release

Prepare this private metadata file as `release.json`, replacing the example ID:

```json
{
  "releaseId": "20261016-001",
  "buildFile": "build.json",
  "configVersion": "20261016-001",
  "configFile": "runtime.env"
}
```

The `buildFile` supplies the source commit and image digest; the config reference
selects this release's private snapshot. Never upload `runtime.env` or copy it
into a public artifact. Avoid exporting deployment variables in the shell:
shell variables override `--env-file` during Compose interpolation.

## Activate and check

From the selected release directory, using its `runtime.env`:

```sh
docker compose --env-file ./runtime.env -f ./compose.yaml config --quiet
docker compose --env-file ./runtime.env -f ./compose.yaml pull app
```

For a fresh embedded database, start `db` and wait for its healthcheck. For an
existing deployment, retain the running database. If migrations are needed,
prepare a backup and decide schema compatibility first, then run:

```sh
docker compose --env-file ./runtime.env -f ./compose.yaml --profile tools run --rm --no-deps migrate
```

Use SQL from this release's `migrations/` and the same runtime configuration.
Migration failure stops the update; do not continue to activate the app. Record
the resulting database version and dirty state. App startup does not apply SQL.

```sh
docker compose --env-file ./runtime.env -f ./compose.yaml up -d --no-deps --no-build --pull never app
docker compose --env-file ./runtime.env -f ./compose.yaml ps app
docker compose --env-file ./runtime.env -f ./compose.yaml logs --tail=100 app
curl -fsS https://nero.wrzdx.tech/ >/dev/null
curl -fsS https://nero.wrzdx.tech/login >/dev/null
curl -i https://nero.wrzdx.tech/api/v1/users/me
```

The unauthenticated API request should return `401 invalid_token`. Check static
assets, login, sending messages and WebSockets. Match `APP_PORT` with the Nginx
upstream if changing the existing port. Keep the previous directory and image.

For a fuller check from a development checkout, `go run ./deploy/smoke
https://nero.wrzdx.tech` creates test accounts and a chat, exercises messages and
WebSockets, then anonymizes its users. It writes data; the empty chat and
anonymized rows remain.

## Roll back the app

Enter the previous retained release directory and use **its** `runtime.env`:

```sh
docker compose --env-file ./runtime.env -f ./compose.yaml up -d --no-deps --no-build --pull never app
```

This selects the previous app image and configuration without rebuilding or
restarting PostgreSQL. It does not restore database data/schema or external
Nginx configuration. Do not run down migrations automatically. Database
compatibility and backup restoration are separate decisions.

Before declaring the release procedure complete, rehearse A -> B -> A on a test
deployment and verify the image digest, runtime settings, HTTP and WebSockets.
Pinning the PostgreSQL and migrator images by digest is also pending; their
current version tags do not provide immutable identities for those tools.

See [V: Build, release, run](https://12factor.net/build-release-run),
[XII: Admin processes](https://12factor.net/admin-processes), and
[Compose interpolation](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/).
