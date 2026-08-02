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
		// Fall back to local config during development
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

	// Print dashboard to stdout
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Printf("║  RetailEdge Proxy — Store %s         ║\n", cfg.StoreID)
	fmt.Println("╠══════════════════════════════════════════════╣")
	fmt.Printf("║  Products in Near Cache : %-18d ║\n", m.ProductCount)

	if m.LastSyncAt != nil {
		age := time.Since(*m.LastSyncAt).Round(time.Second)
		fmt.Printf("║  Last inbound sync      : %-18s ║\n", age.String()+" ago")
	} else {
		fmt.Printf("║  Last inbound sync      : %-18s ║\n", "never")
	}

	queueStatus := fmt.Sprintf("pending=%-3d sent=%-3d failed=%-1d",
		m.QueuePending, m.QueueSent, m.QueueFailed)
	fmt.Printf("║  Write queue            : %-18s ║\n", queueStatus)
	fmt.Printf("║  Schema version         : %-18s ║\n", m.SchemaVersion)
	fmt.Println("╠══════════════════════════════════════════════╣")

	// Health status
	health := "✅ HEALTHY"
	if m.ProductCount == 0 {
		health = "⚠️  EMPTY CACHE"
	} else if m.CacheAgeSecs > 1800 {
		health = "⚠️  STALE CACHE (>30m)"
	} else if m.QueuePending > 50 {
		health = "⚠️  QUEUE BACKLOG"
	} else if m.QueueFailed > 0 {
		health = "❌ FAILED WRITES"
	}
	fmt.Printf("║  Status                 : %-18s ║\n", health)
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()
}