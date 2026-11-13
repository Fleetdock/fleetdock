package dbadminapp

import (
	"context"
	"testing"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// MySQL and MariaDB have no schemas: the schema calls say so instead of
// running anything.
func TestSchemas_EngineWithoutSchemasIsRefused(t *testing.T) {
	svc, acc, eng := fixture(t, databasedom.StatusActive)
	ctx := context.Background()
	id := svc.databases.(oneDatabase).db.ID.String()
	for name, err := range map[string]error{
		"create": svc.CreateSchema(ctx, id, "staging", ""),
		"drop":   svc.DropSchema(ctx, id, "staging", false),
	} {
		if apperr.KindOf(err) != apperr.KindInvalid {
			t.Errorf("%s: err = %v, want invalid", name, err)
		}
	}
	if _, err := svc.SchemaDependents(ctx, id, "staging"); apperr.KindOf(err) != apperr.KindInvalid {
		t.Errorf("dependents: err = %v, want invalid", err)
	}
	if acc.reapply != 0 || len(eng.batches) != 0 {
		t.Errorf("ran something: reapply %d, batches %d", acc.reapply, len(eng.batches))
	}
}

// A database that is being moved, locked or deleted keeps its schemas.
func TestSchemas_InactiveDatabaseRefusesChanges(t *testing.T) {
	svc, _, _ := fixture(t, databasedom.StatusLocked)
	id := svc.databases.(oneDatabase).db.ID.String()
	if err := svc.CreateSchema(context.Background(), id, "staging", ""); apperr.KindOf(err) != apperr.KindConflict {
		t.Errorf("err = %v, want conflict", err)
	}
}
