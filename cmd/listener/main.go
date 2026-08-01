package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pramodlohar/retailedge-proxy/internal/cache"
	pb "github.com/pramodlohar/retailedge-proxy/proto/gen"
	"google.golang.org/grpc/reflection"
)

// socketPath is the Unix domain socket the Listener binds to.
// The Java calling client connects to this path.
const socketPath = "/tmp/retailedge.sock"

// server implements the gRPC ProductService interface.
type server struct {
	pb.UnimplementedProductServiceServer
	db     *cache.DB
	logger *log.Logger
}

// GetProduct handles a single product lookup by ID.
func (s *server) GetProduct(
	ctx context.Context,
	req *pb.GetProductRequest,
) (*pb.GetProductResponse, error) {

	s.logger.Printf("GetProduct: id=%s", req.Id)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "product id is required")
	}

	p, err := s.db.GetProduct(req.Id)
	if err != nil {
		// sql.ErrNoRows means the product is not in the cache
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "product %s not found", req.Id)
		}
		s.logger.Printf("GetProduct error: %v", err)
		return nil, status.Error(codes.Internal, "database error")
	}

	return &pb.GetProductResponse{
		Product: toProto(p),
	}, nil
}

// ListProducts handles a filtered product list request.
func (s *server) ListProducts(
	ctx context.Context,
	req *pb.ListProductsRequest,
) (*pb.ListProductsResponse, error) {

	s.logger.Printf("ListProducts: category=%q", req.Category)

	products, err := s.db.ListProducts(req.Category)
	if err != nil {
		s.logger.Printf("ListProducts error: %v", err)
		return nil, status.Error(codes.Internal, "database error")
	}

	resp := &pb.ListProductsResponse{}
	for _, p := range products {
		resp.Products = append(resp.Products, toProto(p))
	}

	s.logger.Printf("ListProducts: returning %d products", len(resp.Products))
	return resp, nil
}

// toProto converts a cache.Product to the protobuf message type.
func toProto(p *cache.Product) *pb.Product {
	return &pb.Product{
		Id:        p.ID,
		Name:      p.Name,
		Price:     p.Price,
		Category:  p.Category,
		InStock:   p.InStock,
		Version:   int32(p.Version),
		UpdatedAt: p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func main() {
	logger := log.New(os.Stdout, "[listener] ", log.LstdFlags)
	logger.Println("RetailEdge gRPC Listener starting")

	// Open the Near Cache database — runs migrations automatically
	db, err := cache.Open("/tmp/retailedge.db", logger)
	if err != nil {
		logger.Fatalf("FATAL: open database: %v", err)
	}
	defer db.Close()

	// Seed test data so we can verify the read path with grpcurl
	if err := seedTestData(db, logger); err != nil {
		logger.Fatalf("FATAL: seed test data: %v", err)
	}

	// Remove stale socket file if it exists from a previous run
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		logger.Fatalf("FATAL: remove socket: %v", err)
	}

	// Listen on a Unix domain socket
	// File permissions will be 0600 — only the process owner can connect
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		logger.Fatalf("FATAL: listen on socket: %v", err)
	}
	logger.Printf("listening on unix socket: %s", socketPath)

	// Create and register the gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterProductServiceServer(grpcServer, &server{
		db:     db,
		logger: logger,
	})

	// Register reflection — allows grpcurl to discover services
	reflection.Register(grpcServer)

	// Handle shutdown signals gracefully
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	// Start serving in a goroutine so we can wait for the signal
	go func() {
		logger.Println("gRPC server ready")
		if err := grpcServer.Serve(lis); err != nil {
			logger.Printf("gRPC server stopped: %v", err)
		}
	}()

	// Block until shutdown signal
	<-stop
	logger.Println("shutting down cleanly")
	grpcServer.GracefulStop()
	logger.Println("shutdown complete")
}

// seedTestData inserts sample products so the read path can be verified
// immediately with grpcurl. Only used during P2 development.
// In production, the Events Service populates the cache from Pub/Sub.
func seedTestData(db *cache.DB, logger *log.Logger) error {
	products := []cache.Product{
		{
			ID:       "P001",
			Name:     "Organic Milk 2L",
			Price:    89.00,
			Category: "dairy",
			InStock:  true,
			Version:  1,
		},
		{
			ID:       "P002",
			Name:     "Whole Wheat Bread",
			Price:    45.00,
			Category: "bakery",
			InStock:  true,
			Version:  1,
		},
		{
			ID:       "P003",
			Name:     "Orange Juice 1L",
			Price:    65.00,
			Category: "beverages",
			InStock:  false,
			Version:  1,
		},
		{
			ID:       "P004",
			Name:     "Greek Yogurt 500g",
			Price:    120.00,
			Category: "dairy",
			InStock:  true,
			Version:  1,
		},
	}

	for i := range products {
		products[i].UpdatedAt = mustParseTime("2026-08-01T00:00:00Z")
		existing, err := db.GetProduct(products[i].ID)
		if existing != nil {
			// Already seeded — skip
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := db.UpsertProduct(&products[i]); err != nil {
			return err
		}
		logger.Printf("seeded product: %s — %s", products[i].ID, products[i].Name)
	}
	return nil
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}