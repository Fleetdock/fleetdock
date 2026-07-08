// Package moveapp implements the move-database saga on top of the existing
// backup + restore operations. A move is driven forward by completion hooks
// the operations engine calls when its sub-jobs finish.
package moveapp

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	backupapp "github.com/mariadb-cp/db-manager/backend/internal/app/backup"
	databaseapp "github.com/mariadb-cp/db-manager/backend/internal/app/database"
	databasedom "github.com/mariadb-cp/db-manager/backend/internal/domain/database"
	instancedom "github.com/mariadb-cp/db-manager/backend/internal/domain/instance"
	movedom "github.com/mariadb-cp/db-manager/backend/internal/domain/move"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/executor"
)

// Service implements the move-database saga.
type Service struct {
	repo      movedom.Repository
	databases databasedom.Repository
	instances instancedom.Repository
	backups   *backupapp.Service
	dbService *databaseapp.Service
}

// NewService wires the move service.
func NewService(repo movedom.Repository, databases databasedom.Repository, instances instancedom.Repository,
	backups *backupapp.Service, dbService *databaseapp.Service) *Service {
	return &Service{repo: repo, databases: databases, instances: instances, backups: backups, dbService: dbService}
}

// StartInput is the command to move a database.
type StartInput struct {
	SourceDatabaseID string
	TargetInstanceID string
	TargetDatabase   string // empty = keep source name
	DestinationID    string
	DropSource       bool
	CreatedBy        *uuid.UUID
}

// Start validates the request, kicks off a backup of the source, and records
// the move. It advances automatically as the backup and restore complete.
func (s *Service) Start(ctx context.Context, in StartInput) (*movedom.Move, error) {
	srcID, err := uuid.Parse(in.SourceDatabaseID)
	if err != nil {
		return nil, apperr.Invalid("source_database_id", "source_database_id must be a valid UUID")
	}
	tgtInstID, err := uuid.Parse(in.TargetInstanceID)
	if err != nil {
		return nil, apperr.Invalid("target_instance_id", "target_instance_id must be a valid UUID")
	}
	destID, err := uuid.Parse(in.DestinationID)
	if err != nil {
		return nil, apperr.Invalid("destination_id", "destination_id must be a valid UUID")
	}
	src, err := s.databases.GetByID(ctx, srcID)
	if err != nil {
		return nil, err
	}
	tgt, err := s.instances.GetByID(ctx, tgtInstID)
	if err != nil {
		return nil, err
	}
	if !tgt.HasCredentials() {
		return nil, apperr.Invalid("target_instance_id", "the target instance has no admin credentials")
	}
	targetName := in.TargetDatabase
	if targetName == "" {
		targetName = src.Name
	}

	backup, _, err := s.backups.Trigger(ctx, backupapp.TriggerInput{
		DatabaseID:    srcID.String(),
		DestinationID: destID.String(),
		CreatedBy:     in.CreatedBy,
	})
	if err != nil {
		return nil, err
	}

	m := &movedom.Move{
		ID:               uuid.New(),
		SourceDatabaseID: srcID,
		TargetInstanceID: tgtInstID,
		TargetDatabase:   targetName,
		DestinationID:    destID,
		DropSource:       in.DropSource,
		BackupID:         &backup.ID,
		Status:           movedom.StatusBackingUp,
		CreatedBy:        in.CreatedBy,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// List returns recent moves.
func (s *Service) List(ctx context.Context) ([]*movedom.Move, error) { return s.repo.List(ctx) }

// Get returns one move.
func (s *Service) Get(ctx context.Context, id string) (*movedom.Move, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.GetByID(ctx, uid)
}

// OnBackupComplete advances a move whose backup just finished (called by the
// operations engine). It is a no-op when the backup is not part of a move.
func (s *Service) OnBackupComplete(ctx context.Context, backupID uuid.UUID, ok bool) {
	m, err := s.repo.GetByBackupID(ctx, backupID)
	if err != nil || m == nil || m.Status != movedom.StatusBackingUp {
		return
	}
	if !ok {
		s.fail(ctx, m, "move: source backup failed")
		return
	}
	job, err := s.backups.Restore(ctx, backupapp.RestoreInput{
		BackupID:         backupID.String(),
		TargetInstanceID: m.TargetInstanceID.String(),
		TargetDatabase:   m.TargetDatabase,
		CreatedBy:        m.CreatedBy,
	})
	if err != nil {
		s.fail(ctx, m, "move: could not start restore: "+err.Error())
		return
	}
	m.RestoreJobID = &job.ID
	m.Status = movedom.StatusRestoring
	if err := s.repo.Update(ctx, m); err != nil {
		slog.Error("move: update after backup", "id", m.ID, "error", err.Error())
	}
}

// OnRestoreComplete finalizes a move whose restore just finished.
func (s *Service) OnRestoreComplete(ctx context.Context, restoreJobID uuid.UUID, ok bool, result json.RawMessage) {
	m, err := s.repo.GetByRestoreJobID(ctx, restoreJobID)
	if err != nil || m == nil || m.Status != movedom.StatusRestoring {
		return
	}
	if !ok {
		s.fail(ctx, m, "move: restore failed")
		return
	}

	// Register the moved database on the target instance (metadata record).
	charset, collation := "utf8mb4", "utf8mb4_unicode_ci"
	if src, e := s.databases.GetByID(ctx, m.SourceDatabaseID); e == nil {
		charset, collation = src.Charset, src.Collation
	}
	if db, e := databasedom.NewDatabase(m.TargetInstanceID, m.TargetDatabase, charset, collation,
		map[string]string{"moved": "true"}, nil); e == nil {
		_ = s.databases.Create(ctx, db) // conflicts (already tracked) are fine
	}

	var res executor.Result
	if json.Unmarshal(result, &res) == nil && res.TableCount > 0 {
		m.TableCount = &res.TableCount
	}
	m.Status = movedom.StatusCompleted
	if err := s.repo.Update(ctx, m); err != nil {
		slog.Error("move: update after restore", "id", m.ID, "error", err.Error())
	}

	// Cutover: optionally drop the source database now that the copy is verified.
	if m.DropSource {
		if err := s.dbService.Delete(ctx, m.SourceDatabaseID.String(), true, m.CreatedBy); err != nil {
			slog.Warn("move: could not drop source", "id", m.ID, "error", err.Error())
		}
	}
}

func (s *Service) fail(ctx context.Context, m *movedom.Move, msg string) {
	m.Status = movedom.StatusFailed
	m.Error = &msg
	if err := s.repo.Update(ctx, m); err != nil {
		slog.Error("move: mark failed", "id", m.ID, "error", err.Error())
	}
}
