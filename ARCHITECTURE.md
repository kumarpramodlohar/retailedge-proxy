# RetailEdge Proxy — Architecture

## Overview

RetailEdge Proxy is an offline-first store master data proxy. It runs
on a Linux VM inside each retail store and keeps a local SQLite Near
Cache of product master data. The store serves product reads instantly
from the local cache — the WAN link to GCP is never in the read path.

When the WAN drops: reads continue uninterrupted. Writes queue locally.
When the WAN returns: the queue drains and inbound sync resumes from its
checkpoint. The Java POS client notices nothing either way.
Java POS Client (existing)
│
│ gRPC over Unix domain socket
│ (local, microsecond latency, no network dependency)
▼
┌─────────────────────────────────────────────────────┐
│ Store VM — Delivery Scope │
│ │
│ ┌──────────────┐ ┌───────────────────────┐ │
│ │ gRPC │ │ Near Cache │ │
│ │ Listener │─────▶│ SQLite · WAL mode │ │
│ │ (Go) │◀─────│ single writer │ │
│ └──────────────┘ └───────────────────────┘ │
│ │ ▲ │
│ │ │ writes │
│ ▼ │ │
│ ┌──────────────┐ ┌──────────────────────┐ │
│ │ Change │ │ Events Service │ │
│ │ Request │ │ (Go) · Pub/Sub pull │ │
│ │ Queue │ │ ACK on success │ │
│ │ SQLite │ └──────────────────────┘ │
│ └──────────────┘ │
│ │ │
│ ┌──────────────┐ │
│ │ API Service │ │
│ │ (Go) │ │
│ │ retry+backoff│ │
│ └──────────────┘ │
└─────────┬───────────────────────┬───────────────────┘
│ │
│ HTTPS POST │ Pub/Sub streaming pull
▼ ▼
Cloud REST API MDM Change Events
(Cloud Run) (GCP Pub/Sub)

---

## Services

### gRPC Listener

- Serves `GetProduct` and `ListProducts` from the Near Cache
- Listens on a Unix domain socket (`/var/lib/retailedge/retailedge.sock`)
- Runs schema migrations at startup
- Seeds test data during development
- **Never writes to the products table** — read-only path
- Packaged as `retailedge-listener_0.1.0_arm64.deb`

### Events Service

- Subscribes to GCP Pub/Sub (`store-001-product-changes`)
- Streaming pull — persistent connection, receives messages as they arrive
- `MaxOutstandingMessages=1` — processes one message at a time (single writer rule)
- ACKs only after a successful write to the Near Cache
- NACKs on failure — Pub/Sub redelivers
- **The single designated writer to the products table**
- Packaged as `retailedge-events_0.1.0_arm64.deb`

### API Service

- Polls the Change Request Queue every 5 seconds
- POSTs pending writes to the Cloud REST API
- Exponential backoff with full jitter — prevents thundering herd on reconnect
- Abandons entries after 10 failed attempts
- **Never reads from the products table** — write drain path only
- Packaged as `retailedge-api_0.1.0_arm64.deb`

---

## Data Layer

### Near Cache — `internal/cache/`

SQLite database at `/var/lib/retailedge/retailedge.db`.

| PRAGMA         | Value  | Why                                           |
| -------------- | ------ | --------------------------------------------- |
| `journal_mode` | `WAL`  | Concurrent readers while one writer is active |
| `busy_timeout` | `5000` | Wait 5s before returning SQLITE_BUSY          |
| `foreign_keys` | `ON`   | Referential integrity enforced                |

Single writer rule: only the Events Service writes to the `products`
table. The Listener and API Service are read-only. This is enforced
architecturally, not just by convention.

### Migrations — `internal/cache/sql/`

Versioned forward-only migrations embedded in the binary at compile
time via `//go:embed`. Run automatically at every service startup.
Refuse to start if the database schema is ahead of the binary.

| File                                  | Purpose                  |
| ------------------------------------- | ------------------------ |
| `001_create_schema_version.sql`       | Version tracking table   |
| `002_create_products.sql`             | Near Cache product table |
| `003_create_change_request_queue.sql` | Outbound write queue     |

---

## Architectural Decisions

### Why SQLite, not PostgreSQL?

SQLite is an embedded library — no server process, no configuration,
no operator required. The store VM is unattended. A crashed PostgreSQL
process would mean a store that cannot serve customers until someone
SSHes in to restart it. SQLite starts when the service starts and stops
when it stops.

Trade-off accepted: single writer only. Mitigated by designating exactly
one service (Events Service) as the writer.

### Why Unix domain socket, not TCP?

