package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := log.New(os.Stdout, "[heartbeat] ", log.LstdFlags)
	logger.Println("RetailEdge heartbeat service starting")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	for {
		select {
		case <-ticker.C:
			logger.Println("store proxy alive")
		case <-stop:
			logger.Println("shutting down cleanly")
			return
		}
	}
}
