# RetailEdge Proxy — Session Context File
# Paste this into a new Claude chat to restore full project context instantly.
# Update the "Current state" and "Next session" sections after every session.
# Last updated: July 2026 — end of P0 session

---

## Who I am

- **Name:** Pramod Lohar
- **Role:** Software Engineer, 9+ years experience
- **Current employer:** Baker Hughes, Kolkata, India (oil & gas domain)
- **Stack at work:** Java 21 / Spring Boot, React / TypeScript, Stencil.js, gRPC,
  Kafka, CQRS, PostgreSQL, Module Federation, GeoToolKit JS
- **AI tooling:** GitHub Copilot, ADLC (AI-assisted Dev Lifecycle), LangChain,
  OpenAI API, SpecKit spec-driven development
- **Job status:** Actively searching. Gap analysis done for Go Technical Lead role
  — decision: too early to apply, need 6 months Go hands-on first.
  Previously screened for Deloitte T&T EAD Consultant (Java) and SAP Concur
  Development Expert roles.
- **Education:** MCA — Biju Patnaik University of Technology, Rourkela, Odisha, 2009
- **Certifications:** Generative AI with LLMs (DeepLearning.AI), ChatGPT Prompt
  Engineering for Developers, LangChain for LLM Application Development

---

## The Project — RetailEdge Proxy

### What it is
An offline-first store master data proxy in Go. Runs on a Linux VM inside each
retail store. Keeps a local SQLite Near Cache of product master data. Serves reads
locally even when the WAN to GCP is down. Syncs inbound changes from cloud MDM via
Google Pub/Sub. Queues outbound writes to a Cloud REST API with retry and backoff.
Ships as .deb packages managed by systemd. No containers. No Kubernetes.

### Why it exists
Deliberate portfolio project to close the Go and Linux-native delivery gap identified
in the Go Technical Lead role gap analysis. Target completion: early November 2026
(14 weekends, ~56 hours).

### Repos
- **Code:** https://github.com/kumarpramodlohar/retailedge-proxy (public)
- **Career docs:** pramod-career-docs (GitHub, private)

### Architecture (in scope — Store VM only)
```
Java Calling Client
      |
      | gRPC over Unix domain socket
      v
gRPC Listener -----> Near Cache (SQLite, WAL mode)
      |                     ^
      |               Events Service <--- MDM Change Events (GCP Pub/Sub)
      v
gRPC API Service ---------> Cloud REST API (GCP Cloud Run)
      |
Change Request Cache (SQLite queue)
```

Three Go services: gRPC Listener, gRPC API Service, gRPC Events Service.
One data entity in scope: Product master data only.
Out of scope: Customer/Store/Inventory/Pricing, Kubernetes, Terraform, web UI.

---

## Environment

### Mac (host)
- Apple Silicon (arm64), macOS
- VS Code with Go extension
- Homebrew installed
- Go installed on Mac (for go mod init / IDE support)

### Store VM
- Tool: Multipass
- Name: store-vm
- OS: Ubuntu 24.04 LTS (arm64 / aarch64)
- IP: 192.168.252.2
- CPU: 2, RAM: 2G, Disk: 10G
- Storage: internal Mac storage (NOT the ExFAT external SSD — abandoned)
- Go version inside VM: go1.22.2 linux/arm64
- systemd version: 255

### Workflow
- Edit code in VS Code on Mac
- Multipass mount syncs files to VM live:
  ~/retailedge-proxy => /home/ubuntu/retailedge-proxy
- Build and test inside the VM terminal
- NOTE: multipass mount command hangs the terminal — this is normal,
  the mount is active. Open a new terminal tab and continue.

### Multipass daemon
- Must be running before any multipass commands
- Start: sudo launchctl load /Library/LaunchDaemons/com.canonical.multipassd.plist
- Stop:  sudo launchctl unload /Library/LaunchDaemons/com.canonical.multipassd.plist
- Check: sudo launchctl list | grep multipass (PID = running, dash = crashed)
- Log:   sudo cat /Library/Logs/Multipass/multipassd.log | tail -50

