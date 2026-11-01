// Package catalogapp keeps the fleet-wide schema catalog fresh and answers
// schema searches from it.
package catalogapp

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	catalogdom "github.com/Fleetdock/fleetdock/backend/internal/domain/catalog"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// ColumnSource reads a database's tables and columns.
type ColumnSource interface {
	AdminColumns(ctx context.Context, databaseID uuid.UUID) (*engine.ColumnCatalog, error)
}

// Service refreshes and searches the catalog.
type Service struct {
	repo     catalogdom.Repository
	source   ColumnSource
	interval time.Duration
	retry    time.Duration
	timeout  time.Duration
	parallel int
	batch    int
	now      func() time.Time
}

// NewService builds the service. interval is how old a database's catalog
// may get before it is refreshed.
func NewService(repo catalogdom.Repository, source ColumnSource, interval time.Duration) *Service {
	return &Service{
		repo:     repo,
		source:   source,
		interval: interval,
		retry:    10 * time.Minute,
		timeout:  60 * time.Second,
		parallel: 4,
		batch:    50,
		now:      time.Now,
	}
}

// Enabled reports whether background refreshes run at all.
func (s *Service) Enabled() bool { return s.interval > 0 }

// RefreshDue refreshes up to one batch of databases whose catalog is stale,
// a few at a time. It returns how many it attempted.
func (s *Service) RefreshDue(ctx context.Context) int {
	if !s.Enabled() {
		return 0
	}
	now := s.now()
	ids, err := s.repo.ListDue(ctx, now.Add(-s.interval), now.Add(-s.retry), s.batch)
	if err != nil {
		slog.Error("list catalog refreshes due", "error", err.Error())
		return 0
	}
	sem := make(chan struct{}, s.parallel)
	var wg sync.WaitGroup
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(id uuid.UUID) {
			defer func() { <-sem; wg.Done() }()
			s.RefreshOne(ctx, id)
		}(id)
	}
	wg.Wait()
	return len(ids)
}

// RefreshOne reads one database's catalog and stores it. A failure keeps
// the previous catalog and is recorded for the next retry.
func (s *Service) RefreshOne(ctx context.Context, databaseID uuid.UUID) {
	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	cat, err := s.source.AdminColumns(cctx, databaseID)
	if err != nil {
		if ferr := s.repo.Failed(ctx, databaseID, err.Error(), s.now()); ferr != nil {
			slog.Error("record catalog failure", "database_id", databaseID, "error", ferr.Error())
		}
		return
	}
	if err := s.repo.Replace(ctx, databaseID, cat, s.now()); err != nil {
		slog.Error("store catalog", "database_id", databaseID, "error", err.Error())
	}
}

// SchemaChanged refreshes a database in the background after DDL made
// through Fleetdock, so search reflects it before the next periodic pass.
func (s *Service) SchemaChanged(databaseID uuid.UUID) {
	go s.RefreshOne(context.Background(), databaseID)
}

// Search returns raw matches (not yet filtered by what the caller may see).
func (s *Service) Search(ctx context.Context, q string, limit int) ([]catalogdom.Hit, error) {
	return s.repo.Search(ctx, q, limit)
}
