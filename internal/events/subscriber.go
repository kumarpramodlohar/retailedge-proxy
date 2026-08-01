package events

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
)

// Subscriber pulls messages from a GCP Pub/Sub subscription and
// passes each one to the Handler. It runs until the context is cancelled.
// Uses streaming pull — a persistent connection that receives messages
// as they arrive, more efficient than repeated synchronous pulls.
type Subscriber struct {
	client       *pubsub.Client
	subscription *pubsub.Subscription
	handler      *Handler
	logger       *log.Logger
}

// NewSubscriber creates a Subscriber connected to the given project
// and subscription, authenticated with the credentials file.
func NewSubscriber(
	ctx context.Context,
	project string,
	subscriptionID string,
	credentialsFile string,
	handler *Handler,
	logger *log.Logger,
) (*Subscriber, error) {
	client, err := pubsub.NewClient(ctx, project,
		option.WithCredentialsFile(credentialsFile),
	)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}

	sub := client.Subscription(subscriptionID)

	// Configure receive settings:
	// MaxOutstandingMessages=1 ensures we process one message at a time.
	// This is important because we have a single SQLite writer —
	// parallel processing would race on the database connection.
	sub.ReceiveSettings.MaxOutstandingMessages = 1

	logger.Printf("subscriber ready: project=%s subscription=%s",
		project, subscriptionID)

	return &Subscriber{
		client:       client,
		subscription: sub,
		handler:      handler,
		logger:       logger,
	}, nil
}

// Run starts the streaming pull loop. Blocks until ctx is cancelled.
// For each message:
//   - calls handler.Handle() to parse and write to Near Cache
//   - ACKs the message only if Handle() succeeds
//   - NACKs the message if Handle() fails — Pub/Sub redelivers it
func (s *Subscriber) Run(ctx context.Context) error {
	s.logger.Println("starting streaming pull from Pub/Sub")

	err := s.subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		s.logger.Printf("received message: id=%s event_id=%s",
			msg.ID, string(msg.Data)[:min(40, len(msg.Data))])

		if err := s.handler.Handle(msg.Data); err != nil {
			// Handle failed — NACK so Pub/Sub redelivers
			s.logger.Printf("handle error (nacking): %v", err)
			msg.Nack()
			return
		}

		// Success — ACK to tell Pub/Sub we processed it
		msg.Ack()
		s.logger.Printf("message acked: id=%s", msg.ID)
	})

	if err != nil && ctx.Err() == nil {
		// Real error, not just context cancellation
		return fmt.Errorf("subscription receive: %w", err)
	}

	return nil
}

// Close releases the Pub/Sub client connection.
func (s *Subscriber) Close() error {
	return s.client.Close()
}

// min returns the smaller of two integers.
// Needed for safe string slicing in log output.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}