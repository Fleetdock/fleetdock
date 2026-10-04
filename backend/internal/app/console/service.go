// Package consoleapp serves SQL console history and saved queries. Entries
// are private to their user; the caller's identity scopes every operation.
package consoleapp

import (
	"context"
	"strings"

	"github.com/google/uuid"

	consoledom "github.com/Fleetdock/fleetdock/backend/internal/domain/console"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// maxSavedSQL bounds a saved query's text.
const maxSavedSQL = 1 << 20

// Service implements console history and saved-query use cases.
type Service struct {
	repo consoledom.Repository
}

// NewService wires the service.
func NewService(repo consoledom.Repository) *Service { return &Service{repo: repo} }

// History returns the user's recent runs against a database.
func (s *Service) History(ctx context.Context, userID uuid.UUID, databaseID string, limit int) ([]consoledom.HistoryEntry, error) {
	did, err := uuid.Parse(databaseID)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.ListHistory(ctx, userID, did, limit)
}

// SavedInput is a saved query to create or update.
type SavedInput struct {
	Name       string
	SQL        string
	DatabaseID string // optional
}

func (in SavedInput) validate() (string, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 200 {
		return "", "", apperr.Invalid("name", "name must be 1–200 characters")
	}
	if strings.TrimSpace(in.SQL) == "" {
		return "", "", apperr.Invalid("sql", "sql is required")
	}
	if len(in.SQL) > maxSavedSQL {
		return "", "", apperr.Invalid("sql", "sql is too long (max 1 MB)")
	}
	return name, in.SQL, nil
}

// ListSaved returns the user's saved queries; with a database id, those for
// that database plus the user's unscoped ones.
func (s *Service) ListSaved(ctx context.Context, userID uuid.UUID, databaseID string) ([]consoledom.SavedQuery, error) {
	var did *uuid.UUID
	if databaseID != "" {
		id, err := uuid.Parse(databaseID)
		if err != nil {
			return nil, apperr.Invalid("database_id", "database_id must be a valid UUID")
		}
		did = &id
	}
	return s.repo.ListSaved(ctx, userID, did)
}

// CreateSaved saves a query.
func (s *Service) CreateSaved(ctx context.Context, userID uuid.UUID, in SavedInput) (*consoledom.SavedQuery, error) {
	name, sql, err := in.validate()
	if err != nil {
		return nil, err
	}
	q := &consoledom.SavedQuery{UserID: userID, Name: name, SQL: sql}
	if in.DatabaseID != "" {
		id, err := uuid.Parse(in.DatabaseID)
		if err != nil {
			return nil, apperr.Invalid("database_id", "database_id must be a valid UUID")
		}
		q.DatabaseID = &id
	}
	if err := s.repo.CreateSaved(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// UpdateSaved renames or rewrites one of the user's saved queries.
func (s *Service) UpdateSaved(ctx context.Context, userID uuid.UUID, id string, in SavedInput) (*consoledom.SavedQuery, error) {
	qid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	name, sql, err := in.validate()
	if err != nil {
		return nil, err
	}
	return s.repo.UpdateSaved(ctx, userID, qid, name, sql)
}

// DeleteSaved deletes one of the user's saved queries.
func (s *Service) DeleteSaved(ctx context.Context, userID uuid.UUID, id string) error {
	qid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.DeleteSaved(ctx, userID, qid)
}