The Java client and gRPC Listener run on the same VM. A Unix socket is
faster (no network stack) and secured by filesystem permissions. The
socket file at `/var/lib/retailedge/retailedge.sock` is owned by the
`retailedge` system user — network-adjacent processes cannot connect.

### Why .deb + systemd, not Docker?

Client requirement: no container abstractions on store VMs. Debian
packages and systemd are the standard Linux service management
primitives. Each service is a separate systemd unit with its own
restart policy. One service crashing does not affect the others.

### Why static config, not service discovery?

When the WAN drops, there is no registry to query. Every endpoint is
written to `/etc/retailedge/site.conf` at provisioning time and never
changes at runtime.

### Why full jitter on backoff?

Without jitter, 500 stores reconnecting simultaneously all retry at
second 1, then second 2, then second 4 — all at the same time.
Full jitter spreads retries randomly across the backoff window.

---

## Offline Behaviour

| WAN State | Reads                     | Writes                        | Inbound Sync               |
| --------- | ------------------------- | ----------------------------- | -------------------------- |
| Up        | ✅ Served from Near Cache | ✅ Queue drains immediately   | ✅ Events consumed         |
| Down      | ✅ Served from Near Cache | ⏳ Queue accumulates locally  | ⏳ Paused at checkpoint    |
| Restored  | ✅ Served from Near Cache | ✅ Queue drains automatically | ✅ Resumes from checkpoint |

Reads are never affected by WAN state. The Java client has one contract:
call the gRPC Listener. It never knows or cares whether the WAN is up.

---

## Failure Scenarios

| Failure              | Impact                   | Recovery                                                   |
| -------------------- | ------------------------ | ---------------------------------------------------------- |
| gRPC Listener crash  | Reads unavailable        | systemd restarts in 5s                                     |
| Events Service crash | Inbound sync paused      | systemd restarts, resume point persisted                   |
| API Service crash    | Write drain paused       | systemd restarts, queue persists in SQLite                 |
| VM power cut         | All services stop        | systemd starts all on reboot, WAL guarantees no corruption |
| WAN down             | Reads fine, writes queue | Automatic on WAN restore                                   |
| Pub/Sub down         | Inbound sync paused      | 7-day message retention, auto-resume                       |
| Cloud API down       | Writes queue             | Exponential backoff, auto-drain on restore                 |

---

## Repository Layout

retailedge-proxy/
├── cmd/ # Runnable service binaries
│ ├── heartbeat/ # P0 — systemd mechanic proof
│ ├── migrate/ # P1 — migration test harness
│ ├── listener/ # P2 — gRPC read path
│ ├── events/ # P4 — inbound Pub/Sub sync
│ ├── api/ # P5 — outbound write drain
│ └── metrics/ # P7 — health dashboard
├── internal/
│ ├── cache/ # SQLite Near Cache + migrations + queue
│ ├── config/ # Site config loader
│ ├── events/ # Pub/Sub handler + subscriber
│ └── api/ # Cloud API client + drainer
├── proto/ # gRPC service definition + generated code
├── cloud/mockapi/ # Mock Cloud REST API (Cloud Run)
├── packaging/ # .deb packaging for each service
├── scripts/ # chaos.sh demo + seed-products.sh
├── config/ # site.conf template
└── ARCHITECTURE.md # This document

---

## Running Locally

### Prerequisites

- Multipass (macOS) with Ubuntu 24.04 VM
- Go 1.22+ inside the VM
- GCP project with Pub/Sub topic and subscription
- Service account key at `/etc/retailedge/credentials.json`

### Install all services

```bash
# Inside the store VM
sudo dpkg -i retailedge-listener_0.1.0_arm64.deb
sudo dpkg -i retailedge-events_0.1.0_arm64.deb
sudo dpkg -i retailedge-api_0.1.0_arm64.deb

# Verify
sudo systemctl status retailedge-listener retailedge-events retailedge-api
```

### Read a product

```bash
sudo /usr/local/bin/grpcurl -plaintext \
  -d '{"id": "P001"}' \
  unix:/var/lib/retailedge/retailedge.sock \
  retailedge.ProductService/GetProduct
```

### Run the chaos demo

```bash
bash scripts/chaos.sh
```

---

## Why This Project

Built as a deliberate portfolio project to close a gap in Go and
Linux-native delivery experience. The goal was to prove I can
build, package, and operate a Go service without the abstractions
that containers and managed cloud services provide — and to do it
in an offline-first architecture where those abstractions are not
available by design.

_Pramod Lohar — github.com/kumarpramodlohar/retailedge-proxy_
