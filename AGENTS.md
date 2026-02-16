# AGENTS.md

## Project Overview

FerroDB (ferro) is a Go CLI tool for managing database migrations with a full audit trail. It uses a plugin-based driver system to support any database — currently a PostgreSQL adapter is implemented. The binary is named `krab` (legacy) but the project is branded as `ferro`/`ferrodb`. Module path: `github.com/qbart/ferrodb`.

The project has two architectures:
- **`ferro/`** — the active system using YAML `.fyml` config files, append-only audit logging, and `urfave/cli/v3`
- **`krab/`** — the legacy system using HCL config, `schema_migrations` table, and `mitchellh/cli`. This code no longer compiles (missing internal packages). Retained as reference only — **`krab/` is obsolete and will be removed in the future**, along with its dependencies (`krabdb/`, `web/`, `views/`, legacy test fixtures).

Do not add features to or fix bugs in `krab/`, `krabdb/`, `web/`, or `views/`. All development should target the `ferro/` package and its sub-packages.

## Project Structure

```
ferro/              # Active architecture
  app.go            # CLI entry point, all command definitions (urfave/cli/v3)
  config/           # YAML config parsing, domain types, checksum
  plugin/           # Driver interfaces (Driver, DriverConnection, DriverQuery)
  run/              # Execution engine (Runner, Builder, Navigator, Migrator, Generator)
  spec/             # Integration tests (full CLI + real PostgreSQL)

plugins/            # Driver implementations
  postgresql.go     # Full PostgreSQL driver (pgx/v5)
  null_driver.go    # No-op driver (returns ErrDriverNotSelected)
  sqlite_driver.go  # Stub (not implemented)
  testcontainers/   # Docker-based PostgreSQL for testing
  registry.go       # Maps driver names to implementations

krab/               # OBSOLETE — legacy HCL-based system, will be removed
krabdb/             # OBSOLETE — legacy DB abstraction, will be removed

fmtx/               # Colored terminal output utilities
tpls/               # Go text/template rendering + faker data generation
web/                # OBSOLETE — legacy web UI, will be removed
views/              # OBSOLETE — legacy templ templates, will be removed
res/                # SVG logos and branding assets
test/fixtures/      # Legacy HCL test fixtures
```

## Configuration Format (.fyml)

Migrations use a Kubernetes-inspired YAML format. Multiple resources per file, separated by `---`.

```yaml
apiVersion: migrations/v1
kind: Migration
metadata:
  name: create_users
spec:
  version: "v1"
  run:
    up:
      sql: CREATE TABLE users (id BIGINT PRIMARY KEY);
    down:
      sql: DROP TABLE users;
---
apiVersion: migrations/v1
kind: MigrationSet
metadata:
  name: public
spec:
  namespace:
    schema: public
  migrations:
    - create_users
---
apiVersion: drivers/v1
kind: Driver
metadata:
  name: local
spec:
  driver: postgresql
  config:
    url: postgres://user:pass@localhost:5432/mydb?sslmode=disable
```

Supported kinds: `Migration`, `MigrationSet`, `Driver`.

SQL can be inline (`sql:`) or reference external files (`file: ./path.sql`), resolved relative to the .fyml file.

## Execution Flow

1. **CLI** (`ferro/app.go`) — parses flags, constructs a `run.Command*` struct
2. **Runner** (`ferro/run/runner.go`) — orchestrates: load config → resolve driver → resolve migration set
3. **Navigator** (`ferro/run/navigator.go`) — manages DB lifecycle: connect → ensure audit tables → acquire lock → execute → release lock → disconnect
4. **Migrator** (`ferro/run/migrator.go`) — business logic for up/down/status/fix/audit
5. **Plugin** (`plugins/postgresql.go`) — executes SQL against PostgreSQL via pgx/v5

## Audit Log System

The active system uses an append-only audit log instead of a simple `schema_migrations` table:

- **`_ferro_audit_log`**: `id`, `applied_at`, `event`, `data` (JSONB), `metadata` (JSONB)
- **`_ferro_audit_lock`**: row-level locking (insert to lock, delete to unlock)
- Events: `migration.up.started`, `migration.up.completed`, `migration.up.failed`, `migration.up.fixed`, and equivalent `down.*` events
- State is computed by replaying all log entries
- Checksums (CRC32 of YAML chunk bytes) are stored in audit log metadata

