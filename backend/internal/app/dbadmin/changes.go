package dbadminapp

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	rowchangedom "github.com/Fleetdock/fleetdock/backend/internal/domain/rowchange"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// SetJournal enables the row change journal behind data-browser undo.
func (s *Service) SetJournal(j rowchangedom.Repository) { s.journal = j }

// EditMeta is who made a data-browser edit and which save it belongs to.
type EditMeta struct {
	UserID uuid.UUID
	// BatchID groups the edits of one Save, so they can be undone together.
	BatchID *uuid.UUID
}

// ChangeRef is what an edit reports back: its journal entry, if any.
type ChangeRef struct {
	ID       *uuid.UUID `json:"change_id,omitempty"`
	Undoable bool       `json:"undoable"`
	Note     string     `json:"note,omitempty"`
}

// InsertRow inserts one row.
func (s *Service) InsertRow(ctx context.Context, databaseID, table string, values engine.RowValues, meta EditMeta) (ChangeRef, error) {
	return s.editRow(ctx, databaseID, table, meta, func(ctx context.Context, ed engine.DataEditor, conn engine.ConnParams, db string) (*engine.RowChange, error) {
		return ed.InsertRow(ctx, conn, db, table, values)
	})
}

// UpdateRow updates the row identified by key.
func (s *Service) UpdateRow(ctx context.Context, databaseID, table string, key, values engine.RowValues, meta EditMeta) (ChangeRef, error) {
	return s.editRow(ctx, databaseID, table, meta, func(ctx context.Context, ed engine.DataEditor, conn engine.ConnParams, db string) (*engine.RowChange, error) {
		return ed.UpdateRow(ctx, conn, db, table, key, values)
	})
}

// DeleteRow deletes the row identified by key.
func (s *Service) DeleteRow(ctx context.Context, databaseID, table string, key engine.RowValues, meta EditMeta) (ChangeRef, error) {
	return s.editRow(ctx, databaseID, table, meta, func(ctx context.Context, ed engine.DataEditor, conn engine.ConnParams, db string) (*engine.RowChange, error) {
		return ed.DeleteRow(ctx, conn, db, table, key)
	})
}

// editRow runs one row edit as the read-write login and journals it. The
// journal is written after the edit committed and after scoped's retries, so
// a retried attempt is never recorded twice; a journal failure never fails
// the (already committed) edit, it only makes it not undoable.
func (s *Service) editRow(ctx context.Context, databaseID, table string, meta EditMeta,
	fn func(ctx context.Context, ed engine.DataEditor, conn engine.ConnParams, db string) (*engine.RowChange, error)) (ChangeRef, error) {
	var ch *engine.RowChange
	var dbID uuid.UUID
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeWrite, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			ed, err := dataEditor(admin)
			if err != nil {
				return err
			}
			dbID = db.ID
			ch, err = fn(ctx, ed, conn, db.Name)
			return err
		})
	if err != nil {
		return ChangeRef{}, err
	}
	return s.record(dbID, table, rowchangedom.SourceEditor, ch, meta, nil), nil
}

