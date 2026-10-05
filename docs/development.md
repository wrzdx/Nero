# Development

[Back to README](../README.md)

## Local setup

Use Go 1.26.4 (see `go.mod`), Node.js/npm, Docker with Compose, GNU Make, and a
POSIX-compatible shell. On Windows, run Make targets from Git Bash: several
recipes use `export`, shell conditionals, and line continuations.

```sh
cp .env.example .env
```

Edit the local file before starting. The sample credentials are for development;
do not deploy the example `JWT_SECRET` or database password. Do not commit `.env`.

```sh
make env-up
make env-port-forward
make migrate-up
npm ci
npm run build:ui
make run
```

PostgreSQL data lives under `out/pgdata`; application logs use `out/logs`.
The port forwarder exposes PostgreSQL on `127.0.0.1:5432`. `make run` sets
`POSTGRES_HOST=localhost`, loads the Makefile environment, runs `go mod tidy`,
and starts the application. It can therefore update module files.

The application does not load `.env` itself. When running `go run ./cmd/nero`
directly, export the configuration into the process environment first.

## Configuration

| Variable | Purpose / current example |
| --- | --- |
| `HTTP_ADDR` | HTTP listen address; `:5050` |
| `HTTP_SHUTDOWN_TIMEOUT` | HTTP shutdown timeout; `30s` |
| `HTTP_ALLOWED_ORIGINS` | HTTP CORS origins; `http://localhost:5050` |
| `JWT_SECRET` | Required signing secret; replace the sample value |
| `AUTH_ACCESS_TOKEN_TTL` | Access-token lifetime; `15m` |
| `AUTH_SESSION_TTL` | Session lifetime; `.env.example` sets `1h` |
| `ENVIRONMENT` | `development`; production enables Secure refresh cookies |
| `TIME_ZONE` | `Europe/Moscow` in the example; application default is UTC |
| `STATIC_DIR` | Static UI files; defaults to `./web/static` |
| `POSTGRES_HOST` | Required; Make sets localhost for local runs/tests |
| `POSTGRES_PORT` | Defaults to `5432` |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Database credentials and name |
| `POSTGRES_TIMEOUT` | Database operation timeout; `10s` |
| `POSTGRES_TEST_DB` | Dedicated integration database; `messenger_test` |
| `LOGGER_LEVEL`, `LOGGER_FOLDER` | Logging level and output directory |

HTTP CORS configuration is not a WebSocket origin allowlist. The WebSocket
handler uses the library's default same-origin acceptance policy.

### Local application container

`make deploy` starts the local `messenger` Compose service. It explicitly uses
`env_file: .env`, so the required JWT secret and other configuration reach the
container. The image builds Tailwind CSS, asset hashes, templ views and Go;
the runtime includes the static files. PostgreSQL and migrations remain separate
steps. The target does not deploy to the remote server.

For remote releases use the [deployment guide](deployment.md). The production
image consumes a prebuilt Linux binary plus `web/static`; it does not compile Go
or run Node on the VPS.

## Tests

```sh
make test-unit
# A particular package:
make test-unit action=./internal/features/messages/service
```

The untagged suite includes WebSocket tests using a local test HTTP server;
it does not require PostgreSQL. The race detector needs a supported Go platform
and C toolchain with CGO enabled:

```sh
go test -race ./internal/features/messages/service ./internal/features/realtime/transport/ws
```

Integration tests use a real PostgreSQL database and the `integration` build tag:

```sh
make env-up
make env-port-forward
make test-env-up
make test-migrate-up
make test-integration
make test-env-down
```

Use a **dedicated disposable database**, never production data. The final command
drops the configured test database with `FORCE`. Creation of an existing database
does not reset it; when validating changed initial migrations, use a fresh test
database. Already-applied migrations are not reapplied when their files change.

For compilation only, without executing database tests:

```sh
go test -tags=integration ./... -run '^$'
```

An `out/go-build-cache` directory may come from setting `GOCACHE` for a local test
process. It is a build cache, not application output or part of the database.

## Generated files

Build the UI with `npm run build:ui` after changing templates, styles or browser
scripts. `*_templ.go`, `web/static/css/app.css` and `assets_generated.go` are
generated outputs. The last file holds content hashes used in CSS/JS URLs;
`make css-build` also refreshes it. See [frontend notes](frontend.md) for watch
mode, page routes, authentication and realtime behavior.

Mocks are generated with Mockery (the project has used v3.7.1):

```sh
mockery
```

`.mockery.yaml` discovers feature interfaces recursively and excludes repository
subpackages. Regenerate after contract changes, then inspect the diff and run tests.

The database diagram is maintained as `docs/database.mmd` and rendered to SVG:

```sh
make diagram-db
```

This target uses Node/npm and `@mermaid-js/mermaid-cli@11.16.0`. On Windows it uses
Chrome at the Makefile's default path; override with
`make diagram-db MERMAID_BROWSER="C:/path/to/chrome.exe"` if needed. The SVG is
embedded in README so GitHub's Mermaid renderer version does not control it.

`make swagger-gen` is a tooling target, not a maintained API specification.
The current API reference is [http-api.md](http-api.md); there is no committed,
complete OpenAPI document or Swagger UI to treat as the contract.

## Before calling a deployment ready

- Start from a fresh database and apply every migration.
- Execute the integration suite, not just compilation.
- Exercise registration, two accounts, direct/group messaging, read state, and
  socket reconnects through the running application's actual middleware stack.
- Check the release's container startup and graceful shutdown.
- Define TLS, secret handling, rate limits, monitoring, and backup/restore for
  the target environment. These are not provided by a successful unit test run.
