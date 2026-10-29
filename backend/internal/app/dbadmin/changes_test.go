package dbadminapp

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	rowchangedom "github.com/Fleetdock/fleetdock/backend/internal/domain/rowchange"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// editorClient is a fake engine whose data editing is scripted.
type editorClient struct {
	fakeClient
	mu sync.Mutex
	// denyEdits makes that many edits fail as a permission error (which
	// the service retries after re-applying grants).
	denyEdits int
	edits     int
	applyErr  error
	applied   []engine.ApplyRequest
}

func (e *editorClient) BrowseRows(context.Context, engine.ConnParams, string, engine.BrowseRequest) (*engine.BrowseResult, error) {
	return nil, nil
}

func (e *editorClient) ImportCSV(context.Context, engine.ConnParams, string, string, io.Reader, engine.ImportOptions) (int64, error) {
	return 0, nil
}

func (e *editorClient) change(op string, key engine.RowValues) (*engine.RowChange, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.edits++
	if e.denyEdits > 0 {
		e.denyEdits--
		return nil, &mysql.MySQLError{Number: 1142, Message: "command denied"}
	}
	v := "1"
	return &engine.RowChange{Op: op, Key: engine.RowValues{"id": &v}, Before: engine.RowValues{"id": &v}, Undoable: true}, nil
}

func (e *editorClient) InsertRow(_ context.Context, _ engine.ConnParams, _, _ string, _ engine.RowValues) (*engine.RowChange, error) {
	return e.change("insert", nil)
}

func (e *editorClient) UpdateRow(_ context.Context, _ engine.ConnParams, _, _ string, key, _ engine.RowValues) (*engine.RowChange, error) {
	return e.change("update", key)
}

func (e *editorClient) DeleteRow(_ context.Context, _ engine.ConnParams, _, _ string, key engine.RowValues) (*engine.RowChange, error) {
	return e.change("delete", key)
}

func (e *editorClient) ApplyRowImage(_ context.Context, _ engine.ConnParams, _, _ string, req engine.ApplyRequest) (*engine.RowChange, error) {
	e.applied = append(e.applied, req)
	if e.applyErr != nil {
		return nil, e.applyErr
	}
	return &engine.RowChange{Op: "update", Key: req.Key, Undoable: true}, nil
}

// memJournal is an in-memory rowchangedom.Repository.
type memJournal struct {
	mu       sync.Mutex
	entries  map[uuid.UUID]*rowchangedom.Change
	order    []uuid.UUID
	failAdd  bool
	released []uuid.UUID
}

func newJournal() *memJournal { return &memJournal{entries: map[uuid.UUID]*rowchangedom.Change{}} }

