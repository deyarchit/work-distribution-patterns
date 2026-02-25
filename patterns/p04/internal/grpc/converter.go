package grpc

import (
	"time"

	pb "work-distribution-patterns/patterns/p04/proto"
	"work-distribution-patterns/shared/models"
)

// TaskToProto converts models.Task to proto.Task
func TaskToProto(t models.Task) *pb.Task {
	stages := make([]*pb.Stage, len(t.Stages))
	for i, s := range t.Stages {
		stages[i] = &pb.Stage{
			Name: s.Name,
		}
	}

	createdAt := t.SubmittedAt.Unix()
	updatedAt := t.SubmittedAt.Unix()
	if t.CompletedAt != nil {
		updatedAt = t.CompletedAt.Unix()
	} else if t.DispatchedAt != nil {
		updatedAt = t.DispatchedAt.Unix()
	}

	return &pb.Task{
		Id:        t.ID,
		Title:     t.Name,
		Status:    string(t.Status),
		Stages:    stages,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

// ProtoToTask converts proto.Task to models.Task
func ProtoToTask(pt *pb.Task) models.Task {
	stages := make([]models.Stage, len(pt.Stages))
	for i, s := range pt.Stages {
		stages[i] = models.Stage{
			Index: i,
			Name:  s.Name,
		}
	}

	createdAt := time.Unix(pt.CreatedAt, 0)
	var dispatchedAt, completedAt *time.Time
	if pt.UpdatedAt != pt.CreatedAt {
		t := time.Unix(pt.UpdatedAt, 0)
		if pt.Status == string(models.TaskCompleted) || pt.Status == string(models.TaskFailed) {
			completedAt = &t
		} else if pt.Status == string(models.TaskRunning) {
			dispatchedAt = &t
		}
	}

	return models.Task{
		ID:           pt.Id,
		Name:         pt.Title,
		Status:       models.TaskStatus(pt.Status),
		SubmittedAt:  createdAt,
		DispatchedAt: dispatchedAt,
		CompletedAt:  completedAt,
		Stages:       stages,
	}
}

// EventToProto converts models.TaskEvent to proto.TaskEvent
func EventToProto(e models.TaskEvent) *pb.TaskEvent {
	pe := &pb.TaskEvent{
		TaskId:    e.TaskID,
		EventType: e.Type,
		Timestamp: time.Now().Unix(),
	}

	if e.StageName != "" {
		pe.StageName = &e.StageName
	}
	if e.Progress != 0 {
		progress := int32(e.Progress)
		pe.Progress = &progress
	}
	if e.Status != "" {
		pe.Status = e.Status
	}

	return pe
}

// ProtoToEvent converts proto.TaskEvent to models.TaskEvent
func ProtoToEvent(pe *pb.TaskEvent) models.TaskEvent {
	e := models.TaskEvent{
		TaskID: pe.TaskId,
		Type:   pe.EventType,
	}

	if pe.StageName != nil {
		e.StageName = *pe.StageName
	}
	if pe.Progress != nil {
		e.Progress = int(*pe.Progress)
	}
	if pe.Status != "" {
		e.Status = pe.Status
	}

	return e
}
