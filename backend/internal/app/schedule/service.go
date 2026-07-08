// Package scheduleapp implements backup-schedule use cases and the scheduler
// tick that turns due schedules into backups.
package scheduleapp

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	backupdestdom "github.com/TajBrains/db-manager/backend/internal/domain/backupdest"
	databasedom "github.com/TajBrains/db-manager/backend/internal/domain/database"
	scheduledom "github.com/TajBrains/db-manager/backend/internal/domain/schedule"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// BackupRunner triggers a scheduled backup (satisfied by *backupapp.Service).
type BackupRunner interface {
	TriggerScheduled(ctx context.Context, databaseID, destinationID, scheduleID uuid.UUID, retentionDays int, createdBy *uuid.UUID) error
}

// Service implements backup-schedule use cases.
type Service struct {
	repo      scheduledom.Repository
	databases databasedom.Repository
	dests     backupdestdom.Repository
	backups   BackupRunner
}

// NewService wires the schedule service.
func NewService(repo scheduledom.Repository, databases databasedom.Repository, dests backupdestdom.Repository, backups BackupRunner) *Service {
	return &Service{repo: repo, databases: databases, dests: dests, backups: backups}
}

// CreateInput describes a new schedule.
type CreateInput struct {
	DatabaseID    string
	DestinationID string
	Cron          string
	RetentionDays int
	Enabled       bool
	CreatedBy     *uuid.UUID
}

// Create validates references and persists a schedule.
func (s *Service) Create(ctx context.Context, in CreateInput) (*scheduledom.Schedule, error) {
	dbID, err := uuid.Parse(in.DatabaseID)
	if err != nil {
		return nil, apperr.Invalid("database_id", "database_id must be a valid UUID")
	}
	destID, err := uuid.Parse(in.DestinationID)
	if err != nil {
		return nil, apperr.Invalid("destination_id", "destination_id must be a valid UUID")
	}
	if _, err := s.databases.GetByID(ctx, dbID); err != nil {
		return nil, err
	}
	if _, err := s.dests.GetByID(ctx, destID); err != nil {
		return nil, err
	}
	sched, err := scheduledom.New(dbID, destID, in.Cron, in.RetentionDays, in.Enabled, in.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, sched); err != nil {
		return nil, err
	}
	return sched, nil
}

// UpdateInput describes changes to a schedule.
type UpdateInput struct {
	ID            string
	DestinationID string
	Cron          string
	RetentionDays int
	Enabled       bool
}

// Update validates and persists changes to a schedule.
func (s *Service) Update(ctx context.Context, in UpdateInput) (*scheduledom.Schedule, error) {
	sched, err := s.Get(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	destID, err := uuid.Parse(in.DestinationID)
	if err != nil {
		return nil, apperr.Invalid("destination_id", "destination_id must be a valid UUID")
	}
	if _, err := s.dests.GetByID(ctx, destID); err != nil {
		return nil, err
	}
	if err := sched.Apply(in.Cron, in.RetentionDays, in.Enabled, destID); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, sched); err != nil {
		return nil, err
	}
	return sched, nil
}

// List returns schedules, optionally filtered by database.
func (s *Service) List(ctx context.Context, databaseID string) ([]*scheduledom.Schedule, error) {
	var filter *uuid.UUID
	if databaseID != "" {
		id, err := uuid.Parse(databaseID)
		if err != nil {
			return nil, apperr.Invalid("database_id", "database_id must be a valid UUID")
		}
		filter = &id
	}
	return s.repo.List(ctx, filter)
}

// Get returns one schedule.
func (s *Service) Get(ctx context.Context, id string) (*scheduledom.Schedule, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.GetByID(ctx, uid)
}

// Delete removes a schedule.
func (s *Service) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.Delete(ctx, uid)
}

// RunDue triggers a backup for each schedule whose next_run_at has passed and
// advances its next run. It is invoked by the scheduler tick and reports how
// many schedules fired.
func (s *Service) RunDue(ctx context.Context) (int, error) {
	now := time.Now()
	due, err := s.repo.Due(ctx, now)
	if err != nil {
		return 0, err
	}
	fired := 0
	for _, sched := range due {
		// Advance the schedule first so a transient backup failure does not
		// cause the same slot to re-fire on the next tick.
		next, err := sched.AdvanceAfter(now)
		if err != nil {
			slog.Error("schedule: advance", "id", sched.ID, "error", err.Error())
			continue
		}
		if err := s.repo.MarkRun(ctx, sched.ID, now, next); err != nil {
			slog.Error("schedule: mark run", "id", sched.ID, "error", err.Error())
			continue
		}
		if err := s.backups.TriggerScheduled(ctx, sched.DatabaseID, sched.DestinationID, sched.ID, sched.RetentionDays, sched.CreatedBy); err != nil {
			slog.Warn("schedule: trigger backup failed", "id", sched.ID, "error", err.Error())
			continue
		}
		fired++
	}
	return fired, nil
}
