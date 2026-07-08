// Package moveapp implements the move-database action on top of the existing
// backup + restore operations. A move is not a grouped/parent job: it triggers
// a backup of the source and, when that completes, a restore into the target —
// both appear as ordinary operations. The saga state (target, cutover) rides in
// the operations' own params, so there is no dedicated moves table.
package moveapp

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	backupapp "github.com/TajBrains/db-manager/backend/internal/app/backup"
	databaseapp "github.com/TajBrains/db-manager/backend/internal/app/database"
	operationapp "github.com/TajBrains/db-manager/backend/internal/app/operation"
	databasedom "github.com/TajBrains/db-manager/backend/internal/domain/database"
	instancedom "github.com/TajBrains/db-manager/backend/internal/domain/instance"
	jobdom "github.com/TajBrains/db-manager/backend/internal/domain/job"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// Service implements the move-database action.
type Service struct {
	databases databasedom.Repository
	instances instancedom.Repository
	backups   *backupapp.Service
	dbService *databaseapp.Service
}

// NewService wires the move service.
func NewService(databases databasedom.Repository, instances instancedom.Repository,
	backups *backupapp.Service, dbService *databaseapp.Service) *Service {
	return &Service{databases: databases, instances: instances, backups: backups, dbService: dbService}
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

// Start validates the request and kicks off a backup of the source carrying the
// move target in its params. It returns the backup operation; the move advances
// automatically as the backup and restore operations complete.
func (s *Service) Start(ctx context.Context, in StartInput) (*jobdom.Job, error) {
	srcID, err := uuid.Parse(in.SourceDatabaseID)
	if err != nil {
		return nil, apperr.Invalid("source_database_id", "source_database_id must be a valid UUID")
	}
	tgtInstID, err := uuid.Parse(in.TargetInstanceID)
	if err != nil {
		return nil, apperr.Invalid("target_instance_id", "target_instance_id must be a valid UUID")
	}
	if _, err := uuid.Parse(in.DestinationID); err != nil {
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

	_, job, err := s.backups.Trigger(ctx, backupapp.TriggerInput{
		DatabaseID:    srcID.String(),
		DestinationID: in.DestinationID,
		Move: &backupapp.MoveSpec{
			TargetInstanceID: tgtInstID.String(),
			TargetDatabase:   targetName,
			DropSource:       in.DropSource,
		},
		CreatedBy: in.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	return job, nil
}

// OnBackupComplete advances a move whose backup just finished (called by the
// operations engine). It is a no-op when the backup is not a move leg.
func (s *Service) OnBackupComplete(ctx context.Context, job *jobdom.Job, ok bool) {
	var p operationapp.Params
	if json.Unmarshal(job.Params, &p) != nil || p.MoveTargetInstanceID == "" {
		return
	}
	if !ok {
		// The failed backup operation is already visible in Operations; there is
		// nothing to restore.
		return
	}
	if _, err := s.backups.Restore(ctx, backupapp.RestoreInput{
		BackupID:             p.BackupID,
		TargetInstanceID:     p.MoveTargetInstanceID,
		TargetDatabase:       p.MoveTargetDatabase,
		MoveSourceDatabaseID: p.MoveSourceDatabaseID,
		MoveDropSource:       p.MoveDropSource,
		CreatedBy:            job.CreatedBy,
	}); err != nil {
		slog.Error("move: could not start restore", "backup_job", job.ID, "error", err.Error())
	}
}

// OnRestoreComplete finalizes a move whose restore just finished: it registers
// the moved database on the target and optionally drops the source (cutover).
// It is a no-op when the restore is not a move leg.
func (s *Service) OnRestoreComplete(ctx context.Context, job *jobdom.Job, ok bool, _ json.RawMessage) {
	var p operationapp.Params
	if json.Unmarshal(job.Params, &p) != nil || p.MoveSourceDatabaseID == "" {
		return
	}
	if !ok {
		return
	}
	srcID, err := uuid.Parse(p.MoveSourceDatabaseID)
	if err != nil {
		return
	}
	tgtInstID, err := uuid.Parse(p.InstanceID)
	if err != nil {
		return
	}

	// Register the moved database on the target instance (metadata record).
	charset, collation := "utf8mb4", "utf8mb4_unicode_ci"
	if src, e := s.databases.GetByID(ctx, srcID); e == nil {
		charset, collation = src.Charset, src.Collation
	}
	if db, e := databasedom.NewDatabase(tgtInstID, p.Database, charset, collation,
		map[string]string{"moved": "true"}, nil); e == nil {
		_ = s.databases.Create(ctx, db) // conflicts (already tracked) are fine
	}

	// Cutover: optionally drop the source now that the copy is verified.
	if p.MoveDropSource {
		if err := s.dbService.Delete(ctx, srcID.String(), true, job.CreatedBy); err != nil {
			slog.Warn("move: could not drop source", "source_database", srcID, "error", err.Error())
		}
	}
}
