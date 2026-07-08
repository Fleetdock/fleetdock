// Package operationapp is the operations (jobs) engine: it records
// operations, routes them to their executor (agent or control plane),
// enriches payloads with credentials and presigned URLs at claim time, and
// applies side effects when operations complete.
package operationapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	backupdom "github.com/TajBrains/db-manager/backend/internal/domain/backup"
	backupdestdom "github.com/TajBrains/db-manager/backend/internal/domain/backupdest"
	databasedom "github.com/TajBrains/db-manager/backend/internal/domain/database"
	instancedom "github.com/TajBrains/db-manager/backend/internal/domain/instance"
	jobdom "github.com/TajBrains/db-manager/backend/internal/domain/job"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
	"github.com/TajBrains/db-manager/backend/internal/platform/engine"
	"github.com/TajBrains/db-manager/backend/internal/platform/executor"
	"github.com/TajBrains/db-manager/backend/internal/platform/storage"
)

// DatabaseStatusRepo is the slice of the database repository this service needs.
type DatabaseStatusRepo interface {
	databasedom.Repository
	SetStatus(ctx context.Context, id uuid.UUID, status databasedom.Status) error
}

// SecretsReader decrypts stored secrets.
type SecretsReader interface {
	Get(ctx context.Context, ref string) ([]byte, error)
}

// EventEmitter queues notification events (satisfied by *notificationapp.Service).
type EventEmitter interface {
	Emit(ctx context.Context, eventType, title, message, severity, aggregateType string, aggregateID uuid.UUID)
}

// Mover advances the move-database saga when its sub-jobs complete (satisfied
// by *moveapp.Service). Both hooks receive the completing job so the move state
// can be read from the job's params; they no-op when the job is not a move leg.
type Mover interface {
	OnBackupComplete(ctx context.Context, job *jobdom.Job, ok bool)
	OnRestoreComplete(ctx context.Context, job *jobdom.Job, ok bool, result json.RawMessage)
}

// Service implements the operations engine.
type Service struct {
	jobs      jobdom.Repository
	instances instancedom.Repository
	databases DatabaseStatusRepo
	backups   backupdom.Repository
	dests     backupdestdom.Repository
	secrets   SecretsReader
	notifier  EventEmitter
	mover     Mover
}

// NewService wires the operations engine.
func NewService(jobs jobdom.Repository, instances instancedom.Repository, databases DatabaseStatusRepo,
	backups backupdom.Repository, dests backupdestdom.Repository, secrets SecretsReader) *Service {
	return &Service{jobs: jobs, instances: instances, databases: databases, backups: backups, dests: dests, secrets: secrets}
}

// SetNotifier attaches an event emitter (optional; nil disables events).
func (s *Service) SetNotifier(n EventEmitter) { s.notifier = n }

// SetMover attaches the move-database saga hook (optional).
func (s *Service) SetMover(m Mover) { s.mover = m }

// Params is the stored (non-sensitive) parameter set of an operation.
type Params struct {
	InstanceID    string `json:"instance_id,omitempty"`
	DatabaseID    string `json:"database_id,omitempty"`
	Database      string `json:"database,omitempty"`
	Charset       string `json:"charset,omitempty"`
	Collation     string `json:"collation,omitempty"`
	BackupID      string `json:"backup_id,omitempty"`
	DestinationID string `json:"destination_id,omitempty"`
	Key           string `json:"key,omitempty"`
	// Provisioning (container lifecycle) params.
	Image        string `json:"image,omitempty"`
	Version      string `json:"version,omitempty"`
	RemoveVolume bool   `json:"remove_volume,omitempty"`
	// Move saga: set on the backup and restore legs of a database move so the
	// move advances via completion hooks without a dedicated table. A move has
	// no grouping job — its backup and restore appear as normal operations.
	MoveTargetInstanceID string `json:"move_target_instance_id,omitempty"` // backup leg only
	MoveTargetDatabase   string `json:"move_target_database,omitempty"`    // backup leg only
	MoveSourceDatabaseID string `json:"move_source_database_id,omitempty"` // both legs; marks a move
	MoveDropSource       bool   `json:"move_drop_source,omitempty"`        // both legs
}

