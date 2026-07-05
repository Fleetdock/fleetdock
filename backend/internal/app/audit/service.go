// Package auditapp implements the audit-log use cases: recording actions and
// reading the log.
package auditapp

import (
	"context"

	"github.com/google/uuid"

	auditdom "github.com/mariadb-cp/db-manager/backend/internal/domain/audit"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// Service implements audit use cases.
type Service struct {
	repo auditdom.Repository
}

// NewService wires the audit service.
func NewService(repo auditdom.Repository) *Service { return &Service{repo: repo} }

// RecordInput describes an action to record.
type RecordInput struct {
	ActorType    auditdom.ActorType
	ActorID      *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Metadata     map[string]any
}

// Record appends an entry to the audit log.
func (s *Service) Record(ctx context.Context, in RecordInput) error {
	if in.ActorType == "" {
		in.ActorType = auditdom.ActorSystem
	}
	if in.Metadata == nil {
		in.Metadata = map[string]any{}
	}
	return s.repo.Append(ctx, &auditdom.Entry{
		ActorType:    in.ActorType,
		ActorID:      in.ActorID,
		Action:       in.Action,
		ResourceType: in.ResourceType,
		ResourceID:   in.ResourceID,
		Metadata:     in.Metadata,
	})
}

// ListParams filters audit listings.
type ListParams struct {
	ActorID      string
	ResourceType string
	ResourceID   string
	Limit        int
	Offset       int
}

// ListResult is a page of audit entries.
type ListResult struct {
	Items  []*auditdom.Entry
	Total  int
	Limit  int
	Offset int
}

// List returns filtered, paginated audit entries.
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
	f := auditdom.ListFilter{ResourceType: p.ResourceType, Limit: limit, Offset: offset}
	if p.ActorID != "" {
		id, err := uuid.Parse(p.ActorID)
		if err != nil {
			return ListResult{}, apperr.Invalid("actor_id", "actor_id must be a valid UUID")
		}
		f.ActorID = &id
	}
	if p.ResourceID != "" {
		id, err := uuid.Parse(p.ResourceID)
		if err != nil {
			return ListResult{}, apperr.Invalid("resource_id", "resource_id must be a valid UUID")
		}
		f.ResourceID = &id
	}
	page, err := s.repo.List(ctx, f)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: page.Items, Total: page.Total, Limit: limit, Offset: offset}, nil
}
