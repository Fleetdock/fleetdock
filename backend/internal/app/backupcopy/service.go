// Package backupcopyapp manages backup copies: a backup restored into a
// temporary database next to its source, kept for a while so a table can be
// compared with it and rows put back, then dropped.
package backupcopyapp

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	backupcopydom "github.com/Fleetdock/fleetdock/backend/internal/domain/backupcopy"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// Restorer starts the restore and drop operations (backupapp.Service).
type Restorer interface {
	RestoreCopy(ctx context.Context, backupID, copyID uuid.UUID, scratch string, createdBy *uuid.UUID) (*jobdom.Job, error)
	DropCopy(ctx context.Context, instanceID, backupID, copyID uuid.UUID, scratch string, createdBy *uuid.UUID) (*jobdom.Job, error)
}

// Preparer readies a restored copy for comparing (disables MySQL events,
// which would otherwise run inside the copy). Optional.
type Preparer interface {
	PrepareCopy(ctx context.Context, instanceID uuid.UUID, scratch string) error
}

// Service runs backup copies.
type Service struct {
	copies    backupcopydom.Repository
	backups   backupdom.Repository
	databases databasedom.Repository
	restorer  Restorer
	preparer  Preparer
	ttl       time.Duration
	maxBytes  int64
	now       func() time.Time
}

// NewService builds the service. ttl is how long a ready copy lives;
// maxBytes caps the compressed size of a backup that may be opened (0 = no
// cap).
func NewService(copies backupcopydom.Repository, backups backupdom.Repository, databases databasedom.Repository,
	restorer Restorer, ttl time.Duration, maxBytes int64) *Service {
	if ttl <= 0 {
		ttl = backupcopydom.DefaultTTL
	}
	return &Service{copies: copies, backups: backups, databases: databases, restorer: restorer, ttl: ttl,
		maxBytes: maxBytes, now: time.Now}
}

// SetPreparer attaches the copy preparer.
func (s *Service) SetPreparer(p Preparer) { s.preparer = p }

