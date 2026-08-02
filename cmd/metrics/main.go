package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pramodlohar/retailedge-proxy/internal/cache"
	"github.com/pramodlohar/retailedge-proxy/internal/config"
)

func main() {
	logger := log.New(os.Stderr, "[metrics] ", log.LstdFlags)

	cfg, err := config.Load("/etc/retailedge/site.conf")
	if err != nil {
		cfg, err = config.Load("config/site.conf")
		if err != nil {
			logger.Fatalf("FATAL: load config: %v", err)
		}
	}

	db, err := cache.Open(cfg.DBPath, logger)
	if err != nil {
		logger.Fatalf("FATAL: open database: %v", err)
	}
	defer db.Close()

	m, err := db.CollectMetrics()
	if err != nil {
		logger.Fatalf("FATAL: collect metrics: %v", err)
	}

	queueDisplay := fmt.Sprintf("pending=%-4d sent=%-4d failed=%-2d cap=%d",
		m.QueuePending, m.QueueSent, m.QueueFailed, m.QueueCapacity)

	queuePct := 0
	if m.QueueCapacity > 0 {
		queuePct = (m.QueuePending * 100) / m.QueueCapacity
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Printf( "║  RetailEdge Proxy — Store %-26s ║\n", cfg.StoreID)
	fmt.Println("╠══════════════════════════════════════════════════════╣")
	fmt.Printf( "║  Products in Near Cache : %-26d ║\n", m.ProductCount)

	if m.LastSyncAt != nil {
		age := time.Since(*m.LastSyncAt).Round(time.Second)
		fmt.Printf("║  Last inbound sync      : %-26s ║\n", age.String()+" ago")
	} else {
		fmt.Printf("║  Last inbound sync      : %-26s ║\n", "never")
	}

	fmt.Printf("║  Write queue            : %-26s ║\n", queueDisplay)
	fmt.Printf("║  Queue fill level       : %-25d%% ║\n", queuePct)
	fmt.Printf("║  Schema version         : %-26s ║\n", m.SchemaVersion)
	fmt.Println("╠══════════════════════════════════════════════════════╣")

	health := "✅ HEALTHY"
	if m.ProductCount == 0 {
		health = "⚠️  EMPTY CACHE"
	} else if m.CacheAgeSecs > 1800 {
		health = "⚠️  STALE CACHE (>30m)"
	} else if queuePct >= 80 {
		health = "🔴 QUEUE NEAR FULL (>80%)"
	} else if m.QueuePending > 50 {
		health = "⚠️  QUEUE BACKLOG"
	} else if m.QueueFailed > 0 {
		health = "❌ FAILED WRITES"
	}

	fmt.Printf("║  Status                 : %-26s ║\n", health)
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
}