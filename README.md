# MushroomChain

MushroomChain is a Go backend for cooperatives coordinating substrate batch production, quality release, order allocation, shipment execution, and farmer settlement. The product theme was selected from the China News rolling page entry titled "闽宁协作三十载：西吉小菌棒成为群众增收‘金棒棒’", observed on 2026-08-25. The repository is an original operational system and does not reproduce the article.

## Business workflows

The first public workflow is batch registration through commercial availability:

1. An operator or the owning farmer registers a substrate batch with a production date, expiry, species, quantity, and farm price.
2. An inspector records an independent sample and either releases or rejects the batch.
3. Only released, unexpired batches may be allocated. Allocation uses conditional versioned updates so concurrent dispatchers cannot consume the same quantity.
4. Allocation, inventory rows, shipment creation, and audit records commit in one database transaction. Any unsatisfied order line rolls the whole operation back.

The second workflow is fulfillment through farmer payment:

1. A dispatcher creates an idempotent order and allocates released batches in expiry order.
2. Delivery advances the order state and derives settlements from the actual batch allocations, grouped by farm.
3. Settlement creation and outbox notification commit with the delivered order state.
4. A finance user approves and pays each settlement with optimistic version checks.
5. The outbox worker leases events, retries transient publication failures with bounded exponential backoff, and records permanent failure after its attempt budget.

## Roles and sessions

The API distinguishes five roles:

- `operator`: registers batches and manages cooperative operations.
- `farmer`: registers only batches belonging to their farm and sees only their settlements.
- `inspector`: records quality decisions and controls release eligibility.
- `dispatcher`: creates, allocates, and delivers orders.
- `finance`: approves and pays farmer settlements.

`POST /v1/auth/login` creates a server-side session using a random bearer token. Only the SHA-256 token digest is persisted. Sessions expire, can be revoked through `POST /v1/auth/logout`, and are checked against the current active user on every authenticated request. Passwords use bcrypt with its maintained default cost and constant-time verification. Deployments should provision users through a controlled bootstrap or administration process and set a strong `BOOTSTRAP_PASSWORD` only for the first start.

## HTTP API

Public endpoints:

- `GET /healthz`: process liveness.
- `GET /readyz`: database readiness with a bounded context.
- `POST /v1/auth/login`: tenant-scoped login.

Authenticated endpoints:

- `POST /v1/auth/logout`
- `GET /v1/batches`
- `POST /v1/batches`
- `GET /v1/batches/{id}`
- `POST /v1/batches/{id}/inspections`
- `GET /v1/orders`
- `POST /v1/orders`
- `GET /v1/orders/{id}`
- `POST /v1/orders/{id}/allocate`
- `POST /v1/orders/{id}/deliver`
- `GET /v1/orders/{id}/settlements`
- `POST /v1/settlements/{id}/approve`
- `POST /v1/settlements/{id}/pay`

All errors use a stable JSON envelope with `code`, a user-readable `message`, and `request_id`. Clients may provide `X-Request-ID`; otherwise the service generates one. Request contexts propagate through services, SQL operations, readiness checks, and the worker publisher. The server has bounded header/read/write/idle timeouts and performs graceful shutdown on SIGINT or SIGTERM.

## Persistence and migration

The service uses SQLite through the pure-Go `modernc.org/sqlite` driver. The schema contains related tenants, users, sessions, farms, substrate batches, inspections, orders, order lines, allocations, shipments, settlements, idempotency records, outbox events, and audit logs. Foreign keys, unique constraints, status checks, indexes, and timestamps enforce the durable model.

Migration `001_initial.sql` is embedded in the executable and can build an empty database. It is safe to run repeatedly and records schema version 1. Startup enables foreign keys, WAL mode, and a bounded busy timeout. Integration tests use real temporary database files, close and reopen them, and verify that state survives process restart.

Critical concurrency boundaries use SQL conditions rather than service-only check-then-write logic:

- batch and order transitions require the expected version;
- allocation decrements only a released batch with sufficient quantity and the expected version;
- shipment and outbox claims have an owner and expiring lease;
- settlement approval/payment require the current version and state;
- cross-entity operations run through `WithinTx` and explicitly roll back on errors.

## Local development

Requirements:

- Go 1.26.1
- `GOTOOLCHAIN=local`
- Docker with BuildKit for container verification

Create local configuration:

```sh
cp .env.example .env
export BOOTSTRAP_PASSWORD='replace-with-a-strong-password'
GOTOOLCHAIN=local go run ./cmd/server
```

The optional bootstrap creates tenant `xiji-coop` and one active user for each role. Their emails follow `<role>@mushroomchain.local` and share the one-time bootstrap password. Remove `BOOTSTRAP_PASSWORD` after first startup. Production deployments should place the database on a persistent volume and provision credentials through their secret manager.

Run verification:

```sh
GOTOOLCHAIN=local go test ./... -count=1
GOTOOLCHAIN=local go test -race ./... -count=1
GOTOOLCHAIN=local go vet ./...
GOTOOLCHAIN=local go build ./...
```

## Container

The Dockerfile performs a multi-stage build. `TARGETOS` and `TARGETARCH` come from the requested platform; the file does not hard-code a CPU architecture or copy a host binary.

```sh
docker build --platform linux/amd64 -t mushroomchain:amd64 .
docker build --platform linux/arm64 -t mushroomchain:arm64 .
docker run --rm --platform linux/amd64 -p 18080:8080 mushroomchain:amd64
```

Container data is stored at `/data/mushroomchain.db`; mount `/data` for persistence. The application listens on port 8080 by default. Health checks should request both `/healthz` and `/readyz` on the actual mapped host port.

## Test coverage

The suite covers domain validation and illegal state transitions, password/token lifecycle, money and pagination boundaries, migration idempotency, foreign keys, session revocation, transaction rollback, optimistic conflicts, deterministic concurrent allocation, shipment/outbox lease recovery, restart persistence, service role checks, end-to-end allocation and settlement, HTTP error contracts, readiness failure, request IDs, worker retry, and graceful cancellation. Tests do not require online services and do not skip the database integration layer.