func (j *memJournal) Add(_ context.Context, c *rowchangedom.Change) error {
	if j.failAdd {
		return errors.New("metadata database down")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	c.ID, c.CreatedAt = uuid.New(), time.Now()
	cp := *c
	j.entries[c.ID] = &cp
	j.order = append(j.order, c.ID)
	return nil
}

func (j *memJournal) Get(_ context.Context, id uuid.UUID) (*rowchangedom.Change, error) {
	c, ok := j.entries[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *c
	return &cp, nil
}

func (j *memJournal) List(_ context.Context, f rowchangedom.ListFilter) ([]rowchangedom.Change, error) {
	var out []rowchangedom.Change
	for i := len(j.order) - 1; i >= 0; i-- {
		c := j.entries[j.order[i]]
		if f.BatchID != nil && (c.BatchID == nil || *c.BatchID != *f.BatchID) {
			continue
		}
		out = append(out, *c)
	}
	return out, nil
}

func (j *memJournal) ClaimUndo(_ context.Context, id, user uuid.UUID, at time.Time) (bool, error) {
	c := j.entries[id]
	if c.UndoneAt != nil {
		return false, nil
	}
	c.UndoneAt, c.UndoneBy = &at, &user
	return true, nil
}

func (j *memJournal) ReleaseUndo(_ context.Context, id uuid.UUID) error {
	j.released = append(j.released, id)
	j.entries[id].UndoneAt = nil
	return nil
}

func (j *memJournal) PruneBefore(context.Context, time.Time) (int64, error) { return 0, nil }

func editFixture(t *testing.T) (*Service, *editorClient, *memJournal, *databasedom.Database) {
	t.Helper()
	user, host, ref := "root", "db.example.com", "instance/x/root"
	inst := &instancedom.Instance{ID: uuid.New(), Engine: "fake-edit-engine", Kind: instancedom.KindExternal,
		Host: &host, Port: 3306, Username: &user, RootSecretRef: &ref, DataAccess: instancedom.DataAccessManaged}
	db := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "app", Status: databasedom.StatusActive}
	ed := &editorClient{fakeClient: fakeClient{&fakeEngine{failAt: -1}}}
	engine.Register("fake-edit-engine", ed)
	svc := NewService(oneInstance{inst: inst}, oneDatabase{db: db}, nil, staticSecrets{}, &fakeAccess{})
	j := newJournal()
	svc.SetJournal(j)
	return svc, ed, j, db
}

func TestEditJournalsOnceAcrossRetry(t *testing.T) {
	svc, ed, j, db := editFixture(t)
	ed.denyEdits = 1 // first attempt refused, retried after re-applying grants
	batch := uuid.New()
	user := uuid.New()
	ref, err := svc.UpdateRow(context.Background(), db.ID.String(), "t", nil, nil, EditMeta{UserID: user, BatchID: &batch})
	if err != nil {
		t.Fatal(err)
	}
	if ed.edits != 2 {
		t.Fatalf("edits = %d, want a retry", ed.edits)
	}
	if len(j.entries) != 1 || ref.ID == nil || !ref.Undoable {
		t.Fatalf("journal = %d entries, ref %+v", len(j.entries), ref)
	}
	c := j.entries[*ref.ID]
	if c.Source != rowchangedom.SourceEditor || *c.BatchID != batch || *c.UserID != user || c.Table != "t" {
		t.Fatalf("entry = %+v", c)
	}
}

func TestEditFailureJournalsNothing(t *testing.T) {
	svc, ed, j, db := editFixture(t)
	ed.denyEdits = 5
	if _, err := svc.DeleteRow(context.Background(), db.ID.String(), "t", nil, EditMeta{UserID: uuid.New()}); err == nil {
		t.Fatal("expected the edit to fail")
	}
	if len(j.entries) != 0 {
		t.Fatalf("journal = %d entries", len(j.entries))
	}
}

func TestJournalFailureKeepsTheEdit(t *testing.T) {
	svc, _, j, db := editFixture(t)
	j.failAdd = true
	ref, err := svc.InsertRow(context.Background(), db.ID.String(), "t", nil, EditMeta{UserID: uuid.New()})
	if err != nil {
		t.Fatalf("a journal failure must not fail the edit: %v", err)
	}
	if ref.Undoable || ref.ID != nil || ref.Note == "" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestUndo(t *testing.T) {
	svc, ed, j, db := editFixture(t)
	ctx := context.Background()
	user := EditMeta{UserID: uuid.New()}
	ref, _ := svc.DeleteRow(ctx, db.ID.String(), "t", nil, user)

	// Conflict: released, reported, not an error.
	ed.applyErr = engine.ErrRowChanged
	res, err := svc.UndoChange(ctx, db.ID.String(), *ref.ID, user, false)
	if err != nil || res.OK || !res.Conflict {
		t.Fatalf("conflict = %+v, %v", res, err)
	}
	if len(j.released) != 1 || j.entries[*ref.ID].UndoneAt != nil {
		t.Fatal("a failed undo must release its claim")
	}
	if ed.applied[0].Check == nil || ed.applied[0].Want == nil {
		t.Fatalf("undoing a delete must check absence and re-insert: %+v", ed.applied[0])
	}

	// Forced: no check, recorded as an undo of the change.
	ed.applyErr = nil
	res, err = svc.UndoChange(ctx, db.ID.String(), *ref.ID, user, true)
	if err != nil || !res.OK || res.Ref == nil || res.Ref.ID == nil {
		t.Fatalf("undo = %+v, %v", res, err)
	}
	if ed.applied[1].Check != nil {
		t.Fatal("force must skip the check")
	}
	u := j.entries[*res.Ref.ID]
	if u.Source != rowchangedom.SourceUndo || *u.UndoOf != *ref.ID {
		t.Fatalf("undo entry = %+v", u)
	}

	// Twice: refused without touching the database.
	res, _ = svc.UndoChange(ctx, db.ID.String(), *ref.ID, user, true)
	if res.OK || res.Error == "" || len(ed.applied) != 2 {
		t.Fatalf("second undo = %+v", res)
	}

	// Another database's change is not found here.
	if _, err := svc.UndoChange(ctx, uuid.NewString(), *ref.ID, user, false); err == nil {
		t.Fatal("a change of another database must not be undone")
	}
}

func TestUndoBatch(t *testing.T) {
	svc, ed, _, db := editFixture(t)
	ctx := context.Background()
	batch := uuid.New()
	user := EditMeta{UserID: uuid.New(), BatchID: &batch}
	for i := 0; i < 3; i++ {
		if _, err := svc.UpdateRow(ctx, db.ID.String(), "t", nil, nil, user); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = svc.UpdateRow(ctx, db.ID.String(), "t", nil, nil, EditMeta{UserID: user.UserID})
	res, err := svc.UndoBatch(ctx, db.ID.String(), batch, EditMeta{UserID: user.UserID}, false)
	if err != nil || len(res) != 3 {
		t.Fatalf("batch undo = %+v, %v", res, err)
	}
	for _, r := range res {
		if !r.OK {
			t.Fatalf("result %+v", r)
		}
	}
	if len(ed.applied) != 3 {
		t.Fatalf("applied %d", len(ed.applied))
	}
}
