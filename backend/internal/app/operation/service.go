// Package operationapp is the operations (jobs) engine: it records
// operations, routes them to their executor (agent or control plane),
// enriches payloads with credentials and presigned URLs at claim time, and
// applies side effects when operations complete.
package operationapp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	backupdom "github.com/mariadb-cp/db-manager/backend/internal/domain/backup"
	backupdestdom "github.com/mariadb-cp/db-manager/backend/internal/domain/backupdest"
	databasedom "github.com/mariadb-cp/db-manager/backend/internal/domain/database"
	instancedom "github.com/mariadb-cp/db-manager/backend/internal/domain/instance"
	jobdom "github.com/mariadb-cp/db-manager/backend/internal/domain/job"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/engine"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/executor"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/storage"
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

// Service implements the operations engine.
type Service struct {
	jobs      jobdom.Repository
	instances instancedom.Repository
	databases DatabaseStatusRepo
	backups   backupdom.Repository
	dests     backupdestdom.Repository
	secrets   SecretsReader
}

// NewService wires the operations engine.
func NewService(jobs jobdom.Repository, instances instancedom.Repository, databases DatabaseStatusRepo,
	backups backupdom.Repository, dests backupdestdom.Repository, secrets SecretsReader) *Service {
	return &Service{jobs: jobs, instances: instances, databases: databases, backups: backups, dests: dests, secrets: secrets}
}

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
	}
	return payload, nil
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
		}
	case jobdom.TypeImportDatabases:
		if ok {
			s.importDiscovered(ctx, p, result)
		}
	}
	return nil
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
