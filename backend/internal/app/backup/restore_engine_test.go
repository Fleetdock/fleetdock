package backupapp

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	backupdestdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backupdest"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type goneDatabases struct{ databasedom.Repository }

func (goneDatabases) GetByID(context.Context, uuid.UUID) (*databasedom.Database, error) {
	return nil, apperr.NotFound("database not found")
}

type oneInstance struct {
	instancedom.Repository
	inst *instancedom.Instance
}

func (r oneInstance) GetByID(context.Context, uuid.UUID) (*instancedom.Instance, error) {
	return r.inst, nil
}

func TestRestore_RefusesAnotherEngineFamily(t *testing.T) {
	user, ref := "root", "secret-ref"
	url, dest := "s3://bucket/x.sql.gz", uuid.New()
	for _, c := range []struct {
		tool   string
		target instancedom.Engine
		refuse bool
	}{
		{"pg_dump", instancedom.EngineMySQL, true},
		{"mariadb-dump", instancedom.EnginePostgres, true},
		{"mysqldump", instancedom.EnginePostgres, true},
		// Same family: the check lets it through (it then needs the
		// destination, which this test does not provide).
		{"pg_dump", instancedom.EnginePostgres, false},
		{"mysqldump", instancedom.EngineMariaDB, false},
	} {
		bid := uuid.New()
		backups := &fakeBackupRepo{items: map[uuid.UUID]*backupdom.Backup{bid: {
			ID: bid, Status: backupdom.StatusCompleted, Engine: c.tool, StorageURL: &url, DestinationID: &dest,
		}}}
		inst := &instancedom.Instance{ID: uuid.New(), Engine: c.target, Username: &user, RootSecretRef: &ref}
		svc := NewService(backups, goneDatabases{}, oneInstance{inst: inst}, failingDests{}, nil)

		_, err := svc.Restore(context.Background(), RestoreInput{
			BackupID: bid.String(), TargetInstanceID: inst.ID.String(), TargetDatabase: "app",
		})
		refused := err != nil && apperr.KindOf(err) == apperr.KindInvalid && strings.Contains(err.Error(), "can only be restored")
		if refused != c.refuse {
			t.Errorf("%s dump to %s: err = %v, want refused=%v", c.tool, c.target, err, c.refuse)
		}
	}
}

type failingDests struct{ backupdestdom.Repository }

func (failingDests) GetByID(context.Context, uuid.UUID) (*backupdestdom.Destination, error) {
	return nil, apperr.NotFound("destination not found")
}
