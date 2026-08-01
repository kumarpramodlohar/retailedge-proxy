package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// ProductChange represents an inbound write from the Store VM.
// The API Service POSTs this when draining the Change Request Queue.
type ProductChange struct {
	StoreID   string  `json:"store_id"`
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Category  string  `json:"category"`
	InStock   bool    `json:"in_stock"`
	Version   int     `json:"version"`
}

// ChangeResponse is returned to the Store VM after accepting a write.
type ChangeResponse struct {
	Status    string `json:"status"`
	MessageID string `json:"message_id"`
	Timestamp string `json:"timestamp"`
}

func main() {
	logger := log.New(os.Stdout, "[cloud-api] ", log.LstdFlags)
	logger.Println("RetailEdge mock Cloud API starting")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Health check — Cloud Run calls this to verify the service is up
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Accept product change writes from the Store VM API Service
	mux.HandleFunc("/v1/products/changes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var change ProductChange
		if err := json.NewDecoder(r.Body).Decode(&change); err != nil {
			logger.Printf("invalid request body: %v", err)
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		logger.Printf("received change: store=%s product=%s price=%.2f",
			change.StoreID, change.ProductID, change.Price)

		// In production this would write to the MDM system.
		// Here we just acknowledge it and log it.
		resp := ChangeResponse{
			Status:    "accepted",
			MessageID: fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	})

	logger.Printf("listening on port %s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Fatalf("FATAL: %v", err)
	}
}