// isProvisionType reports whether a job type is a container lifecycle op.
func isProvisionType(t jobdom.Type) bool {
	switch t {
	case jobdom.TypeProvisionInstance, jobdom.TypeStartInstance, jobdom.TypeStopInstance,
		jobdom.TypeRestartInstance, jobdom.TypeRemoveInstance:
		return true
	}
	return false
}

// Create records a new pending operation.
func (s *Service) Create(ctx context.Context, typ jobdom.Type, resourceType string, resourceID *uuid.UUID,
	serverID *uuid.UUID, params Params, createdBy *uuid.UUID) (*jobdom.Job, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("marshal params: %w", err))
	}
	j := &jobdom.Job{
		ID:           uuid.New(),
		Type:         typ,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Status:       jobdom.StatusPending,
		ServerID:     serverID,
		Params:       raw,
		CreatedBy:    createdBy,
	}
	if err := s.jobs.Create(ctx, j); err != nil {
		return nil, err
	}
	return j, nil
}

// Get returns one operation.
func (s *Service) Get(ctx context.Context, id string) (*jobdom.Job, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.jobs.GetByID(ctx, uid)
}

// AppendLogs persists execution log lines for an operation. It is called by
// the control-plane worker directly and by the agent handler on behalf of a
// remote agent. Logs are best-effort; callers must not fail a job on error.
func (s *Service) AppendLogs(ctx context.Context, jobID uuid.UUID, lines []jobdom.JobLog) error {
	return s.jobs.AppendLogs(ctx, jobID, lines)
}

// Logs returns an operation's log lines with seq > afterSeq (for incremental
// tailing), ordered by seq and capped at limit.
func (s *Service) Logs(ctx context.Context, id string, afterSeq, limit int) ([]jobdom.JobLog, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	if afterSeq < 0 {
		afterSeq = 0
	}
	return s.jobs.ListLogs(ctx, uid, afterSeq, limit)
}

// ListParams filters operation listings.
type ListParams struct {
	Status     string
	Type       string
	ResourceID string
	Limit      int
	Offset     int
}

// ListResult is a page of operations.
type ListResult struct {
	Items  []*jobdom.Job
	Total  int
	Limit  int
	Offset int
}

// List returns filtered, paginated operations.
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
	f := jobdom.ListFilter{Limit: limit, Offset: offset}
	if p.Status != "" {
		st := jobdom.Status(p.Status)
		f.Status = &st
	}
	if p.Type != "" {
		t := jobdom.Type(p.Type)
		f.Type = &t
	}
	if p.ResourceID != "" {
		rid, err := uuid.Parse(p.ResourceID)
		if err != nil {
			return ListResult{}, apperr.Invalid("resource_id", "resource_id must be a valid UUID")
		}
		f.ResourceID = &rid
	}
	page, err := s.jobs.List(ctx, f)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: page.Items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

// Claim atomically claims the next pending operation for an executor
// (serverID nil = the control plane) and returns it with its enriched,
// credential-bearing payload. Enrichment failures fail the job.
func (s *Service) Claim(ctx context.Context, serverID *uuid.UUID) (*jobdom.Job, *executor.Payload, error) {
	j, err := s.jobs.ClaimNext(ctx, serverID)
	if err != nil || j == nil {
		return nil, nil, err
	}
	payload, err := s.buildPayload(ctx, j)
	if err != nil {
		msg := err.Error()
		_ = s.Complete(ctx, j.ID, jobdom.StatusFailed, nil, &msg)
		return nil, nil, err
	}
	if j.Type == jobdom.TypeBackup {
		if bid, e := uuid.Parse(payloadBackupID(j)); e == nil {
			_ = s.backups.MarkRunning(ctx, bid)
		}
	}
	return j, payload, nil
}

func payloadBackupID(j *jobdom.Job) string {
	var p Params
	_ = json.Unmarshal(j.Params, &p)
	return p.BackupID
}

