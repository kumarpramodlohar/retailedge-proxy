# RetailEdge Proxy

An offline-first store master data proxy in Go.  
Deployed via Debian packages and systemd — no containers, no Kubernetes.

> Portfolio project built to demonstrate Go, Linux-native delivery,  
> and bare-metal service management without cloud abstractions.

---

## Architecture

Calling Client (Java)
│
▼
gRPC Listener ← Unix domain socket
│
▼
Near Cache ← SQLite (WAL mode, single writer)
│
┌────┴────┐
│ │
▼ ▼
gRPC API gRPC Events ← systemd units, dedicated non-root users
Service Service
│ │
▼ ▼
Cloud MDM Change
REST API Events (PubSub)
**In scope (Store VM):** gRPC Listener, API Service, Events Service,  
Near Cache (SQLite), Site Config, Logs.  
**Out of scope:** Kubernetes, Terraform, multi-store fleet, web UI.

---

## Prerequisites

### On your Mac

| Tool                 | Install                                                |
| -------------------- | ------------------------------------------------------ |
| Homebrew             | [brew.sh](https://brew.sh)                             |
| Multipass            | `brew install --cask multipass`                        |
| Go                   | `brew install go`                                      |
| VS Code              | [code.visualstudio.com](https://code.visualstudio.com) |
| VS Code Go extension | Install from VS Code extensions panel                  |

### Check your Mac has room

```bash
df -h /
# Need at least 5 GB free
```

---

## Environment Setup (one-time)

### 1. Start the Multipass daemon

```bash
sudo launchctl load /Library/LaunchDaemons/com.canonical.multipassd.plist
```

### 2. Launch the Store VM

```bash
multipass launch 24.04 --name store-vm --cpus 2 --memory 2G --disk 10G
```

Takes 3–5 minutes on first run.

### 3. Mount your project folder into the VM

```bash
multipass mount ~/retailedge-proxy store-vm:/home/ubuntu/retailedge-proxy
```

> Note: the terminal will appear to hang — this is normal.  
> Open a new terminal tab and continue. The mount is active.

### 4. Bootstrap the VM (run once inside the VM)

```bash
multipass shell store-vm
sudo apt update && sudo apt install -y golang-go build-essential
go mod init github.com/pramodlohar/retailedge-proxy
```

### 5. Open VS Code on your Mac

```bash
code ~/retailedge-proxy
```

Edit files in VS Code — changes appear inside the VM instantly via the mount.

---

## Every Session — Start Here

```bash
# On Mac
multipass start store-vm
multipass mount ~/retailedge-proxy store-vm:/home/ubuntu/retailedge-proxy

# Open a new terminal tab, then enter the VM
multipass shell store-vm

# Open VS Code
code ~/retailedge-proxy
```

---

## Project Structure

retailedge-proxy/
├── cmd/
│ └── heartbeat/
│ └── main.go # P0: proof-of-concept heartbeat service
├── internal/ # P1+: cache, config, sync, queue packages
├── proto/ # P2+: gRPC definitions
├── packaging/
│ └── heartbeat/
│ ├── DEBIAN/
│ │ ├── control # Package metadata
│ │ └── postinst # Runs on install: creates user, starts service
│ ├── etc/systemd/system/
│ │ └── retailedge-heartbeat.service
│ └── usr/local/bin/ # Binary lands here (not committed)
├── migrations/ # P1+: numbered SQL migration files
├── scripts/ # Build, install, chaos demo
├── cloud/ # P3+: mock Cloud API + GCP setup
├── go.mod
├── .gitignore
└── README.md

---

## P0 · Slice Zero — Heartbeat Service

**Goal:** Prove the Debian packaging and systemd mechanic before writing  
any business logic. De-risk the least familiar part first.

**What it does:** A Go service that logs `store proxy alive` every 5 seconds,  
runs as a dedicated non-root user, and restarts automatically on crash.

### Build

```bash
# Inside the VM
cd ~/retailedge-proxy
go build -o packaging/heartbeat/usr/local/bin/retailedge-heartbeat ./cmd/heartbeat/
```

### Package

```bash
dpkg-deb --build packaging/heartbeat retailedge-heartbeat_0.1.0_arm64.deb
```

### Install

```bash
sudo dpkg -i retailedge-heartbeat_0.1.0_arm64.deb
```

### Verify

```bash
sudo systemctl status retailedge-heartbeat
sudo journalctl -u retailedge-heartbeat -f
```

Expected output every 5 seconds:
[heartbeat] store proxy alive

### The kill test — proof of restart policy

```bash
# Get the PID
sudo systemctl status retailedge-heartbeat | grep "Main PID"

# Kill it hard
sudo kill -9 <PID>

# Wait 6 seconds
sleep 6

# Confirm systemd restarted it with a new PID
sudo systemctl status retailedge-heartbeat
```

systemd restarts the service within 5 seconds automatically.  
This is blast-radius isolation without containers.

### Uninstall (if you need to reinstall cleanly)

```bash
sudo systemctl stop retailedge-heartbeat
sudo systemctl disable retailedge-heartbeat
sudo dpkg -r retailedge-heartbeat
sudo userdel retailedge
```

---

## Troubleshooting

| Problem                                  | Fix                                                                                 |
| ---------------------------------------- | ----------------------------------------------------------------------------------- |
| `cannot connect to the multipass socket` | `sudo pkill -f multipassd` then reload the daemon                                   |
| `Hash does not match` on launch          | `sudo rm -rf /var/root/Library/Caches/multipassd/qemu/vault/images/` then re-launch |
| Daemon shows dash (no PID) in launchctl  | `sudo cat /Library/Logs/Multipass/multipassd.log \| tail -50`                       |
| `status=203/EXEC` in systemctl           | Binary path wrong in unit file — check `/usr/` not `/user/`                         |
| `go.mod file not found`                  | Run `go mod init github.com/pramodlohar/retailedge-proxy` inside the VM             |
| `postinst` permission denied             | `chmod 755 packaging/heartbeat/DEBIAN/postinst` inside the VM                       |
| Mount command hangs                      | Normal — open a new terminal tab, mount is active                                   |

---

## Decisions & Trade-offs

| Decision       | What I chose                         | What I gave up            | When I'd choose differently     |
| -------------- | ------------------------------------ | ------------------------- | ------------------------------- |
| Packaging      | `.deb` + systemd                     | Container portability     | If the target was Kubernetes    |
| Service user   | Dedicated non-root `retailedge`      | Simpler single-user setup | Never — always use non-root     |
| Restart policy | `Restart=on-failure`, `RestartSec=5` | Immediate restart         | If crash loops needed a backoff |
| Architecture   | `arm64` (Apple Silicon VM)           | amd64 portability         | Match your production target    |

---

## Build Phases

| Phase             | Status  | Description                                           |
| ----------------- | ------- | ----------------------------------------------------- |
| P0 · Slice Zero   | ✅ Done | Heartbeat service, .deb packaging, systemd, kill test |
| P1 · Data Layer   | 🔲 Next | SQLite Near Cache, WAL mode, versioned migrations     |
| P2 · Read Path    | 🔲      | gRPC Listener over Unix socket                        |
| P3 · Cloud Side   | 🔲      | GCP project, Pub/Sub, mock Cloud API on Cloud Run     |
| P4 · Inbound Sync | 🔲      | Events Service consuming Pub/Sub                      |
| P5 · Write Path   | 🔲      | API Service with retry/backoff queue                  |
| P6 · Packaging    | 🔲      | All three services as proper .deb packages            |
| P7 · Demo         | 🔲      | Chaos script, offline demo, metrics                   |
| P8 · Showcase     | 🔲      | ARCHITECTURE.md, screen recording, recruiter-ready    |

---

## Key Learnings (P0)

- `go mod init` must run before `go build` — and run it inside the VM, not on Mac
- `postinst` needs `chmod 755` — VS Code cannot set Unix permissions
- `ExecStart` path must be exact — `/usr/` not `/user/` (caught this the hard way)
- Multipass mount hangs the terminal on macOS — the mount still works, open a new tab
- VS Code on Mac + Multipass mount = live editing directly in the VM with no sync needed
- The kill test is the proof — `kill -9` + systemd restart = blast-radius isolation without containers

---

_Built by Pramod Lohar as a deliberate gap-closing project.  
Background: 9+ years Java/Spring Boot. This project builds Go and Linux-native delivery depth._
