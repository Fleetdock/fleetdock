package backupapp

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
)

// listedDatabases answers GetByID and List from items; List mimics the
// repository's substring search on Search.
type listedDatabases struct {
	someDatabases
}

func (r listedDatabases) List(_ context.Context, f databasedom.ListFilter) (databasedom.Page, error) {
	var p databasedom.Page
	for _, d := range r.items {
		if f.InstanceID != nil && d.InstanceID != *f.InstanceID {
			continue
		}
		if f.Search != "" && !strings.Contains(d.Name, f.Search) {
			continue
		}
		p.Items = append(p.Items, d)
	}
	p.Total = len(p.Items)
	return p, nil
}

func TestResolveRestoreTarget(t *testing.T) {
	user, ref := "root", "secret-ref"
	url, dest := "s3://bucket/x.sql.gz", uuid.New()
	inst := &instancedom.Instance{ID: uuid.New(), Engine: instancedom.EngineMySQL, Username: &user, RootSecretRef: &ref}
	src := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "app"}
	billing := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "billing"}
	billingArchive := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "billing_archive"}
	sysDB := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "sys", System: true}
	dbs := listedDatabases{someDatabases{items: map[uuid.UUID]*databasedom.Database{
		src.ID: src, billing.ID: billing, billingArchive.ID: billingArchive, sysDB.ID: sysDB,
	}}}
	bid := uuid.New()
	backups := &fakeBackupRepo{items: map[uuid.UUID]*backupdom.Backup{bid: {
		ID: bid, DatabaseID: src.ID, Status: backupdom.StatusCompleted, Engine: "mariadb-dump", StorageURL: &url, DestinationID: &dest,
	}}}
	svc := NewService(backups, dbs, oneInstance{inst: inst}, failingDests{}, nil)

	for _, c := range []struct {
		name     string
		target   string
		wantName string
		existing *uuid.UUID
		system   bool
	}{
		{"default target is the source", "", "app", &src.ID, false},
		{"another existing database", "billing", "billing", &billing.ID, false},
		{"a new name", "billing_copy", "billing_copy", nil, false},
		{"unregistered system database", "mysql", "mysql", nil, true},
		{"registered system database", "sys", "sys", &sysDB.ID, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := svc.ResolveRestoreTarget(context.Background(), RestoreInput{BackupID: bid.String(), TargetDatabase: c.target})
			if err != nil {
				t.Fatal(err)
			}
			if got.InstanceID != inst.ID || got.Database != c.wantName || got.System != c.system {
				t.Errorf("got %+v", got)
			}
			if (got.Existing == nil) != (c.existing == nil) || (c.existing != nil && *got.Existing != *c.existing) {
				t.Errorf("existing = %v, want %v", got.Existing, c.existing)
			}
		})
	}
}
