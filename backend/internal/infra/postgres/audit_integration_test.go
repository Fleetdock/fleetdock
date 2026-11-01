//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
)

func TestIntegrationAudit(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewAuditRepository(pool)

	uid := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash) VALUES ($1, 'ann@x.dev', 'Ann', 'x')`, uid); err != nil {
		t.Fatal(err)
	}
	dbID := uuid.New()
	tok := uuid.New()
	add := func(action, outcome string, actor *uuid.UUID) auditdom.Event {
		e := auditdom.Event{
			ID: uuid.New(), ActorUserID: actor, ActorEmail: "ann@x.dev", ActorKind: "api_token", TokenID: &tok,
			IP: "2001:db8::1", UserAgent: "curl/8", Action: action, Outcome: outcome,
			ResourceType: "database", ResourceID: &dbID, ResourceName: "shop", Metadata: json.RawMessage(`{"table":"orders"}`),
		}
		if err := repo.Append(ctx, &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	add("database.create", "success", &uid)
	add("data.row_update", "success", &uid)
	add("data.row_delete", "denied", &uid)
	add("auth.login", "failure", nil)

	all, err := repo.List(ctx, auditdom.Filter{Limit: 10})
	if err != nil || len(all) != 4 {
		t.Fatalf("list = %d, %v", len(all), err)
	}
	if e := all[3]; e.Action != "database.create" || e.IP != "2001:db8::1" || e.ResourceName != "shop" ||
		*e.TokenID != tok || e.ActorKind != "api_token" || string(e.Metadata) != `{"table": "orders"}` {
		t.Errorf("oldest = %+v (%s)", e, e.Metadata)
	}

	data, _ := repo.List(ctx, auditdom.Filter{ActionPrefix: "data.", Limit: 10})
	denied, _ := repo.List(ctx, auditdom.Filter{Outcome: "denied", Limit: 10})
	mine, _ := repo.List(ctx, auditdom.Filter{ActorUserID: &uid, ResourceID: &dbID, Limit: 10})
	exact, _ := repo.List(ctx, auditdom.Filter{ActionPrefix: "data.row", Limit: 10})
	if len(data) != 2 || len(denied) != 1 || len(mine) != 3 || len(exact) != 0 {
		t.Errorf("filters: data %d, denied %d, mine %d, exact %d", len(data), len(denied), len(mine), len(exact))
	}

	// Paging by (created_at, id) never repeats or skips.
	seen := map[uuid.UUID]bool{}
	var after *auditdom.Cursor
	for i := 0; i < 4; i++ {
		page, err := repo.List(ctx, auditdom.Filter{Limit: 1, After: after})
		if err != nil || len(page) != 1 || seen[page[0].ID] {
			t.Fatalf("page %d = %+v, %v", i, page, err)
		}
		seen[page[0].ID] = true
		after = &auditdom.Cursor{CreatedAt: page[0].CreatedAt, ID: page[0].ID}
	}
	if rest, _ := repo.List(ctx, auditdom.Filter{Limit: 1, After: after}); len(rest) != 0 {
		t.Errorf("past the end = %+v", rest)
	}

	// Still append-only to everyone else.
	if _, err := pool.Exec(ctx, `UPDATE audit_events SET action = 'x'`); err == nil {
		t.Error("update allowed")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_events`); err == nil {
		t.Error("delete allowed")
	}

	// Deleting the user keeps their events, with the email.
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, uid); err != nil {
		t.Fatalf("delete user with audit events: %v", err)
	}
	after2, _ := repo.List(ctx, auditdom.Filter{Limit: 10})
	if len(after2) != 4 || after2[0].ActorUserID != nil || after2[1].ActorEmail != "ann@x.dev" {
		t.Errorf("after user delete = %+v", after2[1])
	}

	if n := repo.ResourceName(ctx, "user", uid); n != "" {
		t.Errorf("deleted user name = %q", n)
	}
	if n := repo.ResourceName(ctx, "nope", uid); n != "" {
		t.Errorf("unknown type name = %q", n)
	}

	// Retention.
	n, err := repo.PruneBefore(ctx, time.Now().Add(time.Hour))
	if err != nil || n != 4 {
		t.Fatalf("prune = %d, %v", n, err)
	}
	if left, _ := repo.List(ctx, auditdom.Filter{Limit: 10}); len(left) != 0 {
		t.Errorf("left after prune = %d", len(left))
	}
}
