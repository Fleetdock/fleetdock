package dbadminapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	backupcopydom "github.com/Fleetdock/fleetdock/backend/internal/domain/backupcopy"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	rowchangedom "github.com/Fleetdock/fleetdock/backend/internal/domain/rowchange"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// Comparing a table with a backup copy and putting rows back.

// CopyLookup finds backup copies (backupcopyapp.Service).
type CopyLookup interface {
	Get(ctx context.Context, databaseID, copyID uuid.UUID) (*backupcopydom.Copy, error)
}

// SetCopies enables backup-copy comparisons.
func (s *Service) SetCopies(c CopyLookup) { s.copies = c }

// Diff limits.
const (
	diffTimeout    = 45 * time.Second
	diffPageRows   = 1000
	maxRestoreRows = 200
	diffCellBytes  = 1024
)

// Diff phases: first the live table is scanned and looked up in the copy
// (rows added or changed since the backup), then the copy is scanned and
// looked up in the live table (rows removed since).
const (
	PhaseLive   = "live"
	PhaseBackup = "backup"
)

// DiffCursor is where a comparison continues.
type DiffCursor struct {
	Phase string             `json:"phase"`
	After rowchangedom.Image `json:"after,omitempty"`
}

// DiffItem is one row that differs.
type DiffItem struct {
	// Kind: "added" (exists now, not in the backup), "removed" (in the
	// backup, gone now) or "changed".
	Kind string             `json:"kind"`
	Key  rowchangedom.Image `json:"key"`
	// Live and Backup are the row's values in Columns order (cut for
	// display); absent for the side the row is missing from.
	Live    []*string `json:"live,omitempty"`
	Backup  []*string `json:"backup,omitempty"`
	Changed []string  `json:"changed,omitempty"`
	// LiveHash fingerprints the row as compared, so putting it back can
	// check nobody changed it meanwhile.
	LiveHash string `json:"live_hash,omitempty"`
}

// CopyDiff is one step of a comparison.
type CopyDiff struct {
	Columns  []string    `json:"columns"`
	Key      []string    `json:"key"`
	Items    []DiffItem  `json:"items"`
	Scanned  int         `json:"scanned"`
	Next     *DiffCursor `json:"next,omitempty"`
	Warnings []string    `json:"warnings,omitempty"`
}

// readyCopy returns a database's copy if it can be compared.
func (s *Service) readyCopy(ctx context.Context, databaseID string, copyID uuid.UUID) (*backupcopydom.Copy, error) {
	if s.copies == nil {
		return nil, apperr.Invalid("copy_id", "backup copies are not available")
	}
	did, err := uuid.Parse(databaseID)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	c, err := s.copies.Get(ctx, did, copyID)
	if err != nil {
		return nil, err
	}
	if c.Status != backupcopydom.StatusReady {
		return nil, apperr.Conflict(fmt.Sprintf("the backup copy is %s, not ready", c.Status))
	}
	return c, nil
}

// copySide connects to a copy's temporary database with the instance's
// admin login: the copy is a scratch database no data login knows.
func (s *Service) copySide(ctx context.Context, c *backupcopydom.Copy) (engine.RowReader, engine.ConnParams, error) {
	_, admin, conn, err := s.target(ctx, c.InstanceID.String())
	if err != nil {
		return nil, conn, err
	}
	rr, ok := admin.(engine.RowReader)
	if !ok {
		return nil, conn, apperr.Invalid("engine", "this engine cannot compare tables")
	}
	conn.Database = c.ScratchName
	return rr, conn, nil
}

