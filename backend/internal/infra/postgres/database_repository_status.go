package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// SetStatus transitions a database's lifecycle status (used by the
// operations engine when async jobs finish).
func (r *DatabaseRepository) SetStatus(ctx context.Context, id uuid.UUID, status databasedom.Status) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE databases SET status = $2, version = version + 1 WHERE id = $1 AND deleted_at IS NULL`,
		id, string(status))
	if err != nil {
		return apperr.Internal(fmt.Errorf("set database status: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("database not found")
	}
	return nil
}
