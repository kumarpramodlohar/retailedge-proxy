package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pramodlohar/retailedge-proxy/internal/api"
	"github.com/pramodlohar/retailedge-proxy/internal/cache"
	"github.com/pramodlohar/retailedge-proxy/internal/config"
)

func main() {
	logger := log.New(os.Stdout, "[api-svc] ", log.LstdFlags)
	logger.Println("RetailEdge API Service starting")

	// Load site config
	cfg, err := config.Load("config/site.conf")
	if err != nil {
		logger.Fatalf("FATAL: load config: %v", err)
	}
	logger.Printf("store=%s cloud=%s", cfg.StoreID, cfg.CloudAPIURL)

	// Open Near Cache — migrations run at startup
	db, err := cache.Open(cfg.DBPath, logger)
	if err != nil {
		logger.Fatalf("FATAL: open database: %v", err)
	}
	defer db.Close()

	// Create Cloud API client
	client := api.NewClient(cfg.CloudAPIURL, cfg.StoreID)

	// Create drainer
	drainer := api.NewDrainer(db, client, logger)

	// Handle shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		logger.Println("shutdown signal received")
		cancel()
	}()

	// Seed some test entries so we can verify the drain immediately
	if err := seedTestQueue(db, logger); err != nil {
		logger.Fatalf("FATAL: seed queue: %v", err)
	}

	// Start the drain loop — blocks until ctx cancelled
	drainer.Run(ctx)

	logger.Println("API Service stopped cleanly")
}

// seedTestQueue adds 3 test entries to the queue for P5 verification.
// In production the gRPC Listener enqueues entries when the Java client
// submits product changes. We simulate that here.
func seedTestQueue(db *cache.DB, logger *log.Logger) error {
	entries := []struct {
		productID string
		payload   string
	}{
		{
			"P001",
			`{"product_id":"P001","name":"Organic Milk 2L","price":97.00,"category":"dairy","in_stock":true,"version":4}`,
		},
		{
			"P002",
			`{"product_id":"P002","name":"Whole Wheat Bread","price":48.00,"category":"bakery","in_stock":true,"version":2}`,
		},
		{
			"P006",
			`{"product_id":"P006","name":"Green Tea 100g","price":180.00,"category":"beverages","in_stock":true,"version":1}`,
		},
	}

	pending, _, _, err := db.QueueStats()
	if err != nil {
		return err
	}
	if pending > 0 {
		logger.Printf("queue already has %d pending entries — skipping seed", pending)
		return nil
	}

	for _, e := range entries {
		if err := db.EnqueueChange(e.productID, e.payload); err != nil {
			return err
		}
		logger.Printf("enqueued change: product=%s", e.productID)
	}
	return nil
}