// commonColumns are the columns both sides have, in live order, plus
// warnings about the others.
func commonColumns(live, backup []string) ([]string, []string) {
	inBackup := map[string]bool{}
	for _, c := range backup {
		inBackup[c] = true
	}
	var common, onlyLive, warnings []string
	inLive := map[string]bool{}
	for _, c := range live {
		inLive[c] = true
		if inBackup[c] {
			common = append(common, c)
		} else {
			onlyLive = append(onlyLive, c)
		}
	}
	var onlyBackup []string
	for _, c := range backup {
		if !inLive[c] {
			onlyBackup = append(onlyBackup, c)
		}
	}
	if len(onlyLive) > 0 {
		warnings = append(warnings, "Columns added since the backup are not compared: "+strings.Join(onlyLive, ", "))
	}
	if len(onlyBackup) > 0 {
		warnings = append(warnings, "Columns dropped since the backup are not compared: "+strings.Join(onlyBackup, ", "))
	}
	return common, warnings
}

func cut(row engine.RowValues, cols []string) []*string {
	out := make([]*string, len(cols))
	for i, c := range cols {
		if v := row[c]; v != nil {
			s := *v
			if len(s) > diffCellBytes {
				s = s[:diffCellBytes] + "…"
			}
			out[i] = &s
		}
	}
	return out
}

func sameKey(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// diffPages compares a scanned page of one side with the matching rows of
// the other. scannedLive tells which side was scanned.
func diffPages(scanned, other *engine.ScanPage, common []string, scannedLive bool) []DiffItem {
	byKey := make(map[string]engine.RowValues, len(other.Rows))
	for _, r := range other.Rows {
		byKey[engine.KeyString(scanned.Key, r)] = r
	}
	items := []DiffItem{}
	for _, r := range scanned.Rows {
		o, found := byKey[engine.KeyString(scanned.Key, r)]
		key := rowchangedom.Image{}
		for _, k := range scanned.Key {
			key[k] = r[k]
		}
		switch {
		case scannedLive && !found:
			items = append(items, DiffItem{Kind: "added", Key: key, Live: cut(r, common), LiveHash: engine.RowHash(r, common)})
		case scannedLive:
			var changed []string
			for _, c := range common {
				a, b := r[c], o[c]
				if (a == nil) != (b == nil) || (a != nil && *a != *b) {
					changed = append(changed, c)
				}
			}
			if len(changed) > 0 {
				items = append(items, DiffItem{Kind: "changed", Key: key, Live: cut(r, common), Backup: cut(o, common),
					Changed: changed, LiveHash: engine.RowHash(r, common)})
			}
		case !found:
			items = append(items, DiffItem{Kind: "removed", Key: key, Backup: cut(r, common)})
		}
	}
	return items
}

func keysOf(p *engine.ScanPage) []engine.RowValues {
	out := make([]engine.RowValues, 0, len(p.Rows))
	for _, r := range p.Rows {
		k := engine.RowValues{}
		for _, c := range p.Key {
			k[c] = r[c]
		}
		out = append(out, k)
	}
	return out
}

// DiffCopy compares one page of a table with its backup copy. Call it again
// with Next until Next is nil.
func (s *Service) DiffCopy(ctx context.Context, databaseID string, copyID uuid.UUID, table string, cur *DiffCursor) (*CopyDiff, error) {
	c, err := s.readyCopy(ctx, databaseID, copyID)
	if err != nil {
		return nil, err
	}
	copyReader, copyConn, err := s.copySide(ctx, c)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		cur = &DiffCursor{Phase: PhaseLive}
	}
	if cur.Phase != PhaseLive && cur.Phase != PhaseBackup {
		return nil, apperr.Invalid("cursor", "unknown phase")
	}
	var after engine.RowValues
	if cur.After != nil {
		after = cur.After.Values()
	}
	out := &CopyDiff{Items: []DiffItem{}}
	err = s.scoped(ctx, databaseID, dbaccessdom.ModeRead, diffTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			live, ok := admin.(engine.RowReader)
			if !ok {
				return apperr.Invalid("engine", "this engine cannot compare tables")
			}
			var scanned, other *engine.ScanPage
			var err error
			if cur.Phase == PhaseLive {
				if scanned, err = live.ScanRows(ctx, conn, db.Name, table, after, diffPageRows); err != nil {
					return err
				}
				if other, err = getInBatches(ctx, copyReader, copyConn, c.ScratchName, table, keysOf(scanned)); err != nil {
					return copyTableErr(err, table)
				}
			} else {
				if scanned, err = copyReader.ScanRows(ctx, copyConn, c.ScratchName, table, after, diffPageRows); err != nil {
					return copyTableErr(err, table)
				}
				if other, err = getInBatches(ctx, live, conn, db.Name, table, keysOf(scanned)); err != nil {
					return err
				}
			}
			if !sameKey(scanned.Key, other.Key) {
				return apperr.Conflict("the table's row key changed since the backup; compare it in the SQL console instead")
			}
			liveCols, backupCols := scanned.Columns, other.Columns
			if cur.Phase == PhaseBackup {
				liveCols, backupCols = other.Columns, scanned.Columns
			}
			out.Columns, out.Warnings = commonColumns(liveCols, backupCols)
			out.Key = scanned.Key
			out.Items = diffPages(scanned, other, out.Columns, cur.Phase == PhaseLive)
			out.Scanned = len(scanned.Rows)
			switch {
			case scanned.Next != nil:
				out.Next = &DiffCursor{Phase: cur.Phase, After: rowchangedom.Image(scanned.Next)}
			case cur.Phase == PhaseLive:
				out.Next = &DiffCursor{Phase: PhaseBackup}
			}
			return nil
		})
	if errors.Is(err, engine.ErrNoRowKey) {
		return nil, apperr.Invalid("table", "this table has no primary key or NOT NULL unique key, so its rows cannot be matched with the backup")
	}
	return out, err
}