## CLI Commands

| Command | Description |
|---------|-------------|
| `ferro init` | Scaffold `.ferro/` directory with sample files |
| `ferro validate` | Parse and validate configuration |
| `ferro migrate init` | Generate a new timestamped migration file |
| `ferro migrate up -d <driver> -s <set>` | Apply all pending migrations |
| `ferro migrate down -d <driver> -s <set> -v <version>` | Rollback a specific migration |
| `ferro migrate status -d <driver> -s <set>` | Show migration status |
| `ferro migrate audit -d <driver> -s <set>` | Show audit log history |
| `ferro migrate fix up -d <driver> -s <set> -v <version>` | Mark a failed UP as fixed |
| `ferro migrate fix down -d <driver> -s <set> -v <version>` | Mark a failed DOWN as fixed |

## Plugin/Driver System

Interfaces defined in `ferro/plugin/driver.go`:
- `Driver` — `Connect` / `Disconnect`
- `DriverConnection` — audit table management, locking, query execution
- `DriverQuery` — `Begin` / `Commit` / `Rollback` / `Exec` / `Query`

Registered drivers (in `plugins/registry.go`): `postgresql`, `testcontainer/postgresql`, `sqlite` (stub), `null`.

## Testing

### Running Tests

```bash
# All tests (requires PostgreSQL via docker-compose)
make test

# Quick: only ferro/spec/ tests (no CGO)
make quicktest

# Docker Compose for test DB
docker compose up -d
```

Test PostgreSQL instances (from `docker-compose.yml`):
- Dev: `postgres://krab:secret@localhost:5432/krab`
- Test: `postgres://test:test@localhost:5433/test`

### Test Architecture

- **Integration tests** (`ferro/spec/`): use `cliMock` harness that creates an isolated PostgreSQL database per test, writes .fyml files to a temp dir, runs the full CLI, and asserts on stdout/stderr/audit log/data
- **Unit tests** (`ferro/run/builder_test.go`): test config parsing and validation without a database
- **Runner tests** (`ferro/run/runner_test.go`): integration tests with real PostgreSQL
- Test assertions use `github.com/qbart/expecto` and `github.com/stretchr/testify`

### Key Test Helpers

- `cliMock.RandomDatabase()` — provisions a unique test database
- `cliMock.Files()` — writes .fyml fixture files
- `cliMock.SetTime()` — injects a mock clock for deterministic timestamps
- `cliMock.AssertRun()` / `AssertNotRun()` — runs CLI and checks exit code
- `cliMock.Audit()` — inspects audit log entries
- `cliMock.Data()` — asserts table existence

## Build

```bash
make build          # -> bin/krab
make install        # install air + templ dev tools
make gen            # templ generate (for web UI)
make docker_build   # Docker image
make changelog      # git-chglog
```

Binary embeds SVG assets and `.fyml.tpl` templates via `//go:embed` directives in `main.go`.

Version info is injected via ldflags into `krab.InfoVersion`, `krab.InfoCommit`, `krab.InfoBuildDate`.

## Key Conventions

- **Config directory**: determined by `FERRO_DIR` env var, defaults to current working directory
- **Migration versions**: timestamp format `YYYYMMDD_HHMMSS`
- **Table name prefixing**: audit tables are prefixed per migration set namespace
- **No CGO**: the project builds with `CGO_ENABLED=0` (except SQLite which is stubbed)
- **Error output**: uses `fmtx` package for colored terminal output (red errors, green success, cyan info)
- **ANSI stripping**: `fmtx.StripANSI()` used in tests to compare output cleanly

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `FERRO_DIR` | Root directory for .fyml config files (default: cwd) |
| `DATABASE_URL` | PostgreSQL connection string (legacy/web) |
| `KRAB_ENV` | Environment selector (legacy) |
| `KRAB_DIR` | Config directory (legacy, Docker default: `/etc/krab`) |
| `KRAB_AUTH` | Web UI auth mode (`none` or `basic`) |
