package grpc

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	pb "work-distribution-patterns/patterns/p04/proto"
	"work-distribution-patterns/shared/models"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var errNotConnected = errors.New("not connected to manager")

// Consumer implements contracts.TaskConsumer using gRPC bidirectional streaming
type Consumer struct {
	managerAddr string
	conn        *grpc.ClientConn
	client      pb.WorkDistributionClient
	stream      pb.WorkDistribution_ConnectClient

	mu        sync.Mutex
	taskQueue chan models.Task
	connected bool
}

// NewConsumer creates a new gRPC consumer
func NewConsumer(managerAddr string) *Consumer {
	return &Consumer{
		managerAddr: managerAddr,
		taskQueue:   make(chan models.Task, 10),
	}
}

// Connect establishes a gRPC connection and starts the bidirectional stream
func (c *Consumer) Connect(ctx context.Context) error {
	// Retry loop with backoff
	for {
		if err := c.connect(ctx); err != nil {
			log.Printf("Failed to connect to manager: %v, retrying in 2s...", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}

		// Start receiving tasks
		go c.receiveLoop(ctx)
		return nil
	}
}

// connect establishes the gRPC connection and stream
func (c *Consumer) connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Connect to manager
	conn, err := grpc.NewClient(
		c.managerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}

	c.conn = conn
	c.client = pb.NewWorkDistributionClient(conn)

	// Start bidirectional stream
	stream, err := c.client.Connect(ctx)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("error closing connection: %v", closeErr)
		}
		return err
	}

	c.stream = stream
	c.connected = true

	log.Printf("Connected to manager at %s", c.managerAddr)
	return nil
}

// receiveLoop receives tasks from the stream and queues them
func (c *Consumer) receiveLoop(ctx context.Context) {
	for {
		c.mu.Lock()
		stream := c.stream
		c.mu.Unlock()

		if stream == nil {
			log.Printf("Stream is nil, reconnecting...")
			if err := c.Connect(ctx); err != nil {
				return
			}
			continue
		}

		task, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			log.Printf("Stream closed by manager, reconnecting...")
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			if err := c.Connect(ctx); err != nil {
				return
			}
			continue
		}
		if err != nil {
			log.Printf("Error receiving task: %v, reconnecting...", err)
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			if err := c.Connect(ctx); err != nil {
				return
			}
			continue
		}

		// Convert and queue task
		modelTask := ProtoToTask(task)
		log.Printf("Received task %s", modelTask.ID)

		select {
		case c.taskQueue <- modelTask:
		case <-ctx.Done():
			return
		}
	}
}

// Receive returns the next task from the queue (blocks)
func (c *Consumer) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case task := <-c.taskQueue:
		return task, nil
	}
}

// Emit sends an event to the manager
func (c *Consumer) Emit(ctx context.Context, event models.TaskEvent) error {
	c.mu.Lock()
	stream := c.stream
	connected := c.connected
	c.mu.Unlock()

	if !connected || stream == nil {
		log.Printf("Not connected, cannot emit event")
		return errNotConnected
	}

	protoEvent := EventToProto(event)
	if err := stream.Send(protoEvent); err != nil {
		log.Printf("Failed to emit event: %v", err)
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return err
	}

	return nil
}

// Close closes the gRPC connection
func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stream != nil {
		if err := c.stream.CloseSend(); err != nil {
			log.Printf("Error closing stream: %v", err)
		}
	}

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}
