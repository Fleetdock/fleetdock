package dbadminapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// PreviewBatch makes fakeEngine a Previewer; previews are recorded as
// batches with a "preview:" prefix on the first statement.
func (e *fakeEngine) PreviewBatch(_ context.Context, p engine.ConnParams, _ string, stmts []string, _ int, _ func(int64)) ([]engine.QueryResult, error) {
	e.users = append(e.users, p.User)
	e.batches = append(e.batches, append([]string{"preview:"}, stmts...))
	out := make([]engine.QueryResult, len(stmts))
	for i := range out {
		out[i] = engine.QueryResult{RowsAffected: 2, Notes: []string{"Rolled back: nothing was saved."}}
	}
	return out, nil
}

func TestPreview_RunsAsWriteRoleAndIsNotRecorded(t *testing.T) {
	svc, _, eng := fixture(t, databasedom.StatusActive)
	h := &memHistory{}
	svc.SetHistory(h)
	out, err := svc.Query(context.Background(), QueryInput{
		DatabaseID: uuid.NewString(), SQL: "UPDATE t SET a = 1; SELECT * FROM t", AllowWrite: true, Preview: true, UserID: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.RolledBack || len(out.Results) != 2 || eng.batches[0][0] != "preview:" {
		t.Fatalf("out = %+v, batches %v", out, eng.batches)
	}
	if eng.users[0] != "fleetdock_rw" {
		t.Fatalf("preview ran as %q, want the read-write role", eng.users[0])
	}
	if len(h.entries) != 0 {
		t.Fatalf("a preview must not be recorded: %+v", h.entries)
	}
}

func TestPreview_Refusals(t *testing.T) {
	svc, _, eng := fixture(t, databasedom.StatusActive)
	ctx := context.Background()
	cases := []struct {
		in   QueryInput
		kind apperr.Kind
	}{
		{QueryInput{SQL: "UPDATE t SET a = 1", Preview: true}, apperr.KindForbidden},       // no write access
		{QueryInput{SQL: "SELECT 1", Preview: true, AllowWrite: true}, apperr.KindInvalid}, // nothing to preview
		{QueryInput{SQL: "DELETE FROM t; DROP TABLE t", Preview: true, AllowWrite: true}, apperr.KindInvalid},
		{QueryInput{SQL: "UPDATE t SET a = 1; COMMIT", Preview: true, AllowWrite: true}, apperr.KindInvalid},
	}
	for _, c := range cases {
		c.in.DatabaseID = uuid.NewString()
		_, err := svc.Query(ctx, c.in)
		if apperr.KindOf(err) != c.kind {
			t.Errorf("%q: err = %v, want kind %v", c.in.SQL, err, c.kind)
		}
	}
	if len(eng.batches) != 0 {
		t.Fatalf("refused previews must not reach the engine: %v", eng.batches)
	}
}
