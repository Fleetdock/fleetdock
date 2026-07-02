// Package job is the domain model for asynchronous operations (the
// user-facing name is "operations"). A job either runs on an agent
// (ServerID set) or on the control plane itself (ServerID nil — used for
// external instances the control plane reaches directly).
package job

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Type enumerates supported operations.
type Type string

const (
	TypeCreateDatabase  Type = "create_database"
	TypeDeleteDatabase  Type = "delete_database"
	TypeBackup          Type = "backup"
	TypeRestore         Type = "restore"
	TypeTestConnection  Type = "test_connection"
	TypeImportDatabases Type = "import_databases"
)

// Valid reports whether t is a known job type.
func (t Type) Valid() bool {
	switch t {
	case TypeCreateDatabase, TypeDeleteDatabase, TypeBackup, TypeRestore,
		TypeTestConnection, TypeImportDatabases:
		return true
	}
	return false
}

// Status is the lifecycle state of a job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

// Job is a tracked operation.
type Job struct {
	ID           uuid.UUID
	Type         Type
	ResourceType string
	ResourceID   *uuid.UUID
	Status       Status
	ServerID     *uuid.UUID // executor agent; nil = control plane
	Params       json.RawMessage
	Result       json.RawMessage
	Error        *string
	Progress     int
	CreatedBy    *uuid.UUID
	ClaimedAt    *time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
}

// ListFilter narrows job listings.
type ListFilter struct {
	Status       *Status
	Type         *Type
	ResourceType string
	ResourceID   *uuid.UUID
	Limit        int
	Offset       int
}

// Page is one page of jobs plus the unpaginated total.
type Page struct {
	Items []*Job
	Total int
}

// Repository is the persistence port for jobs.
type Repository interface {
	Create(ctx context.Context, j *Job) error
	GetByID(ctx context.Context, id uuid.UUID) (*Job, error)
	List(ctx context.Context, f ListFilter) (Page, error)
	// ClaimNext atomically claims the oldest pending job for the given
	// executor (nil = control plane) and marks it running.
	ClaimNext(ctx context.Context, serverID *uuid.UUID) (*Job, error)
	// Complete finalizes a job with succeeded/failed status.
	Complete(ctx context.Context, id uuid.UUID, status Status, result json.RawMessage, errMsg *string) error
	UpdateProgress(ctx context.Context, id uuid.UUID, progress int) error
}
