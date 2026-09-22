# thesis-rat-server

Production HTTPS/tunnel deployment: [transport security](docs/transport-security.md).

## Configuration

Copy `.env.example` to the local ignored `.env`. This server deliberately reads
configuration from that file, not exported OS variables. Set `DATABASE_URL`, or
set `DB_PASSWORD` together with the other `DB_*` values. Missing database
configuration fails at startup; there is no built-in password.
Use the environment template only as a template, never commit live credentials.
Database startup errors do not print the connection string.

The current repository has no complete production schema bootstrap. Obtain a
schema-only baseline matching the deployed database from its administrator and
check which migrations have already run. Phase 1 changes no production schema.
Do not re-run `migrations/migrate-register.sql` when `agents.public_key` exists.
Local database dumps are ignored/untracked because they may contain account
hashes and runtime data; the local files are preserved.

## Enrollment

See [Agent API](docs/AGENTS_API_DOCS.md) and
[Enrollment tokens](docs/TOKENS_API_DOCS.md). Registration validates and stores
a canonical Ed25519 public key. An enrollment token authorizes joining; neither
the Agent ID nor `/exists` proves ownership of the private key. WebSocket
private-key authentication is deferred to Phase 4.

## Checks

```powershell
go test ./...
go vet ./...
go build ./...
```

The PostgreSQL enrollment workflow and migration tests require
`TEST_DATABASE_URL` in lib/pq keyword DSN format, pointing **only to a disposable
test database**. Tests create/drop random schemas and may install pgcrypto.
They never read the application's `.env` database configuration.

```powershell
# Configure TEST_DATABASE_URL locally; do not paste its password in reports.
go test ./service -run 'TestEnrollment(Workflow|Migration)Postgres' -count=1 -v
```

`service/testdata/enrollment_schema.sql` is only the isolated enrollment/auth/audit
test subset, not a production bootstrap. Tests cover canonical key storage,
concurrent quota enforcement, rollback after failed inserts, duplicate identity
protection, revoked/expired tokens and the existing legacy-token migration.
