// Package destinationapp manages backup storage destinations (S3/R2/...).
// Secret keys are envelope-encrypted; the API never returns them.
package destinationapp

import (
	"context"
	"strings"

	"github.com/google/uuid"

	backupdestdom "github.com/TajBrains/fleetdock/backend/internal/domain/backupdest"
	secretdom "github.com/TajBrains/fleetdock/backend/internal/domain/secret"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
	"github.com/TajBrains/fleetdock/backend/internal/platform/storage"
)

// Secrets is the secret store surface this service needs.
type Secrets interface {
	Put(ctx context.Context, ref string, kind secretdom.Kind, plaintext []byte) error
	Get(ctx context.Context, ref string) ([]byte, error)
	Delete(ctx context.Context, ref string) error
}

// Service implements backup-destination use cases.
type Service struct {
	repo    backupdestdom.Repository
	secrets Secrets
}

// NewService wires the service.
func NewService(repo backupdestdom.Repository, secrets Secrets) *Service {
	return &Service{repo: repo, secrets: secrets}
}

// CreateInput describes a new destination.
type CreateInput struct {
	Name            string
	Provider        string
	Bucket          string
	Region          string
	Endpoint        string
	Prefix          string
	AccessKeyID     string
	SecretAccessKey string
	CreatedBy       *uuid.UUID
}

// Create validates, encrypts the secret key and persists the destination.
func (s *Service) Create(ctx context.Context, in CreateInput) (*backupdestdom.Destination, error) {
	if strings.TrimSpace(in.SecretAccessKey) == "" {
		return nil, apperr.Invalid("secret_access_key", "secret_access_key is required")
	}
	d, err := backupdestdom.New(in.Name, backupdestdom.Provider(in.Provider),
		in.Bucket, in.Region, in.Endpoint, in.Prefix, in.AccessKeyID, in.CreatedBy)
	if err != nil {
		return nil, err
	}
	d.SecretRef = "backup-destination/" + d.ID.String()
	if err := s.secrets.Put(ctx, d.SecretRef, secretdom.KindS3Credential, []byte(in.SecretAccessKey)); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, d); err != nil {
		_ = s.secrets.Delete(ctx, d.SecretRef)
		return nil, err
	}
	return d, nil
}

// List returns all destinations.
func (s *Service) List(ctx context.Context) ([]*backupdestdom.Destination, error) {
	return s.repo.List(ctx)
}

// Get returns a destination by id.
func (s *Service) Get(ctx context.Context, id string) (*backupdestdom.Destination, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.GetByID(ctx, uid)
}

// UpdateInput describes changes to an existing destination.
// SecretAccessKey is optional; when empty the stored secret is kept.
type UpdateInput struct {
	ID              string
	Name            string
	Provider        string
	Bucket          string
	Region          string
	Endpoint        string
	Prefix          string
	AccessKeyID     string
	SecretAccessKey string
}

// Update validates, optionally rotates the secret key, and persists changes.
func (s *Service) Update(ctx context.Context, in UpdateInput) (*backupdestdom.Destination, error) {
	d, err := s.Get(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := d.Apply(in.Name, backupdestdom.Provider(in.Provider),
		in.Bucket, in.Region, in.Endpoint, in.Prefix, in.AccessKeyID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.SecretAccessKey) != "" {
		if err := s.secrets.Put(ctx, d.SecretRef, secretdom.KindS3Credential, []byte(in.SecretAccessKey)); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Delete soft-deletes a destination (existing backups keep their rows).
func (s *Service) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.SoftDelete(ctx, uid)
}

// Test verifies bucket access with the stored credentials.
func (s *Service) Test(ctx context.Context, id string) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	secretKey, err := s.secrets.Get(ctx, d.SecretRef)
	if err != nil {
		return err
	}
	client, err := storage.New(storage.Config{
		Endpoint: d.Endpoint, Region: d.Region, Bucket: d.Bucket,
		AccessKey: d.AccessKeyID, SecretKey: string(secretKey),
	})
	if err != nil {
		return apperr.Invalid("endpoint", err.Error())
	}
	if err := client.TestAccess(ctx); err != nil {
		return apperr.Invalid("bucket", "bucket not accessible: "+err.Error())
	}
	return nil
}
