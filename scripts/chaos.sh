#!/bin/bash
# RetailEdge Proxy — Chaos Demo Script
# Demonstrates offline-first behaviour: reads keep working when WAN drops.
# Run from inside the Store VM.

set -e

SOCKET="/var/lib/retailedge/retailedge.sock"
DB="/var/lib/retailedge/retailedge.db"
GCP_RANGE="142.250.0.0/16"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${BLUE}[demo]${NC} $1"; }
ok()   { echo -e "${GREEN}[ok]${NC}   $1"; }
warn() { echo -e "${YELLOW}[warn]${NC} $1"; }

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║  RetailEdge Proxy — Offline Demo             ║"
echo "╚══════════════════════════════════════════════╝"
echo ""

# STEP 1: Show all services running
log "STEP 1 — All services running"
sudo systemctl status retailedge-listener retailedge-events retailedge-api \
  --no-pager | grep "Active:"
echo ""

# STEP 2: Show reads working normally
log "STEP 2 — Read product from Near Cache (local — no cloud needed)"
sudo /usr/local/bin/grpcurl -plaintext \
  -d '{"id": "P001"}' \
  unix:${SOCKET} \
  retailedge.ProductService/GetProduct
echo ""

# STEP 3: Metrics before chaos
log "STEP 3 — Near Cache metrics before cutting cloud"
go run ./cmd/metrics/ 2>/dev/null
echo ""

# STEP 4: Cut cloud connectivity
log "STEP 4 — Cutting cloud connectivity via iptables"
warn "Blocking GCP IP range: ${GCP_RANGE}"
sudo iptables -I OUTPUT -d ${GCP_RANGE} -j DROP
ok "WAN link is now DOWN"
echo ""

sleep 2

# STEP 5: Reads still work offline
log "STEP 5 — Reads still work (Near Cache is fully local)"
sudo /usr/local/bin/grpcurl -plaintext \
  -d '{"id": "P001"}' \
  unix:${SOCKET} \
  retailedge.ProductService/GetProduct
ok "Store serving customers — WAN state is irrelevant for reads"
echo ""

# STEP 6: Show write queue building up
log "STEP 6 — Checking write queue (API Service retrying with backoff)"
sleep 3
sqlite3 ${DB} \
  "SELECT id, product_id, status, attempts FROM change_request_queue ORDER BY id DESC LIMIT 5;"
echo ""

# STEP 7: Restore connectivity
log "STEP 7 — Restoring cloud connectivity"
sudo iptables -D OUTPUT -d ${GCP_RANGE} -j DROP
ok "WAN link restored"
echo ""

# STEP 8: Wait for queue to drain
log "STEP 8 — Waiting 15 seconds for queue to drain..."
sleep 15
sqlite3 ${DB} \
  "SELECT status, COUNT(*) as count FROM change_request_queue GROUP BY status;"
echo ""

# STEP 9: Final metrics
log "STEP 9 — Final state"
go run ./cmd/metrics/ 2>/dev/null
echo ""

ok "Demo complete — store served customers throughout the entire WAN outage"
echo ""
