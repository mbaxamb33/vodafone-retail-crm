# Vodafone Retail — customer relationships

A runnable first implementation of the Romanian retail CRM: React + TypeScript frontend and Go HTTP API. This is a local demonstration with fictional data, not a production deployment.

## Run

Requires Node 22.12+ (Node 24 recommended), npm, and Go 1.24+.

```sh
npm install
make dev
```

Open **http://127.0.0.1:5173**. Choose the employee or manager demonstration account. The API listens on `127.0.0.1:8080`. Use the exact frontend origin above; mutations enforce the configured origin.

Alternatively, start each service separately:

```sh
cd backend
DEMO_MODE=true go run ./cmd/server
```

```sh
npm run dev
```

## Implemented slice

- Romanian responsive interface with daily action queue, global phone/name search, customer directory and portfolio.
- Customer creation, normalized Romanian phone numbers, stable random IDs, shared phone numbers permitted.
- Customer history with paginated visits, author attribution, explicit journey-step selection, notes, ownership, optional opportunity and follow-up in one atomic operation.
- Follow-up completion, opportunity board and stage updates, final won/lost states protected against reopening.
- Distinct manager overview (attention queues, current pipeline, dated activity reports) and team workspace (employee workload, portfolio, opportunities, follow-ups, and visit activity).
- Shareable customer profile routes with last-conversation context, all store-visible customer opportunities and follow-ups, standalone ownership changes, and quick visits from search.
- Follow-up scheduling, rescheduling, waiting/unreachable outcomes and completion timestamps. Explicit opportunity links power missing-next-action and seven-day stale-stage indicators.
- Manager date ranges use Europe/Bucharest boundaries. Offer/win counts come from recorded stage events; visit-step incidence is clearly separated from conversion metrics.
- Server-side store isolation and authorization; employees cannot take another employee's customer or mutate another employee's follow-ups/opportunities.
- Cookie sessions, origin checks, login throttling, payload limits, request IDs, health endpoints and graceful shutdown.
- Durable JSON repository with atomic file replacement, restrictive file permissions, and an application-facing repository interface.

## Architecture

`web/src/features/` contains customer, follow-up, opportunity and manager screens. `components/ui.tsx` contains shared primitives. `domain.ts` holds runtime API schemas, types and domain formatting. `api.ts` handles cookies, errors and expired sessions. TanStack Query owns server state; local component state owns filters and drafts. Customer creation uses React Hook Form and Zod.

`backend/internal/crm` owns domain types, validation, permissions, atomic business operations and the repository boundary. `backend/cmd/server` owns HTTP, sessions, origin protection, response encoding and runtime configuration. The JSON adapter is for a single local server process; it is not a database selection for the production system. Do not run multiple servers against the same file.

## Configuration

| Variable      | Default                 | Purpose                                                                                       |
| ------------- | ----------------------- | --------------------------------------------------------------------------------------------- |
| `DEMO_MODE`   | disabled                | Must be `true` for the local demonstration; production authentication is not yet implemented. |
| `LISTEN_ADDR` | `127.0.0.1:8080`        | Go listen address.                                                                            |
| `APP_ORIGIN`  | `http://127.0.0.1:5173` | Exact allowed mutation origin. HTTPS enables Secure session cookies.                          |
| `DATA_PATH`   | `data/store.json`       | Repository path relative to backend working directory.                                        |

Demo profiles are seeded only when the file does not exist. Changes survive server restarts; sessions are in memory and require login after restart. Keep this demo local and use fictional data only.

## Verify

```sh
make test       # Go race tests, frontend domain and interaction tests
make lint       # ESLint, go vet, gofmt check
make build      # strict TypeScript build, Vite bundle, Go binary
npm run format # format frontend, documentation and config
```

CI builds and checks both applications. Business tests exercise transaction rollback, nonsequential journey history, ownership, store boundaries, follow-up permissions, opportunity finality, file persistence, session protection and manager authorization.

## Next implementation milestones

- Production identity provider, managed session persistence, deployment/TLS configuration and operational monitoring.
- Server-side search, pagination and sorting for all lists. The current `/workspace` bootstrap returns the store's customer directory and the authorized work queue, suited to this small local demo; only visit history is paginated.
- Configurable reason/action catalogs, customer editing and retention/anonymization workflows.
- Historical conversion cohorts and longer-term reporting. Current reports show dated operational events and observed visit-step incidence; seeded opportunity states are not invented historical events.
- Persistent internal notifications, field-level backend validation errors, centralized translation catalogs and automated browser E2E coverage. Component interaction tests cover the manager split, rejected ownership changes, colleague visits, search prefill and follow-up outcomes.
- Production persistence decision, migration strategy and concurrency control for multiple service instances.

The demo login deliberately lets the local reviewer choose a role. It must be replaced before any real customer data or external deployment. The server refuses to start without explicit demo mode.

## Workflow details

Ownership changes are audited independently and do not create a visit, change the last interaction date, or silently transfer existing tasks. Employees may claim pool customers and release their own; managers may assign another member of the same store. Visits default to preserving ownership.

Follow-ups remain actionable when waiting for the customer or when a call was unanswered; both require a next check-in date. Completing a follow-up preserves its due date and records completion time. An opportunity next step is an explicitly linked follow-up, not an unrelated task on the same customer. Existing unlinked demo tasks remain unlinked.

Customer profiles at `/customers/{id}` show store-level relationship context to store employees, while personal workspace lists and mutation permissions remain scoped to the responsible employee or manager. Manager audit history remains manager-only.
