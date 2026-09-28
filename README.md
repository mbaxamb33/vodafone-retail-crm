# Vodafone Retail — customer relationships

Internal CRM for a Vodafone retail store: who the customer is, what happened on earlier visits, where the commercial conversation stands, and who follows up. React + TypeScript frontend, Go HTTP API, PostgreSQL.

It is the store's relationship and sales-workflow layer, not a replacement for billing, contract or provisioning systems.

## Run locally

Requires Node 22.12+ (Node 24 recommended), Go 1.24+, and Docker (for PostgreSQL).

```sh
npm install
make seed      # starts PostgreSQL on 127.0.0.1:5434, applies migrations, creates the demo store
make dev       # database + API on 127.0.0.1:8080 + frontend on 127.0.0.1:5173
```

Open **http://127.0.0.1:5173**. `make seed` prints the demo accounts and a generated password; set `DEMO_PASSWORD` before seeding to choose it. Demo accounts (all fictional data):

| Email                         | Role     |
| ----------------------------- | -------- |
| `ioana.marinescu@demo.local`  | employee |
| `andrei.popescu@demo.local`   | employee |
| `elena.dumitrescu@demo.local` | manager  |

The demo history (about two months of visits, stage changes, reassignments) is produced through the real services with a simulated clock, so reports and audit behave as in real use. `make db-reset` deletes all local data and reseeds.

To run the pieces separately: `make db-up`, then `cd backend && DATABASE_URL=… go run ./cmd/server`, and `npm run dev`. If the API runs elsewhere, start Vite with `API_URL=http://host:port npm run dev`.

## Accounts and stores

Accounts are managed with the `crmctl` CLI. Passwords are read from `CRM_PASSWORD` or standard input, never from flags.

```sh
cd backend
export DATABASE_URL=postgres://crm:crm@127.0.0.1:5434/crm?sslmode=disable
go run ./cmd/crmctl create-store -name "Magazin Iași"             # prints the store ID
go run ./cmd/crmctl create-user -store <id> -email ana@example.com -name "Ana Pop" -role employee
go run ./cmd/crmctl reset-password -email ana@example.com          # revokes the user's sessions
go run ./cmd/crmctl deactivate-user -email ana@example.com
go run ./cmd/crmctl list-stores
```

Signed-in users can change their own password with `POST /api/v1/auth/password`.

## Configuration

| Variable       | Default                 | Purpose                                                                           |
| -------------- | ----------------------- | --------------------------------------------------------------------------------- |
| `DATABASE_URL` | required                | PostgreSQL connection URL.                                                        |
| `APP_ENV`      | `development`           | `production` requires an `https://` `APP_ORIGIN`.                                 |
| `LISTEN_ADDR`  | `127.0.0.1:8080`        | API listen address.                                                               |
| `APP_ORIGIN`   | `http://127.0.0.1:5173` | Exact origin allowed to send mutations. HTTPS enables Secure cookies.             |
| `AUTO_MIGRATE` | `true`                  | Apply pending migrations at startup (guarded by an advisory lock).                |
| `SESSION_TTL`  | `8h`                    | Session lifetime.                                                                 |
| `LOG_LEVEL`    | `info`                  | `debug`, `info`, `warn` or `error`.                                               |
| `FEATURES`     | empty                   | Comma-separated feature flags, returned to the frontend by `GET /api/v1/auth/me`. |

Secrets belong in the environment, never in the repository. The credentials in `docker-compose.yml` are for the disposable local container only.

## Architecture

```
backend/
  cmd/server        HTTP server: configuration, wiring, graceful shutdown
  cmd/crmctl        administration CLI: migrations, stores, accounts, demo seed
  internal/crm      domain: entities, validation, role permissions, services, events
  internal/auth     password hashing (bcrypt), sessions, login throttling
  internal/postgres PostgreSQL store, embedded migrations, test helpers (pgtest)
  internal/httpapi  routes, middleware (request IDs, logging, origin check, auth), JSON errors
  internal/apperr   classified errors with codes and field messages
  internal/config   environment configuration
  internal/demo     fictional demo data
web/src
  features/         customers, follow-ups, opportunities, manager, notifications
  components/ui.tsx shared primitives
  domain.ts         runtime API schemas (Zod), types, formatting
  api.ts, hooks.ts  API client and shared queries
```