// Create opens a backup of a database as a copy: it restores it into a new
// temporary database on the same instance.
func (s *Service) Create(ctx context.Context, databaseID, backupID uuid.UUID, createdBy *uuid.UUID) (*backupcopydom.Copy, error) {
	db, err := s.databases.GetByID(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	if db.System {
		return nil, apperr.Invalid("database_id", "system databases cannot be compared with backups")
	}
	b, err := s.backups.GetByID(ctx, backupID)
	if err != nil {
		return nil, err
	}
	if b.DatabaseID != db.ID {
		return nil, apperr.Invalid("backup_id", "this backup is of another database")
	}
	if b.Status != backupdom.StatusCompleted || b.StorageURL == nil {
		return nil, apperr.Invalid("backup_id", "only completed backups in backup storage can be opened")
	}
	if s.maxBytes > 0 && b.SizeBytes != nil && *b.SizeBytes > s.maxBytes {
		return nil, apperr.Invalid("backup_id", "this backup is larger than FLEETDOCK_BACKUP_COPY_MAX_BYTES allows opening")
	}
	c := &backupcopydom.Copy{
		DatabaseID: db.ID, BackupID: b.ID, InstanceID: db.InstanceID, Status: backupcopydom.StatusRestoring,
		TTL: s.ttl, CreatedBy: createdBy,
	}
	// Named after a fresh random id: the row's own id exists only after the
	// insert, and the name is stored with it.
	c.ScratchName = databasedom.ScratchName("fds", uuid.New())
	if err := s.copies.Create(ctx, c); err != nil {
		return nil, err
	}
	job, err := s.restorer.RestoreCopy(ctx, b.ID, c.ID, c.ScratchName, createdBy)
	if err != nil {
		_ = s.copies.MarkFailed(ctx, c.ID, err.Error())
		_ = s.copies.MarkDropped(ctx, c.ID)
		return nil, err
	}
	if err := s.copies.SetJob(ctx, c.ID, job.ID); err != nil {
		return nil, err
	}
	c.JobID = &job.ID
	return c, nil
}

// List returns a database's copies that are not dropped yet, newest first.
func (s *Service) List(ctx context.Context, databaseID uuid.UUID) ([]backupcopydom.Copy, error) {
	return s.copies.ListByDatabase(ctx, databaseID)
}

// Get returns one copy of a database.
func (s *Service) Get(ctx context.Context, databaseID, copyID uuid.UUID) (*backupcopydom.Copy, error) {
	c, err := s.copies.Get(ctx, copyID)
	if err != nil {
		return nil, err
	}
	if c.DatabaseID != databaseID {
		return nil, apperr.NotFound("backup copy not found")
	}
	return c, nil
}

// Extend keeps a ready copy for another TTL from now, up to MaxTTL.
func (s *Service) Extend(ctx context.Context, databaseID, copyID uuid.UUID) (*backupcopydom.Copy, error) {
	c, err := s.Get(ctx, databaseID, copyID)
	if err != nil {
		return nil, err
	}
	if c.Status != backupcopydom.StatusReady || c.ReadyAt == nil {
		return nil, apperr.Conflict("only a ready backup copy can be extended")
	}
	at := s.now().Add(s.ttl)
	if limit := c.ReadyAt.Add(backupcopydom.MaxTTL); at.After(limit) {
		at = limit
	}
	if err := s.copies.SetExpiry(ctx, c.ID, at); err != nil {
		return nil, err
	}
	c.ExpiresAt = &at
	return c, nil
}

// Discard drops a copy now. One still restoring is dropped as soon as its
// restore finishes.
func (s *Service) Discard(ctx context.Context, databaseID, copyID uuid.UUID, by *uuid.UUID) error {
	c, err := s.Get(ctx, databaseID, copyID)
	if err != nil {
		return err
	}
	switch c.Status {
	case backupcopydom.StatusDropped, backupcopydom.StatusDropping:
		return nil
	case backupcopydom.StatusRestoring:
		return s.copies.MarkDropping(ctx, c.ID, nil)
	}
	return s.drop(ctx, c, by)
}

func (s *Service) drop(ctx context.Context, c *backupcopydom.Copy, by *uuid.UUID) error {
	job, err := s.restorer.DropCopy(ctx, c.InstanceID, c.BackupID, c.ID, c.ScratchName, by)
	if err != nil {
		return err
	}
	return s.copies.MarkDropping(ctx, c.ID, &job.ID)
}

// OnCopyRestored implements operationapp.BackupCopyHook.
func (s *Service) OnCopyRestored(ctx context.Context, copyID uuid.UUID, ok bool, errMsg *string) {
	c, err := s.copies.Get(ctx, copyID)
	if err != nil {
		slog.Warn("backup copy restored: unknown copy", "copy_id", copyID, "error", err.Error())
		return
	}
	if c.Status == backupcopydom.StatusDropping || !ok {
		if !ok {
			msg := "the restore failed"
			if errMsg != nil {
				msg = *errMsg
			}
			_ = s.copies.MarkFailed(ctx, c.ID, msg)
		}
		// Drop whatever the restore created (DROP … IF EXISTS).
		if err := s.drop(ctx, c, c.CreatedBy); err != nil {
			slog.Warn("drop backup copy", "copy_id", c.ID, "error", err.Error())
		}
		return
	}
	if s.preparer != nil {
		if err := s.preparer.PrepareCopy(ctx, c.InstanceID, c.ScratchName); err != nil {
			slog.Warn("prepare backup copy", "copy_id", c.ID, "error", err.Error())
		}
	}
	now := s.now()
	if err := s.copies.MarkReady(ctx, c.ID, now, now.Add(c.TTL)); err != nil {
		slog.Warn("mark backup copy ready", "copy_id", c.ID, "error", err.Error())
	}
}

// OnCopyDropped implements operationapp.BackupCopyHook. A failed drop stays
// "dropping" and is retried by ExpireDue.
func (s *Service) OnCopyDropped(ctx context.Context, copyID uuid.UUID, ok bool) {
	if !ok {
		return
	}
	if err := s.copies.MarkDropped(ctx, copyID); err != nil {
		slog.Warn("mark backup copy dropped", "copy_id", copyID, "error", err.Error())
	}
}

// ExpireDue drops expired and failed copies and retries stalled drops. It
// returns how many drops it started.
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	now := s.now()
	due, err := s.copies.ListDue(ctx, now, now.Add(-15*time.Minute))
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range due {
		c := &due[i]
		if err := s.drop(ctx, c, nil); err != nil {
			slog.Warn("expire backup copy", "copy_id", c.ID, "error", err.Error())
			continue
		}
		n++
	}
	return n, nil
}
