package grpc

import (
	"context"
	"errors"
	"io"
	"log"

	pb "work-distribution-patterns/patterns/p04/proto"
	"work-distribution-patterns/shared/models"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client implements contracts.TaskManager using gRPC
type Client struct {
	conn   *grpc.ClientConn
	client pb.TaskManagerClient
}

// NewClient creates a new gRPC client for the API
func NewClient(managerAddr string) (*Client, error) {
	conn, err := grpc.NewClient(
		managerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		client: pb.NewTaskManagerClient(conn),
	}, nil
}

// Submit creates a new task
func (c *Client) Submit(ctx context.Context, task models.Task) error {
	_, err := c.client.Submit(ctx, &pb.SubmitRequest{
		Task: TaskToProto(task),
	})
	return err
}

// Get retrieves a task by ID
func (c *Client) Get(ctx context.Context, id string) (models.Task, bool) {
	resp, err := c.client.Get(ctx, &pb.GetRequest{
		TaskId: id,
	})
	if err != nil {
		log.Printf("Error getting task %s: %v", id, err)
		return models.Task{}, false
	}

	if !resp.Found {
		return models.Task{}, false
	}

	return ProtoToTask(resp.Task), true
}

// List returns all tasks
func (c *Client) List(ctx context.Context) []models.Task {
	resp, err := c.client.List(ctx, &pb.ListRequest{})
	if err != nil {
		log.Printf("Error listing tasks: %v", err)
		return nil
	}

	tasks := make([]models.Task, len(resp.Tasks))
	for i, pt := range resp.Tasks {
		tasks[i] = ProtoToTask(pt)
	}

	return tasks
}

// Subscribe returns a channel of task events
func (c *Client) Subscribe(ctx context.Context, taskID string) (<-chan models.TaskEvent, error) {
	var tid *string
	if taskID != "" {
		tid = &taskID
	}

	stream, err := c.client.Subscribe(ctx, &pb.SubscribeRequest{
		TaskId: tid,
	})
	if err != nil {
		return nil, err
	}

	eventChan := make(chan models.TaskEvent, 100)

	go func() {
		defer close(eventChan)

		for {
			event, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				log.Printf("Error receiving event: %v", err)
				return
			}

			modelEvent := ProtoToEvent(event)

			select {
			case eventChan <- modelEvent:
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventChan, nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	return c.conn.Close()
}
