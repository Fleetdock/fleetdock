// Package backupapp implements backup and restore use cases on top of the
// operations engine.
package backupapp

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	operationapp "github.com/TajBrains/db-manager/backend/internal/app/operation"
	backupdom "github.com/TajBrains/db-manager/backend/internal/domain/backup"
	backupdestdom "github.com/TajBrains/db-manager/backend/internal/domain/backupdest"
	databasedom "github.com/TajBrains/db-manager/backend/internal/domain/database"
	instancedom "github.com/TajBrains/db-manager/backend/internal/domain/instance"
	jobdom "github.com/TajBrains/db-manager/backend/internal/domain/job"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// Service implements backup use cases.
type Service struct {
	backups   backupdom.Repository
	databases databasedom.Repository
	instances instancedom.Repository
	dests     backupdestdom.Repository
	ops       *operationapp.Service
}

// NewService wires the backup service.
func NewService(backups backupdom.Repository, databases databasedom.Repository,
	instances instancedom.Repository, dests backupdestdom.Repository, ops *operationapp.Service) *Service {
	return &Service{backups: backups, databases: databases, instances: instances, dests: dests, ops: ops}
}

// MoveSpec, when set on a backup trigger, marks the backup as the first leg of
// a database move and carries the restore target, so the move advances via
// completion hooks without a dedicated table. The backup and restore appear as
// ordinary operations.
type MoveSpec struct {
	TargetInstanceID string
	TargetDatabase   string
	DropSource       bool
}

// TriggerInput starts a manual backup.
type TriggerInput struct {
	DatabaseID    string
	DestinationID string
	Move          *MoveSpec // set only for the backup leg of a move
	CreatedBy     *uuid.UUID
}

// Trigger creates a manual backup record and its operation.
func (s *Service) Trigger(ctx context.Context, in TriggerInput) (*backupdom.Backup, *jobdom.Job, error) {
	dbID, err := uuid.Parse(in.DatabaseID)
	if err != nil {
		return nil, nil, apperr.Invalid("database_id", "database_id must be a valid UUID")
	}
	destID, err := uuid.Parse(in.DestinationID)
	if err != nil {
		return nil, nil, apperr.Invalid("destination_id", "destination_id must be a valid UUID")
	}
	return s.trigger(ctx, dbID, destID, "manual", nil, nil, in.Move, in.CreatedBy)
}

// TriggerScheduled creates a scheduled backup with a retention boundary. It is
// called by the scheduler and returns just the operation error (the created
// records are internal bookkeeping).
func (s *Service) TriggerScheduled(ctx context.Context, databaseID, destinationID, scheduleID uuid.UUID, retentionDays int, createdBy *uuid.UUID) error {
	expiresAt := time.Now().Add(time.Duration(retentionDays) * 24 * time.Hour)
	_, _, err := s.trigger(ctx, databaseID, destinationID, "scheduled", &scheduleID, &expiresAt, nil, createdBy)
	return err
}

func (s *Service) trigger(ctx context.Context, dbID, destID uuid.UUID, kind string, scheduleID *uuid.UUID, expiresAt *time.Time, move *MoveSpec, createdBy *uuid.UUID) (*backupdom.Backup, *jobdom.Job, error) {
	db, err := s.databases.GetByID(ctx, dbID)
	if err != nil {
		return nil, nil, err
	}
	inst, err := s.instances.GetByID(ctx, db.InstanceID)
	if err != nil {
		return nil, nil, err
	}
	if !inst.HasCredentials() {
		return nil, nil, apperr.Invalid("database_id",
			"the instance has no admin credentials; add a username/password to the instance to enable backups")
	}
	dest, err := s.dests.GetByID(ctx, destID)
	if err != nil {
		return nil, nil, err
	}

	backupID := uuid.New()
	key := backupKey(dest.Prefix, db.Name, backupID)

	params := operationapp.Params{
		InstanceID:    inst.ID.String(),
		DatabaseID:    db.ID.String(),
		Database:      db.Name,
		BackupID:      backupID.String(),
		DestinationID: dest.ID.String(),
		Key:           key,
	}
	if move != nil {
		params.MoveSourceDatabaseID = db.ID.String()
		params.MoveTargetInstanceID = move.TargetInstanceID
		params.MoveTargetDatabase = move.TargetDatabase
		params.MoveDropSource = move.DropSource
	}
	job, err := s.ops.Create(ctx, jobdom.TypeBackup, "backup", &backupID, executorFor(inst), params, createdBy)
	if err != nil {
		return nil, nil, err
	}

	b := &backupdom.Backup{
		ID:            backupID,
		DatabaseID:    db.ID,
		JobID:         &job.ID,
		ScheduleID:    scheduleID,
		DestinationID: &dest.ID,
		Type:          kind,
		Engine:        "mariadb-dump",
		Status:        backupdom.StatusPending,
		ExpiresAt:     expiresAt,
		CreatedBy:     createdBy,
	}
	if err := s.backups.Create(ctx, b); err != nil {
		return nil, nil, err
	}
	return b, job, nil
}