// record journals a change made by user; undoOf links an undo to its entry.
func (s *Service) record(databaseID uuid.UUID, table, source string, ch *engine.RowChange, meta EditMeta, undoOf *uuid.UUID) ChangeRef {
	if s.journal == nil || ch == nil || ch.Op == "" {
		return ChangeRef{}
	}
	c := rowchangedom.FromEngine(databaseID, table, source, ch)
	c.BatchID, c.UndoOf = meta.BatchID, undoOf
	if meta.UserID != uuid.Nil {
		c.UserID = &meta.UserID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.journal.Add(ctx, c); err != nil {
		slog.Error("journal row change", "database_id", databaseID, "table", table, "error", err.Error())
		return ChangeRef{Note: "the change was saved but could not be recorded, so it cannot be undone"}
	}
	return ChangeRef{ID: &c.ID, Undoable: c.Undoable, Note: c.Note}
}

// ListChanges lists a database's journal, newest first.
func (s *Service) ListChanges(ctx context.Context, f rowchangedom.ListFilter) ([]rowchangedom.Change, error) {
	if s.journal == nil {
		return []rowchangedom.Change{}, nil
	}
	return s.journal.List(ctx, f)
}

// UndoResult is the outcome of undoing one change.
type UndoResult struct {
	ChangeID uuid.UUID `json:"change_id"`
	OK       bool      `json:"ok"`
	// Conflict: the row changed since; Force would overwrite that change.
	Conflict bool       `json:"conflict,omitempty"`
	Error    string     `json:"error,omitempty"`
	Ref      *ChangeRef `json:"undo,omitempty"`
}

// UndoChange reverts one journaled change of a database. Unless force, it
// first checks the row still looks as the change left it.
func (s *Service) UndoChange(ctx context.Context, databaseID string, changeID uuid.UUID, meta EditMeta, force bool) (*UndoResult, error) {
	if s.journal == nil {
		return nil, apperr.Invalid("change_id", "the change journal is disabled")
	}
	c, err := s.journal.Get(ctx, changeID)
	if err != nil {
		return nil, err
	}
	if c.DatabaseID.String() != databaseID {
		return nil, apperr.NotFound("change not found")
	}
	return s.undo(ctx, c, meta, force)
}

// UndoBatch reverts every not yet undone change of one save, newest first,
// and reports each.
func (s *Service) UndoBatch(ctx context.Context, databaseID string, batchID uuid.UUID, meta EditMeta, force bool) ([]UndoResult, error) {
	if s.journal == nil {
		return nil, apperr.Invalid("batch_id", "the change journal is disabled")
	}
	did, err := uuid.Parse(databaseID)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	changes, err := s.journal.List(ctx, rowchangedom.ListFilter{DatabaseID: did, BatchID: &batchID, Limit: rowchangedom.MaxBatchUndo})
	if err != nil {
		return nil, err
	}
	out := []UndoResult{}
	for i := range changes {
		c := &changes[i]
		if c.UndoneAt != nil || !c.Undoable {
			continue
		}
		res, err := s.undo(ctx, c, meta, force)
		if err != nil {
			return out, err
		}
		out = append(out, *res)
	}
	return out, nil
}

// undo applies the inverse of c. Expected failures (the row changed, the
// table changed) come back as a result, not an error.
func (s *Service) undo(ctx context.Context, c *rowchangedom.Change, meta EditMeta, force bool) (*UndoResult, error) {
	res := &UndoResult{ChangeID: c.ID}
	if !c.Undoable {
		res.Error = "this change cannot be undone"
		if c.Note != "" {
			res.Error = c.Note
		}
		return res, nil
	}
	req := engine.ApplyRequest{Key: c.Key.Values(), Session: c.Session}
	switch c.Op {
	case "insert":
		req.Check = engine.ExpectImage(c.After.Values())
	case "update":
		req.Check, req.Want = engine.ExpectImage(c.After.Values()), c.Before.Values()
	case "delete":
		req.Check, req.Want = engine.ExpectImage(nil), c.Before.Values()
	}
	if force {
		req.Check = nil
	}
	if meta.UserID == uuid.Nil {
		return nil, apperr.Unauthorized("authentication required")
	}
	ok, err := s.journal.ClaimUndo(ctx, c.ID, meta.UserID, time.Now())
	if err != nil {
		return nil, err
	}
	if !ok {
		res.Error = "this change was already undone"
		return res, nil
	}
	var ch *engine.RowChange
	err = s.scoped(ctx, c.DatabaseID.String(), dbaccessdom.ModeWrite, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			ed, err := dataEditor(admin)
			if err != nil {
				return err
			}
			ch, err = ed.ApplyRowImage(ctx, conn, db.Name, c.Table, req)
			// Map before scoped sees them: they are verdicts, not failures
			// to retry or report as engine errors.
			if errors.Is(err, engine.ErrRowChanged) || errors.Is(err, engine.ErrStructureChanged) {
				return apperr.Conflict(err.Error())
			}
			return err
		})
	if err != nil {
		if rerr := s.journal.ReleaseUndo(context.Background(), c.ID); rerr != nil {
			slog.Error("release undo claim", "change_id", c.ID, "error", rerr.Error())
		}
		if apperr.KindOf(err) == apperr.KindConflict {
			res.Conflict, res.Error = true, err.Error()
			return res, nil
		}
		return nil, err
	}
	res.OK = true
	ref := s.record(c.DatabaseID, c.Table, rowchangedom.SourceUndo, ch, meta, &c.ID)
	res.Ref = &ref
	return res, nil
}
