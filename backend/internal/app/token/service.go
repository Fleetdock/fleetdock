// Package tokenapp holds API-token use cases.
package tokenapp

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	tokendom "github.com/TajBrains/db-manager/backend/internal/domain/token"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
	"github.com/TajBrains/db-manager/backend/internal/platform/auth"
)

// Service implements API-token management.
type Service struct {
	repo tokendom.Repository
}

// NewService wires the token service.
func NewService(repo tokendom.Repository) *Service { return &Service{repo: repo} }

// CreateInput is the command to mint a new token.
type CreateInput struct {
	UserID    uuid.UUID
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

// Created carries the persisted token plus its one-time plaintext secret.
type Created struct {
	Token     *tokendom.Token
	Plaintext string
}

// Create mints a token, stores only its hash, and returns the plaintext once.
func (s *Service) Create(ctx context.Context, in CreateInput) (Created, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Created{}, apperr.Invalid("name", "token name is required")
	}
	full, prefix, hash, err := auth.GenerateToken()
	if err != nil {
		return Created{}, apperr.Internal(err)
	}
	if in.Scopes == nil {
		in.Scopes = []string{}
	}
	t := &tokendom.Token{
		ID:        uuid.New(),
		UserID:    in.UserID,
		Name:      in.Name,
		Prefix:    prefix,
		Scopes:    in.Scopes,
		ExpiresAt: in.ExpiresAt,
	}
	if err := s.repo.Create(ctx, t, hash); err != nil {
		return Created{}, err
	}
	return Created{Token: t, Plaintext: full}, nil
}

// List returns the caller's tokens (without secrets).
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*tokendom.Token, error) {
	return s.repo.ListByUser(ctx, userID)
}

// Revoke revokes one of the caller's tokens.
func (s *Service) Revoke(ctx context.Context, userID uuid.UUID, id string) error {
	tid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.Revoke(ctx, userID, tid)
}
