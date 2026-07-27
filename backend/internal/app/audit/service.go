// Package auditapp records append-only audit events.
package auditapp

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
)

// Service appends audit events.
type Service struct {
	repo auditdom.Repository
}

// NewService wires the audit service.
func NewService(repo auditdom.Repository) *Service {
	return &Service{repo: repo}
}

// Record stores one audit event with secret-free metadata.
func (s *Service) Record(ctx context.Context, actor *uuid.UUID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any) error {
	meta, err := json.Marshal(metadata)
	if err != nil {
		meta = []byte("{}")
	}
	return s.repo.Append(ctx, &auditdom.Event{
		ID:           uuid.New(),
		ActorUserID:  actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     meta,
	})
}
