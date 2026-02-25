package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"

	pb "work-distribution-patterns/patterns/p04/proto"
	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements both WorkDistributionServer and TaskManagerServer
type Server struct {
	pb.UnimplementedWorkDistributionServer
	pb.UnimplementedTaskManagerServer

	store    store.TaskStore
	eventBus events.TaskEventBus

	// Worker management
	mu             sync.RWMutex
	workerStreams  map[string]pb.WorkDistribution_ConnectServer
	availableQueue chan string // worker IDs
	pendingTasks   chan models.Task

	// Event subscriptions
	subMu       sync.RWMutex
	subscribers map[string]chan models.TaskEvent
	nextSubID   int

	// Events from workers (for manager's event loop)
	workerEvents chan models.TaskEvent
}

// NewServer creates a new gRPC server
func NewServer(st store.TaskStore, evBus events.TaskEventBus) *Server {
	return &Server{
		store:          st,
		eventBus:       evBus,
		workerStreams:  make(map[string]pb.WorkDistribution_ConnectServer),
		availableQueue: make(chan string, 100),
		pendingTasks:   make(chan models.Task, 100),
		subscribers:    make(map[string]chan models.TaskEvent),
		workerEvents:   make(chan models.TaskEvent, 100),
	}
}

// Start begins background goroutines for dispatching and event broadcasting
func (s *Server) Start(ctx context.Context) {
	go s.dispatchLoop(ctx)
	go s.eventBroadcastLoop(ctx)
}

// dispatchLoop sends pending tasks to available workers
func (s *Server) dispatchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-s.pendingTasks:
			// Wait for an available worker
			select {
			case <-ctx.Done():
				return
			case workerID := <-s.availableQueue:
				s.mu.RLock()
				stream, exists := s.workerStreams[workerID]
				s.mu.RUnlock()

				if !exists {
					// Worker disconnected, re-queue task
					s.pendingTasks <- task
					continue
				}

				// Send task to worker
				if err := stream.Send(TaskToProto(task)); err != nil {
					log.Printf("Failed to send task %s to worker %s: %v", task.ID, workerID, err)
					// Worker failed, re-queue task and remove worker
					s.pendingTasks <- task
					s.removeWorker(workerID)
					continue
				}

				log.Printf("Dispatched task %s to worker %s", task.ID, workerID)
			}
		}
	}
}

// eventBroadcastLoop forwards events from eventBus to gRPC subscribers
func (s *Server) eventBroadcastLoop(ctx context.Context) {
	eventChan, err := s.eventBus.Subscribe(ctx)
	if err != nil {
		log.Printf("Failed to subscribe to event bus: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-eventChan:
			s.subMu.RLock()
			for _, ch := range s.subscribers {
				select {
				case ch <- event:
				default:
					// Subscriber can't keep up, skip
				}
			}
			s.subMu.RUnlock()
		}
	}
}

// removeWorker removes a worker from the pool
func (s *Server) removeWorker(workerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workerStreams, workerID)
	log.Printf("Worker %s removed", workerID)
}

// WorkDistribution.Connect - bidirectional stream for worker connection
func (s *Server) Connect(stream pb.WorkDistribution_ConnectServer) error {
	// Generate worker ID
	workerID := fmt.Sprintf("worker-%d", len(s.workerStreams)+1)

	// Register worker
	s.mu.Lock()
	s.workerStreams[workerID] = stream
	s.mu.Unlock()

	log.Printf("Worker %s connected", workerID)

	// Signal worker is available
	s.availableQueue <- workerID

	// Handle incoming events from worker
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			s.removeWorker(workerID)
			return nil
		}
		if err != nil {
			s.removeWorker(workerID)
			return err
		}

		// Convert and forward event to manager's event loop
		modelEvent := ProtoToEvent(event)
		select {
		case s.workerEvents <- modelEvent:
		default:
			log.Printf("Worker event queue full, dropping event")
		}

		// For terminal events, mark worker as available again
		if modelEvent.Type == models.EventTaskStatus &&
			(modelEvent.Status == string(models.TaskCompleted) || modelEvent.Status == string(models.TaskFailed)) {
			// Worker finished a task, mark as available
			select {
			case s.availableQueue <- workerID:
			default:
				// Queue full, skip
			}
		}
	}
}

// TaskManager.Submit - creates a new task
func (s *Server) Submit(ctx context.Context, req *pb.SubmitRequest) (*pb.SubmitResponse, error) {
	if req.Task == nil {
		return nil, status.Error(codes.InvalidArgument, "task is required")
	}

	task := ProtoToTask(req.Task)

	// Store task
	if err := s.store.Create(task); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store task: %v", err)
	}

	// Enqueue for dispatch
	select {
	case s.pendingTasks <- task:
	default:
		return nil, status.Error(codes.ResourceExhausted, "dispatch queue full")
	}

	return &pb.SubmitResponse{}, nil
}

// TaskManager.Get - retrieves a task by ID
func (s *Server) Get(_ context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	task, found := s.store.Get(req.TaskId)
	if !found {
		return &pb.GetResponse{Found: false}, nil
	}

	return &pb.GetResponse{
		Task:  TaskToProto(task),
		Found: true,
	}, nil
}

// TaskManager.List - returns all tasks
func (s *Server) List(_ context.Context, _ *pb.ListRequest) (*pb.ListResponse, error) {
	tasks := s.store.List()
	protoTasks := make([]*pb.Task, len(tasks))
	for i, t := range tasks {
		protoTasks[i] = TaskToProto(t)
	}

	return &pb.ListResponse{Tasks: protoTasks}, nil
}

// TaskManager.Subscribe - streams events to API clients
func (s *Server) Subscribe(req *pb.SubscribeRequest, stream pb.TaskManager_SubscribeServer) error {
	// Generate subscription ID
	s.subMu.Lock()
	subID := fmt.Sprintf("sub-%d", s.nextSubID)
	s.nextSubID++
	eventChan := make(chan models.TaskEvent, 100)
	s.subscribers[subID] = eventChan
	s.subMu.Unlock()

	defer func() {
		s.subMu.Lock()
		delete(s.subscribers, subID)
		close(eventChan)
		s.subMu.Unlock()
	}()

	taskID := ""
	if req.TaskId != nil {
		taskID = *req.TaskId
	}

	// Stream events
	for event := range eventChan {
		// Filter by task ID if specified
		if taskID != "" && event.TaskID != taskID {
			continue
		}

		if err := stream.Send(EventToProto(event)); err != nil {
			return err
		}
	}

	return nil
}

// GRPCDispatcher implements contracts.TaskDispatcher for the Manager
type GRPCDispatcher struct {
	server *Server
}

// NewDispatcher creates a new GRPCDispatcher
func NewDispatcher(server *Server) *GRPCDispatcher {
	return &GRPCDispatcher{server: server}
}

// Start starts the dispatcher (called by manager.Manager)
func (d *GRPCDispatcher) Start(ctx context.Context) error {
	d.server.Start(ctx)
	return nil
}

// Dispatch enqueues a task for workers
func (d *GRPCDispatcher) Dispatch(ctx context.Context, task models.Task) error {
	select {
	case d.server.pendingTasks <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return contracts.ErrDispatchFull
	}
}

// ReceiveEvent returns events from workers (blocks until event is available)
func (d *GRPCDispatcher) ReceiveEvent(ctx context.Context) (models.TaskEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskEvent{}, ctx.Err()
	case event := <-d.server.workerEvents:
		return event, nil
	}
}
