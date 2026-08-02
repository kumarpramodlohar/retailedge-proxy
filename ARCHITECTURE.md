# RetailEdge Proxy — Architecture

> **Offline-first store master data proxy in Go.**  
> Runs on a Linux VM inside each retail store. Serves product reads locally.  
> The WAN link to GCP is never in the read path.

---

## Table of Contents

1. [Overview](#overview)
2. [System Diagram](#system-diagram)
3. [Services](#services)
4. [Data Flow](#data-flow)
5. [Data Layer](#data-layer)
6. [Deployment Model](#deployment-model)
7. [Security Model](#security-model)
8. [Architectural Decisions](#architectural-decisions)
9. [Offline Behaviour](#offline-behaviour)
10. [Failure Scenarios](#failure-scenarios)
11. [Repository Layout](#repository-layout)
12. [Running Locally](#running-locally)
13. [Why This Project](#why-this-project)

---

## Overview

RetailEdge Proxy sits permanently between the Java POS client and the cloud.
The client never talks to GCP directly — not even when the WAN is healthy.
Reads always come from the local SQLite Near Cache. The cloud is only involved
in two background paths: inbound sync (Events Service pulls from Pub/Sub) and
outbound writes (API Service drains a local queue to the Cloud REST API).

```
When WAN is UP:   reads from cache · writes drain immediately · sync active
When WAN is DOWN: reads from cache · writes queue locally    · sync paused
When WAN returns: reads from cache · queue drains            · sync resumes
```

The Java POS client has one contract: call the gRPC Listener on the Unix socket.
It never knows or cares about WAN state. The store keeps serving customers
throughout any network outage.

---

## System Diagram

```
  Java POS Client
  (existing system)
        │
        │  gRPC · Unix domain socket
        │  /var/lib/retailedge/retailedge.sock
        │  (local · microsecond latency · no network)
        ▼
╔═══════════════════════════════════════════════════════════════════╗
║  STORE VM  ·  Delivery Scope                                      ║
║                                                                   ║
║  ┌─────────────────┐         ┌─────────────────────────────────┐  ║
║  │  gRPC Listener  │ ──────▶ │         Near Cache              │  ║
║  │                 │ ◀────── │  SQLite · WAL mode              │  ║
║  │  reads only     │  reads  │  /var/lib/retailedge/           │  ║
║  └─────────────────┘         │  retailedge.db                  │  ║
║          │                   └──────────────┬──────────────────┘  ║
║          │ enqueue                          │ ▲                   ║
║          ▼                                  │ │ UpsertProduct      ║
║  ┌─────────────────┐                        │ │ (single writer)    ║
║  │ Change Request  │         ┌──────────────┘ │                   ║
║  │ Queue           │         │  Events Service │                   ║
║  │ SQLite          │         │  streaming pull │                   ║
║  └────────┬────────┘         │  ACK on success └───────────────┐  ║
║           │                  └─────────────────────────────────┘  ║
║           │ drain                                      ▲          ║
║           ▼                                            │          ║
║  ┌─────────────────┐                                   │          ║
║  │  API Service    │                           Pub/Sub pull        ║
║  │  retry+backoff  │                                   │          ║
║  │  full jitter    │                                   │          ║
║  └────────┬────────┘                                   │          ║
╚═══════════╪═══════════════════════════════════════════╪═══════════╝
            │                                           │
            │ HTTPS POST                                │ streaming pull
            ▼                                           ▼
   ┌──────────────────┐                    ┌──────────────────────┐
   │   Cloud REST API │                    │   GCP Pub/Sub        │
   │   Cloud Run      │                    │   mdm-product-changes│
   │   (mock in P3)   │                    │   7-day retention    │
   └──────────────────┘                    └──────────────────────┘
```

---

## Services

### gRPC Listener

**Role:** Read path. Serves all product queries from the Near Cache.

| Property | Value |
|----------|-------|
| Binary | `retailedge-listener` |
| Socket | `/var/lib/retailedge/retailedge.sock` (Unix domain) |
| Package | `retailedge-listener_0.1.0_arm64.deb` |
| systemd | `retailedge-listener.service` |
| Writes to products table | **Never** |

- Implements `GetProduct` and `ListProducts` via gRPC (Protocol Buffers)
- Runs schema migrations at startup before serving any request
- Registers gRPC reflection for tooling (grpcurl)
- Graceful shutdown on SIGTERM — drains in-flight requests

---

### Events Service

**Role:** Inbound sync. The single designated writer to the products table.

| Property | Value |
|----------|-------|
| Binary | `retailedge-events` |
| Package | `retailedge-events_0.1.0_arm64.deb` |
| systemd | `retailedge-events.service` |
| Writes to products table | **Only this service** |

- Subscribes to GCP Pub/Sub via streaming pull (persistent connection)
- `MaxOutstandingMessages=1` — one message at a time (enforces single writer rule)
- **ACK only after successful write** — if write fails, NACKs so Pub/Sub redelivers
- Idempotent by design — `ON CONFLICT(id) DO UPDATE` means same event twice = safe
- De-duplicates on `event_id` field in the JSON payload

---

### API Service

**Role:** Outbound drain. Sends locally queued writes to the Cloud REST API.

| Property | Value |
|----------|-------|
| Binary | `retailedge-api` |
| Package | `retailedge-api_0.1.0_arm64.deb` |
| systemd | `retailedge-api.service` |
| Reads from products table | **Never** |

- Polls Change Request Queue every 5 seconds (FIFO order)
- Batch size: 10 entries per cycle
- Exponential backoff with full jitter: `random(0, min(5m, 1s × 2^attempt))`
- Abandons after 10 failed attempts (marks entry `failed`, stops retrying)
- Injects `store_id` into every payload sent to the Cloud API

---

### Heartbeat Service (P0 proof)

**Role:** Proves the .deb + systemd packaging mechanic.

- Logs "store proxy alive" every 5 seconds
- Demonstrates `Restart=on-failure` — kill -9 → back in 5 seconds
- Not part of the proxy data path

---

## Data Flow

### Read path (always local, always fast)

```
Java POS client
  → gRPC GetProduct(id="P001")
  → Unix socket → gRPC Listener
  → SELECT FROM products WHERE id = ?
  → SQLite Near Cache (local disk)
  → Product{name, price, category, in_stock}
  → gRPC response to client
  Total: < 1 millisecond. No network. No cloud.
```

### Inbound sync path (background, cloud → cache)

```
MDM system publishes product change
  → GCP Pub/Sub topic (mdm-product-changes)
  → Subscription (store-001-product-changes)
  → Events Service streaming pull
  → JSON parse → ProductEvent{product_id, name, price, ...}
  → UpsertProduct() → SQLite Near Cache
  → ACK to Pub/Sub
  Next GetProduct call returns updated data immediately.
```

### Outbound write path (background, queue → cloud)

```
Java POS client submits product change
  → gRPC Listener → EnqueueChange() → Change Request Queue (SQLite)
  → Returns immediately (write is durable before returning)
  API Service polls queue (every 5s)
  → POST /v1/products/changes to Cloud REST API
  → On success: MarkSent()
  → On failure: MarkFailed(), retry with backoff
```

---

## Data Layer

### Near Cache — `internal/cache/`

Location: `/var/lib/retailedge/retailedge.db`  
Owner: `retailedge` system user  
Mode: SQLite with WAL journal

#### SQLite PRAGMA configuration

| PRAGMA | Value | Why |
|--------|-------|-----|
| `journal_mode` | `WAL` | Unlimited concurrent readers while one writer is active |
| `busy_timeout` | `5000` | Wait up to 5 seconds before returning `SQLITE_BUSY` |
| `foreign_keys` | `ON` | Referential integrity enforced at connection level |
| `max_open_conns` | `1` | Single connection serialises all writes at driver level |

#### Single writer rule

Only the Events Service writes to the `products` table. This is enforced at
three levels:

1. **Architecture** — only `cmd/events/` calls `UpsertProduct()`
2. **Driver** — `conn.SetMaxOpenConns(1)` serialises all writes
3. **SQLite** — WAL mode still allows only one concurrent writer

The single writer rule is what makes WAL mode safe here. If multiple services
wrote simultaneously, they would contend on the write lock and generate
`SQLITE_BUSY` errors.

#### Versioned migrations — `internal/cache/sql/`

Migrations are embedded in the binary at compile time via `//go:embed sql/*.sql`.
They run automatically at every service startup, before any reads or writes.

| Migration | Table created | Purpose |
|-----------|--------------|---------|
| `001_create_schema_version.sql` | `schema_version` | Version tracking. `version` column is TEXT not INTEGER. |
| `002_create_products.sql` | `products` | Near Cache. `id` is TEXT PRIMARY KEY (MDM IDs are strings). |
| `003_create_change_request_queue.sql` | `change_request_queue` | Outbound write queue with status, attempts, error tracking. |

**Fail-safe rule:** if the database schema version is ahead of the binary's
known migrations, the service refuses to start. This prevents an old binary
from silently corrupting a schema it does not understand.

#### products table

```sql
CREATE TABLE products (
    id         TEXT    PRIMARY KEY,       -- e.g. "P001" — from MDM, not generated
    name       TEXT    NOT NULL,
    price      REAL    NOT NULL,
    category   TEXT    NOT NULL,
    in_stock   INTEGER NOT NULL DEFAULT 1, -- boolean stored as 0/1
    version    INTEGER NOT NULL DEFAULT 1, -- incremented by MDM on each change
    updated_at TEXT    NOT NULL            -- RFC3339 timestamp
);
```

#### change_request_queue table

```sql
CREATE TABLE change_request_queue (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id   TEXT    NOT NULL,
    payload      TEXT    NOT NULL,         -- JSON to POST to Cloud API
    status       TEXT    NOT NULL DEFAULT 'pending', -- pending|sent|failed
    attempts     INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL,
    last_attempt TEXT,
    error        TEXT
);
```

---

## Deployment Model

### Per-store installation

Each store VM runs four systemd services, all installed via Debian packages:

```
retailedge-heartbeat  (P0 — proof of mechanic)
retailedge-listener   (P2 — gRPC read path)
retailedge-events     (P4 — inbound sync)
retailedge-api        (P5 — outbound drain)
```

### File system layout on the Store VM

```
/usr/local/bin/
├── retailedge-heartbeat    ← Go binary (statically linked except CGO/sqlite3)
├── retailedge-listener
├── retailedge-events
└── retailedge-api

/etc/systemd/system/
├── retailedge-heartbeat.service
├── retailedge-listener.service
├── retailedge-events.service
└── retailedge-api.service

/etc/retailedge/
├── site.conf               ← static store config (never changes at runtime)
└── credentials.json        ← GCP service account key (pubsub.subscriber only)

/var/lib/retailedge/
├── retailedge.db           ← SQLite Near Cache (WAL mode)
├── retailedge.db-shm       ← WAL shared memory file
├── retailedge.db-wal       ← WAL write-ahead log
└── retailedge.sock         ← Unix domain socket (gRPC Listener)
```

### systemd unit configuration

All services share the same pattern:

```ini
[Service]
User=retailedge
Group=retailedge
WorkingDirectory=/var/lib/retailedge
ExecStart=/usr/local/bin/retailedge-<service>
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
```

`Restart=on-failure` + `RestartSec=5` means any crash brings the service back
within 5 seconds automatically — no operator intervention required.

### Site config — `/etc/retailedge/site.conf`

Static key=value file deployed at provisioning time. Never changes at runtime.
No service discovery — the WAN may be down and there is no registry to query.

```ini
STORE_ID=STORE-001
CLOUD_API_URL=https://retailedge-mock-api-921013929826.us-central1.run.app
PUBSUB_PROJECT=retailedge-proxy
PUBSUB_SUBSCRIPTION=store-001-product-changes
GOOGLE_APPLICATION_CREDENTIALS=/etc/retailedge/credentials.json
DB_PATH=/var/lib/retailedge/retailedge.db
SOCKET_PATH=/var/lib/retailedge/retailedge.sock
```

---

## Security Model

### Principle of least privilege

The `retailedge` system user is created at install time:

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin retailedge
```

- No login shell — cannot be used as an interactive account
- No home directory — no persistent user state
- Owns `/var/lib/retailedge/` and `/etc/retailedge/` only

### GCP service account scope

The service account `retailedge-store-vm` has exactly one IAM role:
`roles/pubsub.subscriber`. It cannot:

- Publish to Pub/Sub
- Access any other GCP service
- View billing or project resources
- Create or delete resources

### Unix socket permissions

The socket at `/var/lib/retailedge/retailedge.sock` is owned by `retailedge`.
Only processes running as `retailedge` or in the `retailedge` group can connect.
Network-adjacent processes cannot reach it — there is no TCP port to scan.

### Credentials

The GCP service account key at `/etc/retailedge/credentials.json` is:

```
owner: retailedge
mode:  600  (only owner can read)
```

It is never committed to source control. `.gitignore` excludes `*-key.json`
and `*credentials*.json`.

---

## Architectural Decisions

### Why SQLite, not PostgreSQL or MySQL?

PostgreSQL and MySQL are client-server databases — they run as a separate
process that your application connects to. If that process crashes on an
unattended store VM, the application cannot serve reads until someone SSHes in.

SQLite is an embedded library. It starts when the service starts, stops when
the service stops, and lives as a single file. On an unattended store VM with
no operator, this is not a compromise — it is the correct choice.

**Trade-off accepted:** single writer only. Mitigated by designating exactly
one service (Events Service) as the writer. The single-writer constraint
validated the architecture — it forced a clean separation between the read
path and the write path.

**Trigger to reconsider:** if Near Cache data volume exceeds 5 GB, or if
write throughput requirements exceed ~500 writes/second, migrate to PostgreSQL.

---

### Why Unix domain socket, not TCP?

The Java client and gRPC Listener run on the same VM. Unix sockets are:

- **Faster** — no TCP/IP stack overhead, no port allocation
- **Safer** — secured by filesystem permissions, not network ACLs
- **Simpler** — no port conflicts, no firewall rules

A network attacker scanning the VM's open ports will not find the gRPC
Listener — it has no TCP port.

---

### Why .deb + systemd, not Docker or Kubernetes?

Client requirement: no container abstractions on store VMs. The practical
reasons align with this:

- **No Docker daemon to manage** — one less process that can fail
- **systemd is already process manager 1** — using it for services is not
  adding complexity, it is using what is already there
- **Blast-radius isolation** — each service is a separate unit with its own
  restart policy. One crash does not affect the others
- **Native packaging** — `dpkg -i` is the universal Linux install primitive

---

### Why static config, not service discovery?

When the WAN drops, there is no registry to query. A service discovery client
that cannot reach Consul or etcd would block startup — exactly when the proxy
needs to start fastest (on reboot after a power cut during an outage).

Static config is loaded from disk. It works with no network. It works at
boot time before any network interface is up.

---

### Why full jitter on exponential backoff?

Without jitter, 500 stores reconnecting simultaneously after a cloud outage
all retry at second 1, then second 2, then second 4 — all at exactly the same
time. This creates a thundering herd that can take the Cloud API down again
immediately after recovery.

Full jitter formula: `sleep = random(0, min(maxDelay, baseDelay × 2^attempt))`

This spreads 500 stores randomly across the backoff window. The Cloud API
sees a smooth ramp of traffic rather than repeated spikes.

---

### Why at-least-once delivery is acceptable here

GCP Pub/Sub guarantees at-least-once delivery — a message may arrive more
than once. This is acceptable because `UpsertProduct` is idempotent:

```sql
INSERT INTO products (...) VALUES (...)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name, price = excluded.price, ...
```

Processing the same event twice writes the same data twice — no harm done.
The `event_id` field in the payload provides an additional de-duplication
handle if stronger guarantees are needed later.

---

## Offline Behaviour

| WAN State | Reads | Writes | Inbound Sync |
|-----------|-------|--------|--------------|
| **Up** | ✅ Near Cache (instant) | ✅ Queue drains within 5s | ✅ Streaming pull active |
| **Down** | ✅ Near Cache (instant) | ⏳ Queue accumulates in SQLite | ⏳ Paused — resumes at checkpoint |
| **Restored** | ✅ Near Cache (instant) | ✅ Queue auto-drains | ✅ Auto-resumes, catches up |

Reads are **never** affected by WAN state. The Java client has one contract:
call the gRPC Listener on the Unix socket. It never observes a network error,
never retries, never implements a fallback. The proxy handles all of that.

**Maximum offline resilience:**
- Pub/Sub retains messages for 7 days — store can be offline for up to a week
  with no MDM changes lost
- SQLite WAL guarantees no data corruption on sudden power loss
- Change Request Queue persists across restarts — no writes lost on service crash

---

## Failure Scenarios

| Failure | Impact on Store | Recovery mechanism |
|---------|----------------|--------------------|
| gRPC Listener crashes | Reads unavailable | `systemd` restarts in 5s |
| Events Service crashes | Inbound sync paused | `systemd` restarts; Pub/Sub resume point is stored — no events lost or replayed |
| API Service crashes | Write drain paused | `systemd` restarts; Change Request Queue persists in SQLite |
| All services crash | Everything stops | `systemd` starts all on reboot; WAL guarantees Near Cache integrity |
| VM power cut | Everything stops | Same as all services crash — fully automatic recovery |
| WAN drops | Reads fine; writes queue | Fully automatic on WAN restore |
| GCP Pub/Sub down | Inbound sync paused | 7-day message retention; auto-resume when Pub/Sub recovers |
| Cloud API down | Write drain retrying | Exponential backoff + jitter; auto-drain when API recovers |
| Bad migration deployed | Service refuses to start | Schema version mismatch check; deploy correct binary to fix |
| Disk full | SQLite writes fail | Alert at 80% disk; bounded Change Request Queue prevents unbounded growth |

---

## Repository Layout

```
retailedge-proxy/
│
├── cmd/                          ← Runnable binaries (one per service)
│   ├── heartbeat/main.go         P0 — systemd mechanic proof
│   ├── migrate/main.go           P1 — migration test harness
│   ├── listener/main.go          P2 — gRPC read path
│   ├── events/main.go            P4 — inbound Pub/Sub sync
│   ├── api/main.go               P5 — outbound write drain
│   └── metrics/main.go           P7 — health dashboard
│
├── internal/                     ← Shared packages (imported by cmd/)
│   ├── cache/
│   │   ├── db.go                 Open(), PRAGMAs, WAL, Migrate()
│   │   ├── migrate.go            Versioned runner + //go:embed
│   │   ├── product.go            GetProduct, ListProducts, UpsertProduct
│   │   ├── queue.go              EnqueueChange, PendingChanges, MarkSent
│   │   ├── metrics.go            CollectMetrics() for health dashboard
│   │   └── sql/
│   │       ├── 001_create_schema_version.sql
│   │       ├── 002_create_products.sql
│   │       └── 003_create_change_request_queue.sql
│   ├── config/
│   │   └── config.go             Load(), validate(), Config struct
│   ├── events/
│   │   ├── handler.go            ProductEvent parse + UpsertProduct
│   │   └── subscriber.go         Pub/Sub streaming pull, ACK/NACK
│   └── api/
│       ├── client.go             HTTP POST to Cloud REST API
│       └── drainer.go            Poll loop, backoff, jitter
│
├── proto/
│   ├── product.proto             gRPC service definition (source of truth)
│   └── gen/
│       ├── product.pb.go         Generated — do not edit
│       └── product_grpc.pb.go    Generated — do not edit
│
├── cloud/
│   └── mockapi/main.go           Mock Cloud REST API (deployed to Cloud Run)
│
├── packaging/                    ← .deb structure (source only — binaries gitignored)
│   ├── heartbeat/DEBIAN/
│   ├── listener/DEBIAN/
│   ├── events/DEBIAN/
│   └── api/DEBIAN/
│
├── scripts/
│   ├── chaos.sh                  9-step offline chaos demo
│   └── seed-products.sh          Seeds 5 products via Pub/Sub
│
├── config/
│   └── site.conf                 Store config template
│
├── ARCHITECTURE.md               This document
├── README.md
└── CONTEXT.md                    Session context for AI-assisted development
```

---

## Running Locally

### Prerequisites

- macOS with Multipass installed
- Ubuntu 24.04 VM (`multipass launch 24.04 --name store-vm --cpus 2 --memory 2G`)
- Go 1.22+ inside the VM (`sudo apt install golang-go`)
- GCP project with Pub/Sub topic and subscription configured
- Service account key at `/etc/retailedge/credentials.json` on the VM

### Build all services

```bash
# Inside the store VM
cd ~/retailedge-proxy
go build ./...
```

### Run migrations (verify data layer)

```bash
go run ./cmd/migrate/
# Expected: 3 migrations applied, WAL confirmed
```

### Start the gRPC Listener

```bash
go run ./cmd/listener/
```

### Read a product (second terminal)

```bash
sudo /usr/local/bin/grpcurl -plaintext \
  -d '{"id": "P001"}' \
  unix:/var/lib/retailedge/retailedge.sock \
  retailedge.ProductService/GetProduct
```

### Install as systemd services

```bash
sudo dpkg -i retailedge-listener_0.1.0_arm64.deb
sudo dpkg -i retailedge-events_0.1.0_arm64.deb
sudo dpkg -i retailedge-api_0.1.0_arm64.deb

# Verify all running
sudo systemctl status retailedge-listener retailedge-events retailedge-api
```

### Check health

```bash
go run ./cmd/metrics/
```

### Run the chaos demo

```bash
# Seed products via Pub/Sub (on Mac)
bash scripts/seed-products.sh

# Run offline demo (in VM) — cuts WAN, proves reads continue, restores
bash scripts/chaos.sh
```

---

## Why This Project

Built as a deliberate portfolio project to close a gap in Go and Linux-native
delivery experience. The specific goal was to prove I can build, package, and
operate a Go service without the abstractions that containers and managed cloud
services provide — and to do it in an offline-first architecture where those
abstractions are not available by design.

Every phase was proven with a kill test before moving to the next:

| Phase | Kill test |
|-------|-----------|
| P0 | `kill -9` → systemd restarts within 5s |
| P1 | Second migration run → "schema is up to date" |
| P2 | Unknown ID → `Code: NotFound` |
| P3 | `gcloud pubsub pull` → `ACK_STATUS: SUCCESS` |
| P4 | Pub/Sub price change → grpcurl returns updated price |
| P5 | 3 entries queued → `pending=0 sent=3 failed=0` |
| P6 | All 4 services simultaneously `active (running)` |
| P7 | `iptables` cuts cloud → reads unchanged, queue drains on restore |

---

*Pramod Lohar · [github.com/kumarpramodlohar/retailedge-proxy](https://github.com/kumarpramodlohar/retailedge-proxy)*
