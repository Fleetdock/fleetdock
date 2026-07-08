// Package schedule is the domain model for recurring (scheduled) backups.
package schedule

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
	"github.com/TajBrains/db-manager/backend/internal/platform/cron"
)

// Schedule is a recurring backup of one database to one destination.
type Schedule struct {
	ID            uuid.UUID
	DatabaseID    uuid.UUID
	DestinationID uuid.UUID
	Cron          string
	Engine        string
	RetentionDays int
	Enabled       bool
	LastRunAt     *time.Time
	NextRunAt     *time.Time
	CreatedBy     *uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// New validates input and constructs a Schedule, computing the first run time.
func New(databaseID, destinationID uuid.UUID, cronExpr string, retentionDays int, enabled bool, createdBy *uuid.UUID) (*Schedule, error) {
	sched, err := cron.Parse(cronExpr)
	if err != nil {
		return nil, apperr.Invalid("cron", err.Error())
	}
	if retentionDays <= 0 {
		return nil, apperr.Invalid("retention_days", "retention_days must be greater than 0")
	}
	s := &Schedule{
		ID:            uuid.New(),
		DatabaseID:    databaseID,
		DestinationID: destinationID,
		Cron:          cronExpr,
		Engine:        "mariadb-dump",
		RetentionDays: retentionDays,
		Enabled:       enabled,
		CreatedBy:     createdBy,
	}
	if enabled {
		next := sched.Next(time.Now())
		s.NextRunAt = &next
	}
	return s, nil
}

// Apply validates and overwrites mutable fields, recomputing the next run.
func (s *Schedule) Apply(cronExpr string, retentionDays int, enabled bool, destinationID uuid.UUID) error {
	sched, err := cron.Parse(cronExpr)
	if err != nil {
		return apperr.Invalid("cron", err.Error())
	}
	if retentionDays <= 0 {
		return apperr.Invalid("retention_days", "retention_days must be greater than 0")
	}
	s.Cron = cronExpr
	s.RetentionDays = retentionDays
	s.Enabled = enabled
	s.DestinationID = destinationID
	if enabled {
		next := sched.Next(time.Now())
		s.NextRunAt = &next
	} else {
		s.NextRunAt = nil
	}
	return nil
}

// AdvanceAfter computes the next run time strictly after `from`.
func (s *Schedule) AdvanceAfter(from time.Time) (time.Time, error) {
	sched, err := cron.Parse(s.Cron)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}

// Repository is the persistence port for backup schedules.
type Repository interface {
	Create(ctx context.Context, s *Schedule) error
	GetByID(ctx context.Context, id uuid.UUID) (*Schedule, error)
	List(ctx context.Context, databaseID *uuid.UUID) ([]*Schedule, error)
	Update(ctx context.Context, s *Schedule) error
	Delete(ctx context.Context, id uuid.UUID) error
	// Due returns enabled schedules whose next_run_at is at or before now.
	Due(ctx context.Context, now time.Time) ([]*Schedule, error)
	// MarkRun records a run: sets last_run_at and the recomputed next_run_at.
	MarkRun(ctx context.Context, id uuid.UUID, lastRun, nextRun time.Time) error
}