func getInBatches(ctx context.Context, r engine.RowReader, p engine.ConnParams, db, table string, keys []engine.RowValues) (*engine.ScanPage, error) {
	var out *engine.ScanPage
	for start := 0; start < len(keys) || out == nil; start += engine.MaxGetRows {
		end := min(start+engine.MaxGetRows, len(keys))
		page, err := r.GetRows(ctx, p, db, table, keys[start:end])
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = page
		} else {
			out.Rows = append(out.Rows, page.Rows...)
		}
		if end == len(keys) {
			break
		}
	}
	return out, nil
}

func copyTableErr(err error, table string) error {
	if strings.Contains(err.Error(), "not found") {
		return apperr.Invalid("table", fmt.Sprintf("the backup has no table %s", table))
	}
	return err
}

// RestoreRowInput is one row to put back as it was in the backup.
type RestoreRowInput struct {
	Key  rowchangedom.Image `json:"key"`
	Kind string             `json:"kind"`
	// LiveHash is DiffItem.LiveHash: the row must still be as compared.
	LiveHash string `json:"live_hash"`
}

// RestoreRowResult is the outcome of one row.
type RestoreRowResult struct {
	Key      rowchangedom.Image `json:"key"`
	OK       bool               `json:"ok"`
	Conflict bool               `json:"conflict,omitempty"`
	Error    string             `json:"error,omitempty"`
	ChangeID *uuid.UUID         `json:"change_id,omitempty"`
}

