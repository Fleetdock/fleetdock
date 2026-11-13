package backupapp

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type someDatabases struct {
	databasedom.Repository
	items map[uuid.UUID]*databasedom.Database
}

func (r someDatabases) GetByID(_ context.Context, id uuid.UUID) (*databasedom.Database, error) {
	if db, ok := r.items[id]; ok {
		return db, nil
	}
	return nil, apperr.NotFound("database not found")
}

// TestCopySchema_Refusals covers what is refused before an operation is
// queued (the service has no operations engine here, so anything that gets
// past the checks would panic).
func TestCopySchema_Refusals(t *testing.T) {
	user, ref := "root", "secret-ref"
	inst := &instancedom.Instance{ID: uuid.New(), Engine: instancedom.EnginePostgres, Username: &user, RootSecretRef: &ref}
	my := &instancedom.Instance{ID: uuid.New(), Engine: instancedom.EngineMySQL, Username: &user, RootSecretRef: &ref}
	src := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "shop", Status: databasedom.StatusActive}
	other := &databasedom.Database{ID: uuid.New(), InstanceID: uuid.New(), Name: "elsewhere", Status: databasedom.StatusActive}
	locked := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "busy", Status: databasedom.StatusLocked}
	dbs := someDatabases{items: map[uuid.UUID]*databasedom.Database{src.ID: src, other.ID: other, locked.ID: locked}}

	for _, c := range []struct {
		name string
		inst *instancedom.Instance
		in   CopySchemaInput
		want string
	}{
		{"system schema", inst, CopySchemaInput{DatabaseID: src.ID.String(), Schema: "pg_catalog", TargetSchema: "x"}, "PostgreSQL's own"},
		{"reserved name", inst, CopySchemaInput{DatabaseID: src.ID.String(), Schema: "sales", TargetSchema: "pg_sales"}, "reserved"},
		{"bad name", inst, CopySchemaInput{DatabaseID: src.ID.String(), Schema: "sales", TargetSchema: "a b"}, "invalid schema name"},
		{"same name", inst, CopySchemaInput{DatabaseID: src.ID.String(), Schema: "sales", TargetSchema: "sales"}, "name of its own"},
		{"mysql", my, CopySchemaInput{DatabaseID: src.ID.String(), Schema: "sales", TargetSchema: "x"}, "only PostgreSQL"},
		{"other instance", inst, CopySchemaInput{DatabaseID: src.ID.String(), Schema: "sales", TargetSchema: "x", TargetDatabaseID: other.ID.String()}, "same instance"},
		{"inactive target", inst, CopySchemaInput{DatabaseID: src.ID.String(), Schema: "sales", TargetSchema: "x", TargetDatabaseID: locked.ID.String()}, "needs it active"},
		{"bad target id", inst, CopySchemaInput{DatabaseID: src.ID.String(), Schema: "sales", TargetSchema: "x", TargetDatabaseID: "nope"}, "target_database_id"},
	} {
		svc := NewService(&fakeBackupRepo{}, dbs, oneInstance{inst: c.inst}, failingDests{}, nil)
		_, err := svc.CopySchema(context.Background(), c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

// TestRestoreSchema_Refusals covers what restore-schema refuses before
// anything is queued, and that whole-database paths refuse schema backups.
func TestRestoreSchema_Refusals(t *testing.T) {
	user, ref := "root", "secret-ref"
	inst := &instancedom.Instance{ID: uuid.New(), Engine: instancedom.EnginePostgres, Username: &user, RootSecretRef: &ref}
	db := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "shop", Status: databasedom.StatusActive}
	url, dest, sales := "s3://bucket/x.sql.gz", uuid.New(), "sales"
	whole, part := uuid.New(), uuid.New()
	backups := &fakeBackupRepo{items: map[uuid.UUID]*backupdom.Backup{
		whole: {ID: whole, DatabaseID: db.ID, Status: backupdom.StatusCompleted, Engine: "pg_dump", StorageURL: &url, DestinationID: &dest},
		part:  {ID: part, DatabaseID: db.ID, Status: backupdom.StatusCompleted, Engine: "pg_dump", StorageURL: &url, DestinationID: &dest, SchemaName: &sales},
	}}
	svc := NewService(backups, someDatabases{items: map[uuid.UUID]*databasedom.Database{db.ID: db}}, oneInstance{inst: inst}, failingDests{}, nil)
	ctx := context.Background()

	for _, c := range []struct {
		name string
		in   RestoreSchemaInput
		want string
	}{
		{"whole-database backup", RestoreSchemaInput{BackupID: whole.String()}, "whole database"},
		{"replace under a new name", RestoreSchemaInput{BackupID: part.String(), TargetSchema: "sales_old", Replace: true}, "own name"},
		{"reserved name", RestoreSchemaInput{BackupID: part.String(), TargetSchema: "pg_x"}, "reserved"},
	} {
		if _, err := svc.RestoreSchema(ctx, c.in); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", c.name, err, c.want)
		}
	}
	if _, err := svc.Restore(ctx, RestoreInput{BackupID: part.String()}); err == nil || !strings.Contains(err.Error(), "restore-schema") {
		t.Errorf("whole restore of a schema backup: %v", err)
	}
	if _, err := svc.Verify(ctx, part.String(), nil); err == nil || !strings.Contains(err.Error(), "one schema") {
		t.Errorf("verify of a schema backup: %v", err)
	}
}
