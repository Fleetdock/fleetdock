// Package backupdest is the domain model for backup storage destinations —
// any S3-compatible object store (AWS S3, Cloudflare R2, MinIO, ...).
package backupdest

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// Provider identifies the storage flavour (drives endpoint defaults).
type Provider string

const (
	ProviderS3           Provider = "s3"
	ProviderR2           Provider = "r2"
	ProviderS3Compatible Provider = "s3_compatible"
)

// Valid reports whether p is a known provider.
func (p Provider) Valid() bool {
	switch p {
	case ProviderS3, ProviderR2, ProviderS3Compatible:
		return true
	}
	return false
}

// Destination is a configured backup target bucket.
type Destination struct {
	ID          uuid.UUID
	Name        string
	Provider    Provider
	Bucket      string
	Region      string
	Endpoint    string
	Prefix      string
	AccessKeyID string
	SecretRef   string
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
	DeletedAt   *time.Time
}

func validateFields(name string, provider Provider, bucket, region, endpoint, prefix, accessKeyID string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 63 {
		return apperr.Invalid("name", "name is required and must be at most 63 characters")
	}
	if !provider.Valid() {
		return apperr.Invalid("provider", "provider must be one of: s3, r2, s3_compatible")
	}
	if strings.TrimSpace(bucket) == "" {
		return apperr.Invalid("bucket", "bucket is required")
	}
	if provider != ProviderS3 && strings.TrimSpace(endpoint) == "" {
		return apperr.Invalid("endpoint", "endpoint is required for non-AWS providers")
	}
	if strings.TrimSpace(accessKeyID) == "" {
		return apperr.Invalid("access_key_id", "access_key_id is required")
	}
	return nil
}

func normalizeFields(bucket, region, endpoint, prefix, accessKeyID string) (string, string, string, string, string) {
	return strings.TrimSpace(bucket),
		strings.TrimSpace(region),
		strings.TrimSpace(strings.TrimSuffix(endpoint, "/")),
		strings.Trim(prefix, "/"),
		strings.TrimSpace(accessKeyID)
}

// New validates input and constructs a Destination (SecretRef set by service).
func New(name string, provider Provider, bucket, region, endpoint, prefix, accessKeyID string, createdBy *uuid.UUID) (*Destination, error) {
	if err := validateFields(name, provider, bucket, region, endpoint, prefix, accessKeyID); err != nil {
		return nil, err
	}
	bucket, region, endpoint, prefix, accessKeyID = normalizeFields(bucket, region, endpoint, prefix, accessKeyID)
	return &Destination{
		ID:          uuid.New(),
		Name:        strings.TrimSpace(name),
		Provider:    provider,
		Bucket:      bucket,
		Region:      region,
		Endpoint:    endpoint,
		Prefix:      prefix,
		AccessKeyID: accessKeyID,
		CreatedBy:   createdBy,
	}, nil
}

// Apply validates and overwrites mutable fields on an existing destination.
func (d *Destination) Apply(name string, provider Provider, bucket, region, endpoint, prefix, accessKeyID string) error {
	if err := validateFields(name, provider, bucket, region, endpoint, prefix, accessKeyID); err != nil {
		return err
	}
	bucket, region, endpoint, prefix, accessKeyID = normalizeFields(bucket, region, endpoint, prefix, accessKeyID)
	d.Name = strings.TrimSpace(name)
	d.Provider = provider
	d.Bucket = bucket
	d.Region = region
	d.Endpoint = endpoint
	d.Prefix = prefix
	d.AccessKeyID = accessKeyID
	return nil
}

// Repository is the persistence port for destinations.
type Repository interface {
	Create(ctx context.Context, d *Destination) error
	GetByID(ctx context.Context, id uuid.UUID) (*Destination, error)
	List(ctx context.Context) ([]*Destination, error)
	Update(ctx context.Context, d *Destination) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
}