### SSD situation (resolved — do not retry)
- External SSD (ExtreamSSD) is formatted as exFAT
- exFAT does not support Unix permissions — Multipass daemon cannot write SSH keys
- Resolution: removed symlinks, Multipass uses internal Mac storage
- Do NOT attempt to use the SSD for Multipass again unless reformatted as APFS

---

## Current Build State

### P0 — Slice Zero: COMPLETE (July 30, 2026)

**What was built:**
- Go heartbeat service (cmd/heartbeat/main.go)
  - Logs "store proxy alive" every 5 seconds
  - Handles SIGTERM and SIGINT for clean shutdown
  - Uses goroutines and channels (Go concurrency model)
- Debian package: retailedge-heartbeat_0.1.0_arm64.deb
  - DEBIAN/control: Package metadata, Architecture: arm64
  - DEBIAN/postinst: Creates retailedge system user, enables and starts service
  - etc/systemd/system/retailedge-heartbeat.service: Unit file
  - usr/local/bin/retailedge-heartbeat: The compiled Go binary
- systemd unit file:
  - User=retailedge, Group=retailedge (dedicated non-root user)
  - Restart=on-failure, RestartSec=5
  - StandardOutput=journal
- Kill test PASSED: sudo kill -9 <PID> => systemd restarted within 5 seconds

**Project structure:**
```
retailedge-proxy/
├── cmd/heartbeat/main.go
├── packaging/heartbeat/
│   ├── DEBIAN/control
│   ├── DEBIAN/postinst (chmod 755 required — set inside VM)
│   ├── etc/systemd/system/retailedge-heartbeat.service
│   └── usr/local/bin/retailedge-heartbeat (binary — gitignored)
├── go.mod (module: github.com/pramodlohar/retailedge-proxy)
├── .gitignore
├── README.md (full P0 documentation)
└── CONTEXT.md (this file)
```

**Useful commands (run inside the VM):**
```bash
# Build
cd ~/retailedge-proxy
go build -o packaging/heartbeat/usr/local/bin/retailedge-heartbeat ./cmd/heartbeat/

# Package
dpkg-deb --build packaging/heartbeat retailedge-heartbeat_0.1.0_arm64.deb

# Install
sudo dpkg -i retailedge-heartbeat_0.1.0_arm64.deb

# Check status
sudo systemctl status retailedge-heartbeat

# Watch logs
sudo journalctl -u retailedge-heartbeat -f

# Kill test
sudo systemctl status retailedge-heartbeat | grep "Main PID"
sudo kill -9 <PID>
sleep 6
sudo systemctl status retailedge-heartbeat

# Clean reinstall
sudo systemctl stop retailedge-heartbeat
sudo systemctl disable retailedge-heartbeat
sudo dpkg -r retailedge-heartbeat
sudo userdel retailedge
```

### P1 through P8 — NOT YET STARTED

---

## Build Plan Summary

| Phase | Status | Description |
|-------|--------|-------------|
| P0 Slice Zero | DONE | Heartbeat .deb + systemd + kill test |
| P1 Data Layer | NEXT | SQLite Near Cache, WAL, migrations |
| P2 Read Path | TODO | gRPC Listener over Unix socket |
| P3 Cloud Side | TODO | GCP Pub/Sub + Cloud Run mock API |
| P4 Inbound Sync | TODO | Events Service consuming Pub/Sub |
| P5 Write Path | TODO | API Service with retry/backoff queue |
| P6 Packaging | TODO | All 3 services as proper .deb packages |
| P7 Demo | TODO | Chaos script + offline demo + metrics |
| P8 Showcase | TODO | ARCHITECTURE.md + screen recording |

---

## Next Session — P1: SQLite Data Layer

**Goal:** Create the Near Cache SQLite schema for the Product entity.
Enable WAL mode. Write the versioned migration runner.

