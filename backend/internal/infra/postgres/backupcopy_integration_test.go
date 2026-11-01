//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	backupcopydom "github.com/Fleetdock/fleetdock/backend/internal/domain/backupcopy"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func TestIntegrationBackupCopies(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := "root"
	inst, _ := instancedom.NewExternal("primary", instancedom.EngineMariaDB, "11.4", "db.example.com", 3306, &user, nil, nil)
	if err := NewInstanceRepository(pool).Create(ctx, inst); err != nil {
		t.Fatal(err)
	}
	// Discovery of a verification scratch database registers nothing.
	if _, err := NewDatabaseRepository(pool).Reconcile(ctx, inst.ID, []databasedom.Observed{{Name: "shop"}, {Name: "fdv_0123abcd456789ef"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM databases WHERE deleted_at IS NULL`).Scan(&n)
	if n != 1 {
		t.Fatalf("registered %d databases, want 1 (scratch names are reserved)", n)
	}
	var dbID uuid.UUID
	_ = pool.QueryRow(ctx, `SELECT id FROM databases`).Scan(&dbID)
	b := &backupdom.Backup{ID: uuid.New(), DatabaseID: dbID, Type: "manual", Engine: "mariadb-dump", Status: backupdom.StatusCompleted}
	if err := NewBackupRepository(pool).Create(ctx, b); err != nil {
		t.Fatal(err)
	}

	repo := NewBackupCopyRepository(pool)
	mk := func() *backupcopydom.Copy {
		return &backupcopydom.Copy{DatabaseID: dbID, BackupID: b.ID, InstanceID: inst.ID, ScratchName: databasedom.ScratchName("fds", uuid.New()),
			Status: backupcopydom.StatusRestoring, TTL: time.Hour}
	}
	c := mk()
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, mk()); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("second live copy: %v", err)
	}
	now := time.Now()
	if err := repo.MarkReady(ctx, c.ID, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, c.ID)
	if err != nil || got.Status != backupcopydom.StatusReady || got.TTL != time.Hour || got.BackupCreatedAt == nil {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if due, _ := repo.ListDue(ctx, now, now); len(due) != 0 {
		t.Fatalf("not expired yet: %v", due)
	}
	if due, _ := repo.ListDue(ctx, now.Add(2*time.Hour), now); len(due) != 1 {
		t.Fatalf("expired: %v", due)
	}
	job := uuid.New()
	if err := repo.MarkDropping(ctx, c.ID, &job); err != nil {
		t.Fatal(err)
	}
	// Once dropping, a new copy may be opened.
	if err := repo.Create(ctx, mk()); err != nil {
		t.Fatalf("copy after dropping: %v", err)
	}
	if err := repo.MarkDropped(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := repo.ListByDatabase(ctx, dbID)
	if len(list) != 1 {
		t.Fatalf("listed %d copies, want the live one only", len(list))
	}
}
