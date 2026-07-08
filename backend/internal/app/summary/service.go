// Package summaryapp implements the overview-dashboard use case.
package summaryapp

import (
	"context"

	statsdom "github.com/TajBrains/db-manager/backend/internal/domain/stats"
)

// Service returns aggregate control-plane statistics.
type Service struct {
	repo statsdom.Repository
}

// NewService wires the summary service.
func NewService(repo statsdom.Repository) *Service { return &Service{repo: repo} }

// Get returns the current fleet summary.
func (s *Service) Get(ctx context.Context) (statsdom.Summary, error) {
	return s.repo.Summary(ctx)
}
