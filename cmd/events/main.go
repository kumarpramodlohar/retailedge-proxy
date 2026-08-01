package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pramodlohar/retailedge-proxy/internal/cache"
	"github.com/pramodlohar/retailedge-proxy/internal/config"
	"github.com/pramodlohar/retailedge-proxy/internal/events"
)

func main() {
	logger := log.New(os.Stdout, "[events] ", log.LstdFlags)
	logger.Println("RetailEdge Events Service starting")

	// Load site config — no service discovery, static per store
	cfg, err := config.Load("config/site.conf")
	if err != nil {
		logger.Fatalf("FATAL: load config: %v", err)
	}
	logger.Printf("store=%s subscription=%s", cfg.StoreID, cfg.PubSubSubscription)

	// Open Near Cache — runs migrations at startup
	db, err := cache.Open(cfg.DBPath, logger)
	if err != nil {
		logger.Fatalf("FATAL: open database: %v", err)
	}
	defer db.Close()

	// Create message handler — the single writer to the products table
	handler := events.NewHandler(db)

	// Create context that cancels on SIGTERM or SIGINT
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		logger.Println("shutdown signal received")
		cancel()
	}()

	// Create subscriber and start streaming pull
	sub, err := events.NewSubscriber(
		ctx,
		cfg.PubSubProject,
		cfg.PubSubSubscription,
		cfg.CredentialsFile,
		handler,
		logger,
	)
	if err != nil {
		logger.Fatalf("FATAL: create subscriber: %v", err)
	}
	defer sub.Close()

	// Run blocks until context is cancelled (SIGTERM/SIGINT)
	if err := sub.Run(ctx); err != nil {
		logger.Fatalf("FATAL: subscriber: %v", err)
	}

	logger.Println("Events Service stopped cleanly")
}