// RestoreRows makes rows of a table match the backup copy: changed rows get
// the backup's values, removed rows come back, added rows are deleted. Each
// row is checked against the comparison first and journaled (source
// "restore", one batch), so the whole restore can be undone.
func (s *Service) RestoreRows(ctx context.Context, databaseID string, copyID uuid.UUID, table string, rows []RestoreRowInput, meta EditMeta) ([]RestoreRowResult, error) {
	if len(rows) == 0 {
		return nil, apperr.Invalid("rows", "choose rows to restore")
	}
	if len(rows) > maxRestoreRows {
		return nil, apperr.Invalid("rows", fmt.Sprintf("restore at most %d rows at a time", maxRestoreRows))
	}
	c, err := s.readyCopy(ctx, databaseID, copyID)
	if err != nil {
		return nil, err
	}
	copyReader, copyConn, err := s.copySide(ctx, c)
	if err != nil {
		return nil, err
	}
	keys := make([]engine.RowValues, len(rows))
	for i, r := range rows {
		if r.Kind != "added" && r.Kind != "removed" && r.Kind != "changed" {
			return nil, apperr.Invalid("kind", "kind must be added, removed or changed")
		}
		keys[i] = r.Key.Values()
	}
	backupPage, err := copyReader.GetRows(ctx, copyConn, c.ScratchName, table, keys)
	if err != nil {
		return nil, copyTableErr(err, table)
	}
	inst, err := s.instances.GetByID(ctx, c.InstanceID)
	if err != nil {
		return nil, err
	}
	session := engine.CanonicalSession(string(inst.Engine))
	if meta.BatchID == nil {
		b := uuid.New()
		meta.BatchID = &b
	}
	backupByKey := map[string]engine.RowValues{}
	for _, r := range backupPage.Rows {
		backupByKey[engine.KeyString(backupPage.Key, r)] = r
	}

	// The live columns decide what is compared and written.
	var liveCols []string
	if err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			rr, ok := admin.(engine.RowReader)
			if !ok {
				return apperr.Invalid("engine", "this engine cannot compare tables")
			}
			p, err := rr.GetRows(ctx, conn, db.Name, table, nil)
			if err == nil {
				liveCols = p.Columns
			}
			return err
		}); err != nil {
		return nil, err
	}
	common, _ := commonColumns(liveCols, backupPage.Columns)

	out := make([]RestoreRowResult, 0, len(rows))
	for _, r := range rows {
		res := RestoreRowResult{Key: r.Key}
		backup := backupByKey[engine.KeyString(backupPage.Key, r.Key.Values())]
		req := engine.ApplyRequest{Key: r.Key.Values(), Session: session}
		switch r.Kind {
		case "removed":
			req.Check = engine.ExpectImage(nil)
		default:
			want := r.LiveHash
			req.Check = func(cur engine.RowValues) error {
				if cur == nil || engine.RowHash(cur, common) != want {
					return engine.ErrRowChanged
				}
				return nil
			}
		}
		if r.Kind != "added" {
			if backup == nil {
				res.Error = "the backup has no such row"
				out = append(out, res)
				continue
			}
			req.Want = engine.RowValues{}
			for _, col := range common {
				req.Want[col] = backup[col]
			}
		}
		var ch *engine.RowChange
		err := s.scoped(ctx, databaseID, dbaccessdom.ModeWrite, opTimeout,
			func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
				ed, err := dataEditor(admin)
				if err != nil {
					return err
				}
				ch, err = ed.ApplyRowImage(ctx, conn, db.Name, table, req)
				if errors.Is(err, engine.ErrRowChanged) || errors.Is(err, engine.ErrStructureChanged) {
					return apperr.Conflict(err.Error())
				}
				return err
			})
		switch {
		case apperr.KindOf(err) == apperr.KindConflict:
			res.Conflict, res.Error = true, "the row changed since it was compared; compare again"
		case err != nil:
			res.Error = apperr.EngineMessage(err)
		default:
			res.OK = true
			ref := s.record(c.DatabaseID, table, rowchangedom.SourceRestore, ch, meta, nil)
			res.ChangeID = ref.ID
		}
		out = append(out, res)
	}
	return out, nil
}

// PrepareCopy readies a freshly restored copy: on MySQL/MariaDB its
// scheduled events are disabled, since they would run inside the copy.
func (s *Service) PrepareCopy(ctx context.Context, instanceID uuid.UUID, scratch string) error {
	_, admin, conn, err := s.target(ctx, instanceID.String())
	if err != nil {
		return err
	}
	ev, ok := admin.(interface {
		DisableEvents(ctx context.Context, p engine.ConnParams, database string) (int, error)
	})
	if !ok {
		return nil
	}
	_, err = ev.DisableEvents(ctx, conn, scratch)
	return err
}