func (s *Service) buildPayload(ctx context.Context, j *jobdom.Job) (*executor.Payload, error) {
	var p Params
	if err := json.Unmarshal(j.Params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}

	// Container lifecycle operations need no DB connection, and the container
	// name derives from the instance id — so remove works even after the
	// instance record is soft-deleted.
	if isProvisionType(j.Type) {
		return s.provisionPayload(ctx, j.Type, p)
	}

	iid, err := uuid.Parse(p.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("operation has no valid instance_id")
	}
	inst, err := s.instances.GetByID(ctx, iid)
	if err != nil {
		return nil, err
	}

	conn, err := s.connParams(ctx, inst)
	if err != nil {
		return nil, err
	}

	payload := &executor.Payload{
		Engine:    string(inst.Engine),
		Conn:      conn,
		Database:  p.Database,
		Charset:   p.Charset,
		Collation: p.Collation,
		BackupID:  p.BackupID,
	}

	switch j.Type {
	case jobdom.TypeBackup:
		dest, client, err := s.storageFor(ctx, p.DestinationID)
		if err != nil {
			return nil, err
		}
		putURL, err := client.PresignPut(ctx, p.Key, 6*time.Hour)
		if err != nil {
			return nil, fmt.Errorf("presign upload: %w", err)
		}
		payload.PutURL = putURL
		payload.StorageURL = fmt.Sprintf("s3://%s/%s", dest.Bucket, p.Key)
	case jobdom.TypeRestore:
		_, client, err := s.storageFor(ctx, p.DestinationID)
		if err != nil {
			return nil, err
		}
		getURL, err := client.PresignGet(ctx, p.Key, 6*time.Hour)
		if err != nil {
			return nil, fmt.Errorf("presign download: %w", err)
		}
		payload.GetURL = getURL
		// Attach the expected checksum so the executor can verify integrity
		// before restoring.
		if bid, e := uuid.Parse(p.BackupID); e == nil {
			if b, e := s.backups.GetByID(ctx, bid); e == nil && b.Checksum != nil {
				payload.Checksum = *b.Checksum
			}
		}
	}
	return payload, nil
}

// provisionPayload builds the Docker lifecycle payload for the agent. The
// container/volume name derives deterministically from the instance id, so
// start/stop/restart/remove need no live instance row. Only provisioning loads
// the instance (for its port + root password).
func (s *Service) provisionPayload(ctx context.Context, typ jobdom.Type, p Params) (*executor.Payload, error) {
	if p.InstanceID == "" {
		return nil, fmt.Errorf("operation has no instance_id")
	}
	containerName := "dbm-" + p.InstanceID
	spec := &executor.ProvisionSpec{
		ContainerName: containerName,
		Volume:        containerName,
		RemoveVolume:  p.RemoveVolume,
	}
	engineName := ""
	if typ == jobdom.TypeProvisionInstance {
		iid, err := uuid.Parse(p.InstanceID)
		if err != nil {
			return nil, fmt.Errorf("operation has no valid instance_id")
		}
		inst, err := s.instances.GetByID(ctx, iid)
		if err != nil {
			return nil, err
		}
		spec.Image = p.Image
		spec.Version = p.Version
		spec.Port = inst.Port
		engineName = string(inst.Engine)
		if inst.RootSecretRef != nil {
			pw, err := s.secrets.Get(ctx, *inst.RootSecretRef)
			if err != nil {
				return nil, fmt.Errorf("load instance credentials: %w", err)
			}
			spec.RootPassword = string(pw)
		}
	}
	return &executor.Payload{Engine: engineName, Provision: spec}, nil
}

func (s *Service) connParams(ctx context.Context, inst *instancedom.Instance) (engine.ConnParams, error) {
	host := "127.0.0.1" // managed: the agent runs on the instance's server
	if inst.Kind == instancedom.KindExternal && inst.Host != nil {
		host = *inst.Host
	}
	conn := engine.ConnParams{Host: host, Port: inst.Port}
	if inst.Username != nil {
		conn.User = *inst.Username
	}
	if inst.RootSecretRef != nil {
		pw, err := s.secrets.Get(ctx, *inst.RootSecretRef)
		if err != nil {
			return conn, fmt.Errorf("load instance credentials: %w", err)
		}
		conn.Password = string(pw)
	}
	return conn, nil
}