// RestoreInput restores a completed backup, optionally into a different
// instance and/or database name (which is how databases move between servers).
type RestoreInput struct {
	BackupID         string
	TargetInstanceID string // empty = original instance
	TargetDatabase   string // empty = original name
	// Move saga: set on the restore leg of a database move so the completion
	// hook can register the target database and optionally drop the source.
	MoveSourceDatabaseID string
	MoveDropSource       bool
	CreatedBy            *uuid.UUID
}

// Restore creates a restore operation for a completed backup.
func (s *Service) Restore(ctx context.Context, in RestoreInput) (*jobdom.Job, error) {
	bid, err := uuid.Parse(in.BackupID)
	if err != nil {
		return nil, apperr.Invalid("backup_id", "backup_id must be a valid UUID")
	}
	b, err := s.backups.GetByID(ctx, bid)
	if err != nil {
		return nil, err
	}
	if b.Status != backupdom.StatusCompleted || b.StorageURL == nil || b.DestinationID == nil {
		return nil, apperr.Invalid("backup_id", "backup is not completed or has no stored artifact")
	}

	db, err := s.databases.GetByID(ctx, b.DatabaseID)
	targetName := in.TargetDatabase
	charset, collation := "utf8mb4", "utf8mb4_unicode_ci"
	if err == nil {
		if targetName == "" {
			targetName = db.Name
		}
		charset, collation = db.Charset, db.Collation
	} else if targetName == "" {
		return nil, apperr.Invalid("target_database", "target_database is required (original database record is gone)")
	}

	instID := ""
	if in.TargetInstanceID != "" {
		instID = in.TargetInstanceID
	} else if db != nil {
		instID = db.InstanceID.String()
	}
	iid, err := uuid.Parse(instID)
	if err != nil {
		return nil, apperr.Invalid("target_instance_id", "target_instance_id must be a valid UUID")
	}
	inst, err := s.instances.GetByID(ctx, iid)
	if err != nil {
		return nil, err
	}
	if !inst.HasCredentials() {
		return nil, apperr.Invalid("target_instance_id", "the target instance has no admin credentials")
	}

	dest, err := s.dests.GetByID(ctx, *b.DestinationID)
	if err != nil {
		return nil, err
	}
	key, err := keyFromStorageURL(*b.StorageURL, dest.Bucket)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	return s.ops.Create(ctx, jobdom.TypeRestore, "backup", &b.ID, executorFor(inst), operationapp.Params{
		InstanceID:           inst.ID.String(),
		Database:             targetName,
		Charset:              charset,
		Collation:            collation,
		BackupID:             b.ID.String(),
		DestinationID:        dest.ID.String(),
		Key:                  key,
		MoveSourceDatabaseID: in.MoveSourceDatabaseID,
		MoveDropSource:       in.MoveDropSource,
	}, in.CreatedBy)
}

// ListParams filters backup listings.
type ListParams struct {
	DatabaseID string
	Limit      int
	Offset     int
}

// ListResult is a page of backups.
type ListResult struct {
	Items  []*backupdom.Backup
	Total  int
	Limit  int
	Offset int
}

// List returns filtered, paginated backups.
func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}
	f := backupdom.ListFilter{Limit: limit, Offset: offset}
	if p.DatabaseID != "" {
		did, err := uuid.Parse(p.DatabaseID)
		if err != nil {
			return ListResult{}, apperr.Invalid("database_id", "database_id must be a valid UUID")
		}
		f.DatabaseID = &did
	}
	page, err := s.backups.List(ctx, f)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: page.Items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

// Get returns one backup.
func (s *Service) Get(ctx context.Context, id string) (*backupdom.Backup, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.backups.GetByID(ctx, uid)
}

// executorFor picks the executor: managed instances run on their server's
// agent, external instances run on the control plane.
func executorFor(inst *instancedom.Instance) *uuid.UUID {
	if inst.Kind == instancedom.KindManaged {
		return inst.ServerID
	}
	return nil
}

func backupKey(prefix, dbName string, id uuid.UUID) string {
	return path.Join(prefix, "backups", dbName, id.String()+".sql.gz")
}

func keyFromStorageURL(storageURL, bucket string) (string, error) {
	want := "s3://" + bucket + "/"
	if !strings.HasPrefix(storageURL, want) {
		return "", fmt.Errorf("storage url %q does not match destination bucket %q", storageURL, bucket)
	}
	return strings.TrimPrefix(storageURL, want), nil
}
