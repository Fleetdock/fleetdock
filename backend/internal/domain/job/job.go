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
	TypeCreateDatabase    Type = "create_database"
	TypeDeleteDatabase    Type = "delete_database"
	TypeBackup            Type = "backup"
	TypeRestore           Type = "restore"
	TypeTestConnection    Type = "test_connection"
	TypeImportDatabases   Type = "import_databases"
	TypeProvisionInstance Type = "provision_instance"
	TypeStartInstance     Type = "start_instance"
	TypeStopInstance      Type = "stop_instance"
	TypeRestartInstance   Type = "restart_instance"
	TypeRemoveInstance    Type = "remove_instance"
)

// Valid reports whether t is a known job type.
func (t Type) Valid() bool {
	switch t {
	case TypeCreateDatabase, TypeDeleteDatabase, TypeBackup, TypeRestore,
		TypeTestConnection, TypeImportDatabases,
		TypeProvisionInstance, TypeStartInstance, TypeStopInstance,
		TypeRestartInstance, TypeRemoveInstance:
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

// JobLog is one line of execution output for a job. Seq is monotonic per job,
// assigned by the executor's log sink, so lines can be ordered and tailed
// incrementally (WHERE seq > afterSeq).
type JobLog struct {
	JobID     uuid.UUID
	Seq       int
	Level     string // info | warn | error | stderr
	Message   string
	CreatedAt time.Time
}

// ListFilter narrows job listings.
type ListFilter struct {
	Status       *Status
	Type         *Type
	ResourceType string
	ResourceID   *uuid.UUID
	Limit        int
	Offset       int
	// CreatedBy, when non-nil, restricts results to jobs created by that user
	// (used to scope operations for callers without global operation:read).
	CreatedBy *uuid.UUID
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
	// AppendLogs persists a batch of execution log lines for a job.
	AppendLogs(ctx context.Context, jobID uuid.UUID, lines []JobLog) error
	// ListLogs returns a job's log lines with seq > afterSeq, ordered by seq,
	// capped at limit.
	ListLogs(ctx context.Context, jobID uuid.UUID, afterSeq, limit int) ([]JobLog, error)
}