func (s *Service) storageFor(ctx context.Context, destinationID string) (*backupdestdom.Destination, *storage.Client, error) {
	did, err := uuid.Parse(destinationID)
	if err != nil {
		return nil, nil, fmt.Errorf("operation has no valid destination_id")
	}
	dest, err := s.dests.GetByID(ctx, did)
	if err != nil {
		return nil, nil, err
	}
	secretKey, err := s.secrets.Get(ctx, dest.SecretRef)
	if err != nil {
		return nil, nil, fmt.Errorf("load destination credentials: %w", err)
	}
	client, err := storage.New(storage.Config{
		Endpoint:  dest.Endpoint,
		Region:    dest.Region,
		Bucket:    dest.Bucket,
		AccessKey: dest.AccessKeyID,
		SecretKey: string(secretKey),
	})
	if err != nil {
		return nil, nil, err
	}
	return dest, client, nil
}

// Complete finalizes an operation and applies its side effects.
func (s *Service) Complete(ctx context.Context, id uuid.UUID, status jobdom.Status, result json.RawMessage, errMsg *string) error {
	j, err := s.jobs.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.jobs.Complete(ctx, id, status, result, errMsg); err != nil {
		return err
	}

	var p Params
	_ = json.Unmarshal(j.Params, &p)
	ok := status == jobdom.StatusSucceeded

	switch j.Type {
	case jobdom.TypeCreateDatabase:
		if dbID, e := uuid.Parse(p.DatabaseID); e == nil {
			st := databasedom.StatusActive
			if !ok {
				st = databasedom.StatusError
			}
			_ = s.databases.SetStatus(ctx, dbID, st)
		}
	case jobdom.TypeBackup:
		if bid, e := uuid.Parse(p.BackupID); e == nil {
			s.completeBackup(ctx, bid, ok, result, errMsg)
			if !ok && s.notifier != nil {
				msg := "A backup operation failed."
				if errMsg != nil {
					msg = *errMsg
				}
				s.notifier.Emit(ctx, "backup.failed", "Backup failed", msg, "critical", "backup", bid)
			}
			if s.mover != nil {
				s.mover.OnBackupComplete(ctx, j, ok)
			}
		}
	case jobdom.TypeRestore:
		if s.mover != nil {
			s.mover.OnRestoreComplete(ctx, j, ok, result)
		}
		if !ok && s.notifier != nil && j.ResourceID != nil {
			msg := "A restore operation failed."
			if errMsg != nil {
				msg = *errMsg
			}
			s.notifier.Emit(ctx, "restore.failed", "Restore failed", msg, "critical", "backup", *j.ResourceID)
		}
	case jobdom.TypeImportDatabases:
		if ok {
			s.importDiscovered(ctx, p, result)
		}
	case jobdom.TypeDeleteDatabase:
		// Metadata is already soft-deleted when the job was enqueued.
	case jobdom.TypeProvisionInstance, jobdom.TypeStartInstance, jobdom.TypeStopInstance,
		jobdom.TypeRestartInstance, jobdom.TypeRemoveInstance:
		s.completeProvision(ctx, j, ok, result, errMsg)
	}
	return nil
}