- **Layers.** Handlers decode and respond; `crm.Service` validates, authorizes and runs business operations; the `crm.Store` interface is the persistence boundary, implemented by `internal/postgres`.
- **Consistency.** Operations touching several records (a visit with ownership change, opportunities and a follow-up) run in one transaction. Rows being changed are locked with `SELECT … FOR UPDATE`, so concurrent edits serialize and several API instances can share the database.
- **Authorization.** Every rule is enforced on the server. Roles map to permissions in `internal/crm/authz.go`; adding a role means adding a row there. All reads and writes are scoped to the signed-in user's store.
- **History.** Visits, opportunity stage events and audit events are append-only. Edited visit notes keep their previous text in `visit_note_revisions`. Reports are computed from recorded events, never from counters.
- **Events.** Each business action is emitted as an audit event inside its transaction; in-app notifications (customer assigned or returned, follow-up assigned, opportunity changed by a colleague) are derived there. This is the hook for future integrations.
- **Configurable lists.** Visit reasons, next actions and product categories live in `catalog_items` (global defaults, optionally overridden per store).
- **Privacy.** Logs record request ID, route, status, duration, user and store — never bodies, query strings or customer data. Managers can anonymize a customer (`POST /customers/{id}/anonymize`), which clears identifying data and free-text notes while keeping counts. Employees see store-level context but not the audit log.

## Verify

```sh
make db-up      # the Go tests use real PostgreSQL in throwaway schemas
make test       # Go tests with the race detector, frontend tests
make lint       # ESLint, go vet, gofmt check
make build      # strict TypeScript build, Vite bundle, backend/bin/{server,crmctl}
npm run format  # format frontend, documentation and config
```

Database tests are skipped when `TEST_DATABASE_URL` is unset locally; CI sets `REQUIRE_DATABASE_TESTS=true` so they cannot be skipped there. CI runs PostgreSQL as a service and fails on formatting, lint, type, test or build errors.

The Go tests cover transaction rollback, historical visit steps, ownership rules, store isolation, follow-up and opportunity lifecycles, report date boundaries and funnel counts, search and paging, note revisions, anonymization, login throttling and session revocation, plus HTTP-level employee and manager workflows.

## Deliberate limits and next steps

- **Identity.** Email and password with server-side sessions. A company identity provider (OIDC/SSO) can replace `internal/auth` without touching the domain. There is no self-service password reset; administrators use `crmctl`.
- **Browser end-to-end tests.** The critical workflows are tested at API level; Playwright tests against a seeded database are the next step.
- **Metrics.** Structured logs carry latency and status per route; a metrics endpoint (e.g. Prometheus) is not yet exposed.
- **Retention.** Anonymization exists; an automatic retention schedule needs a policy decision first.
- **Anonymization and open tasks.** Anonymizing does not close the customer's open follow-ups.
- **Workspace size.** `/workspace` returns the user's own work (store-wide for managers) plus the customers it references. The store directory, search and portfolios use the paginated `/customers` endpoint.
- **Translations.** Interface text is Romanian and sits in the components; the server returns Romanian error messages alongside stable error codes.

## Workflow rules

Ownership changes are audited on their own and never create a visit, change the last interaction or move existing tasks. Employees may claim pool customers and release their own; managers may assign any member of their store. Visits keep the current owner unless the form says otherwise.

A visit stores exactly the journey steps selected; earlier steps are never inferred. Its furthest step feeds the conversation funnel. Opportunities created in a visit start at `identified`, independent of the visit's steps. A next action is linked to the visit's opportunity only when exactly one was created.

Waiting and unreachable follow-ups need a next check-in date. Completion keeps the due date, records the completion time and is final. Won and lost opportunities are final; repeating the current stage records nothing.