**Homework done before P1:**
- Read: "SQLite WAL mode explained"
- Read: SQLite PRAGMA statements (journal_mode, busy_timeout, foreign_keys)

**What P1 will build:**
- internal/cache/ package
- Product table schema (id, name, price, category, updated_at, version)
- Migration runner that:
  - Runs at service startup
  - Is versioned and forward-only
  - Is idempotent (safe to run twice)
  - Refuses to start on version mismatch (fail-safe)
- Enable: PRAGMA journal_mode=WAL
- Enable: PRAGMA busy_timeout=5000
- Enable: PRAGMA foreign_keys=ON
- Single writer: Events Service only
- Verify WAL mode with: PRAGMA journal_mode;

---

## Key Architectural Decisions Made

| Decision | Choice | Why |
|----------|--------|-----|
| Database | SQLite in WAL mode | No server process, offline-capable, zero ops |
| Single writer | Events Service | Only source of truth for cache is inbound MDM sync |
| Architecture | .deb + systemd, no Docker | Client requirement: no cloud/container abstractions |
| Cloud | Real GCP free tier | Closes second resume gap (hands-on cloud evidence) |
| Scope | Product entity only | 56 hours cannot cover 5 entities; depth > breadth |
| VM arch | arm64 (Apple Silicon) | Multipass on M-series Mac |
| Socket type | Unix domain socket (local), TCP (cross-VM) | Performance + security |
| Conflict rule | Cloud-authoritative, local writes provisional | Cloud is source of truth |

---

## Known Gotchas (do not repeat these mistakes)

- **Multipass mount hangs** — the mount IS active even when the terminal hangs.
  Open a new terminal tab. Do not kill it.
- **go mod init must run inside the VM** — not on Mac. Otherwise go.mod is empty
  when synced, causing "missing module declaration" error.
- **postinst needs chmod 755** — VS Code on Mac cannot set Unix permissions.
  Always run chmod 755 inside the VM after the file syncs.
- **ExecStart path must be exact** — we had /user/local/bin/ instead of
  /usr/local/bin/. Caused status=203/EXEC error. Always verify the path.
- **ExFAT SSD cannot hold Unix permissions** — ownership shows _unknown _unknown.
  chown silently does nothing. Use internal Mac storage for Multipass.
- **go.mod sync issue** — do not sync go.mod from Mac. Run go mod init in the VM
  once and never overwrite it from the Mac side.

---

## Documents Produced (all in pramod-career-docs private repo)

| File | Purpose |
|------|---------|
| Architect_First_Pass_Checklist.docx | Reusable 6-step framework for any new system |
| Worked_Example_Store_Proxy.docx | Checklist applied to RetailEdge |
| Go_Lead_Role_Gap_Analysis.docx | Honest gap assessment vs Go Lead JD |
| RetailEdge_Proxy_Build_Plan.docx | 14-weekend charter with interview talking points |
| Multipass_Command_Reference_v2.docx | All commands: Multipass + Go + Linux + Git |
| Tech_Stack_Explained.docx | Every technology explained from scratch |
| Fleet_Delivery_Guide.docx | CI/CD, apt repo, provisioning, fleet updates |
| Database_Decision_Reference.docx | SQLite vs all alternatives + 25 failure scenarios |
| Database_Selection_Template.docx | Blank reusable DB selection framework |
| DB_Template_RetailEdge_Filled.docx | Blank template filled for RetailEdge |

---

## How to Use This File

**At the end of any session:**
1. Update "Current Build State" to reflect what was completed
2. Update "Next Session" with the goals for next time
3. Add any new gotchas discovered
4. Commit and push: git commit -m "Update CONTEXT.md after P1 session"

**At the start of a new session:**
Paste this entire file into the chat with this message:

"Here is my project context. Please read it and confirm you understand
where we left off before we continue:"

Claude will read it and pick up exactly where we left off.