// completeProvision applies instance status changes after a container
// lifecycle operation.
func (s *Service) completeProvision(ctx context.Context, j *jobdom.Job, ok bool, result json.RawMessage, errMsg *string) {
	if j.ResourceID == nil {
		return
	}
	id := *j.ResourceID
	if !ok {
		// remove failures leave the record soft-deleted; others go to error.
		if j.Type != jobdom.TypeRemoveInstance {
			_ = s.instances.SetStatus(ctx, id, instancedom.StatusError)
		}
		if s.notifier != nil {
			msg := "A provisioning operation failed."
			if errMsg != nil {
				msg = *errMsg
			}
			s.notifier.Emit(ctx, string(j.Type), "Provisioning failed", msg, "critical", "instance", id)
		}
		return
	}
	switch j.Type {
	case jobdom.TypeProvisionInstance:
		var res executor.Result
		_ = json.Unmarshal(result, &res)
		if res.ContainerID != "" {
			_ = s.instances.SetContainerID(ctx, id, res.ContainerID)
		}
		_ = s.instances.SetStatus(ctx, id, instancedom.StatusRunning)
	case jobdom.TypeStartInstance, jobdom.TypeRestartInstance:
		_ = s.instances.SetStatus(ctx, id, instancedom.StatusRunning)
	case jobdom.TypeStopInstance:
		_ = s.instances.SetStatus(ctx, id, instancedom.StatusStopped)
	case jobdom.TypeRemoveInstance:
		// Record already soft-deleted at enqueue time; nothing more to do.
	}
}

func (s *Service) completeBackup(ctx context.Context, id uuid.UUID, ok bool, result json.RawMessage, errMsg *string) {
	in := backupdom.CompleteInput{Status: backupdom.StatusFailed, Error: errMsg}
	if ok {
		var res executor.Result
		_ = json.Unmarshal(result, &res)
		in.Status = backupdom.StatusCompleted
		if res.SizeBytes > 0 {
			in.SizeBytes = &res.SizeBytes
		}
		if res.Checksum != "" {
			in.Checksum = &res.Checksum
		}
		if res.StorageURL != "" {
			in.StorageURL = &res.StorageURL
		}
	}
	_ = s.backups.Complete(ctx, id, in)
}

func (s *Service) importDiscovered(ctx context.Context, p Params, result json.RawMessage) {
	iid, err := uuid.Parse(p.InstanceID)
	if err != nil {
		return
	}
	var res executor.Result
	if err := json.Unmarshal(result, &res); err != nil {
		return
	}
	for _, d := range res.Databases {
		db, err := databasedom.NewDatabase(iid, d.Name, d.Charset, d.Collation,
			map[string]string{"imported": "true"}, nil)
		if err != nil {
			continue
		}
		_ = s.databases.Create(ctx, db) // conflicts (already tracked) are fine
	}
}

// PruneExpiredBackups deletes stored objects for completed backups past their
// retention boundary and marks them expired. It processes up to `limit` per
// call and reports how many were pruned.
func (s *Service) PruneExpiredBackups(ctx context.Context, limit int) (int, error) {
	expired, err := s.backups.ListExpired(ctx, time.Now(), limit)
	if err != nil {
		return 0, err
	}
	pruned := 0
	for _, b := range expired {
		dest, client, err := s.storageFor(ctx, b.DestinationID.String())
		if err != nil {
			slog.Warn("prune: storage unavailable", "backup", b.ID, "error", err.Error())
			continue
		}
		key, err := keyFromStorageURL(b.StorageURL, dest.Bucket)
		if err != nil {
			slog.Warn("prune: bad storage url", "backup", b.ID, "error", err.Error())
			continue
		}
		if err := client.Delete(ctx, key); err != nil {
			slog.Warn("prune: delete object", "backup", b.ID, "error", err.Error())
			continue
		}
		if err := s.backups.MarkExpired(ctx, b.ID); err != nil {
			slog.Warn("prune: mark expired", "backup", b.ID, "error", err.Error())
			continue
		}
		pruned++
	}
	return pruned, nil
}

func keyFromStorageURL(storageURL, bucket string) (string, error) {
	want := "s3://" + bucket + "/"
	if !strings.HasPrefix(storageURL, want) {
		return "", fmt.Errorf("storage url %q does not match bucket %q", storageURL, bucket)
	}
	return strings.TrimPrefix(storageURL, want), nil
}

// UpdateProgress records executor progress.
func (s *Service) UpdateProgress(ctx context.Context, id uuid.UUID, progress int) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	return s.jobs.UpdateProgress(ctx, id, progress)
}
