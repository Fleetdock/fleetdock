// Package schemahistoryapp records the structure of watched databases over
// time, alerts when it changes and compares structures across databases.
package schemahistoryapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	schemahistorydom "github.com/Fleetdock/fleetdock/backend/internal/domain/schemahistory"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

// SchemaSource reads a database's structure.
type SchemaSource interface {
	// Schema reads it as the caller's data login sees it.
	Schema(ctx context.Context, databaseID string) (*schemadiff.Schema, error)
	// AdminSchema reads it with the instance's admin login (background).
	AdminSchema(ctx context.Context, databaseID uuid.UUID) (*schemadiff.Schema, error)
}

// Notifier emits notification events.
type Notifier interface {
	Emit(ctx context.Context, eventType, title, message, severity string, aggregateType string, aggregateID uuid.UUID)
}

// DatabaseLookup resolves databases.
type DatabaseLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*databasedom.Database, error)
}

// EventSchemaChanged is emitted when a watched database's structure changes.
const EventSchemaChanged = "database.schema_changed"

// Service records and compares schemas.
type Service struct {
	repo     schemahistorydom.Repository
	source   SchemaSource
	dbs      DatabaseLookup
	notifier Notifier
	interval time.Duration
	timeout  time.Duration
	parallel int
	batch    int
	now      func() time.Time
}

// NewService builds the service. interval is how often watched databases
// are checked (0 disables background checks; snapshots can still be taken by
// hand).
func NewService(repo schemahistorydom.Repository, source SchemaSource, dbs DatabaseLookup, interval time.Duration) *Service {
	return &Service{repo: repo, source: source, dbs: dbs, interval: interval, timeout: 60 * time.Second,
		parallel: 4, batch: 50, now: time.Now}
}

// SetNotifier enables change notifications.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// Interval is the background check interval (0 = off).
func (s *Service) Interval() time.Duration { return s.interval }

// Enabled reports whether background checks run.
func (s *Service) Enabled() bool { return s.interval > 0 }

// TakeSnapshot reads the database's structure and records it if it differs
// from the latest snapshot. It returns the latest snapshot and whether it is
// new.
func (s *Service) TakeSnapshot(ctx context.Context, databaseID uuid.UUID, trigger string, by *uuid.UUID) (*schemahistorydom.Snapshot, bool, error) {
	cur, err := s.source.AdminSchema(ctx, databaseID)
	if err != nil {
		return nil, false, err
	}
	return s.record(ctx, databaseID, cur, trigger, by)
}

func (s *Service) record(ctx context.Context, databaseID uuid.UUID, cur *schemadiff.Schema, trigger string, by *uuid.UUID) (*schemahistorydom.Snapshot, bool, error) {
	prev, err := s.repo.Latest(ctx, databaseID)
	if err != nil {
		return nil, false, err
	}
	hash := cur.Hash()
	if prev != nil && prev.Hash == hash {
		prev.SchemaGz = nil
		return prev, false, nil
	}
	gz, err := schemahistorydom.Encode(cur)
	if err != nil {
		return nil, false, apperr.Internal(err)
	}
	snap := &schemahistorydom.Snapshot{
		DatabaseID: databaseID, Hash: hash, Trigger: trigger, TakenBy: by, SchemaGz: gz,
		Summary: schemahistorydom.Summary{Counts: cur.Counts(), Truncated: cur.Truncated, Stored: gz != nil},
	}
	if prev != nil {
		snap.PreviousID = &prev.ID
		snap.Summary.Highlights = []string{"structure changed"}
		if prev.SchemaGz != nil {
			if old, err := schemahistorydom.Decode(prev.SchemaGz); err == nil {
				changes := schemadiff.Diff(old, cur)
				snap.Summary.Changes = schemadiff.Count(changes)
				snap.Summary.Highlights = highlights(changes, 8)
			}
		}
	}
	if err := s.repo.Add(ctx, snap); err != nil {
		return nil, false, err
	}
	snap.SchemaGz = nil
	if prev != nil && s.notifier != nil {
		s.notifyChanged(ctx, snap)
	}
	return snap, true, nil
}

func (s *Service) notifyChanged(ctx context.Context, snap *schemahistorydom.Snapshot) {
	name := snap.DatabaseID.String()
	if db, err := s.dbs.GetByID(ctx, snap.DatabaseID); err == nil {
		name = db.Name
	}
	msg := strings.Join(snap.Summary.Highlights, "; ")
	if t := snap.Summary.Changes; t.Added+t.Removed+t.Changed > len(snap.Summary.Highlights) {
		msg += fmt.Sprintf(" (%d changes in all)", t.Added+t.Removed+t.Changed)
	}
	s.notifier.Emit(ctx, EventSchemaChanged, "Schema changed: "+name, msg, "info", "database", snap.DatabaseID)
}

var opSymbol = map[string]string{schemadiff.Added: "+", schemadiff.Removed: "−", schemadiff.Changed: "~"}

func highlights(changes []schemadiff.Change, n int) []string {
	out := []string{}
	for i, c := range changes {
		if i == n {
			break
		}
		out = append(out, opSymbol[c.Op]+" "+c.Label())
	}
	return out
}

// RunDue checks the watched databases that are due, a few at a time.
func (s *Service) RunDue(ctx context.Context) int {
	if !s.Enabled() {
		return 0
	}
	now := s.now()
	ids, err := s.repo.ListDue(ctx, now.Add(-s.interval), s.batch)
	if err != nil {
		slog.Error("list schema watches due", "error", err.Error())
		return 0
	}
	sem := make(chan struct{}, s.parallel)
	var wg sync.WaitGroup
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(id uuid.UUID) {
			defer func() { <-sem; wg.Done() }()
			s.check(ctx, id, schemahistorydom.TriggerScheduled)
		}(id)
	}
	wg.Wait()
	return len(ids)
}

// check snapshots one database and records the outcome on its watch.
func (s *Service) check(ctx context.Context, id uuid.UUID, trigger string) {
	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	msg := ""
	if _, _, err := s.TakeSnapshot(cctx, id, trigger, nil); err != nil {
		msg = apperr.EngineMessage(err)
	}
	if err := s.repo.MarkChecked(ctx, id, s.now(), msg); err != nil {
		slog.Error("record schema check", "database_id", id, "error", err.Error())
	}
}

// SchemaChanged snapshots a watched database in the background after DDL
// made through Fleetdock.
func (s *Service) SchemaChanged(databaseID uuid.UUID) {
	go func() {
		ctx := context.Background()
		w, err := s.repo.GetWatch(ctx, databaseID)
		if err != nil || w == nil || !w.Enabled {
			return
		}
		s.check(ctx, databaseID, schemahistorydom.TriggerDDL)
	}()
}

// History is a database's watch and snapshots.
type History struct {
	Watch     *schemahistorydom.Watch     `json:"watch"`
	Interval  int                         `json:"interval_seconds"`
	Snapshots []schemahistorydom.Snapshot `json:"items"`
}

// List returns the database's watch and its newest snapshots.
func (s *Service) List(ctx context.Context, databaseID uuid.UUID) (*History, error) {
	w, err := s.repo.GetWatch(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.List(ctx, databaseID, schemahistorydom.Keep)
	if err != nil {
		return nil, err
	}
	return &History{Watch: w, Interval: int(s.interval.Seconds()), Snapshots: items}, nil
}

// SetWatch turns history on or off. Turning it on takes the first snapshot
// right away when there is none.
func (s *Service) SetWatch(ctx context.Context, databaseID uuid.UUID, enabled bool, by *uuid.UUID) (*schemahistorydom.Watch, error) {
	w, err := s.repo.SetWatch(ctx, databaseID, enabled, by)
	if err != nil {
		return nil, err
	}
	if enabled {
		if latest, err := s.repo.Latest(ctx, databaseID); err == nil && latest == nil {
			s.check(ctx, databaseID, schemahistorydom.TriggerManual)
			if fresh, err := s.repo.GetWatch(ctx, databaseID); err == nil && fresh != nil {
				w = fresh
			}
		}
	}
	return w, nil
}

// Detail is one snapshot with what changed since the one before it.
type Detail struct {
	Snapshot schemahistorydom.Snapshot  `json:"snapshot"`
	Previous *schemahistorydom.Snapshot `json:"previous,omitempty"`
	// Changes since the previous snapshot; nil when there is none or either
	// structure was too large to keep.
	Changes []schemadiff.Change `json:"changes"`
}

// Get returns a snapshot and its changes.
func (s *Service) Get(ctx context.Context, databaseID, id uuid.UUID) (*Detail, error) {
	snap, err := s.repo.Get(ctx, databaseID, id)
	if err != nil {
		return nil, err
	}
	d := &Detail{Snapshot: *snap, Changes: []schemadiff.Change{}}
	d.Snapshot.SchemaGz = nil
	if snap.PreviousID == nil {
		return d, nil
	}
	prev, err := s.repo.Get(ctx, databaseID, *snap.PreviousID)
	if apperr.KindOf(err) == apperr.KindNotFound {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	if snap.SchemaGz != nil && prev.SchemaGz != nil {
		a, err1 := schemahistorydom.Decode(prev.SchemaGz)
		b, err2 := schemahistorydom.Decode(snap.SchemaGz)
		if err1 == nil && err2 == nil {
			d.Changes = schemadiff.Diff(a, b)
		}
	}
	prev.SchemaGz = nil
	d.Previous = prev
	return d, nil
}

// CompareInput names the two sides of a comparison: the source (a database
// as it is now, or one of its snapshots) and the target database the script
// would change.
type CompareInput struct {
	SourceDatabaseID uuid.UUID
	SourceSnapshotID *uuid.UUID
	TargetDatabaseID uuid.UUID
}

// Comparison is the result of a comparison.
type Comparison struct {
	Same       bool     `json:"same"`
	SourceHash string   `json:"source_hash"`
	TargetHash string   `json:"target_hash"`
	Warnings   []string `json:"warnings"`
	schemadiff.Script
}

// Compare diffs the target against the source and writes the script that
// makes the target match. Live databases are read as the caller's data login
// sees them.
func (s *Service) Compare(ctx context.Context, in CompareInput) (*Comparison, error) {
	srcDB, err := s.dbs.GetByID(ctx, in.SourceDatabaseID)
	if err != nil {
		return nil, err
	}
	tgtDB, err := s.dbs.GetByID(ctx, in.TargetDatabaseID)
	if err != nil {
		return nil, err
	}
	srcLabel := srcDB.Name
	var src *schemadiff.Schema
	if in.SourceSnapshotID != nil {
		snap, err := s.repo.Get(ctx, in.SourceDatabaseID, *in.SourceSnapshotID)
		if err != nil {
			return nil, err
		}
		if snap.SchemaGz == nil {
			return nil, apperr.Invalid("source_snapshot_id", "this snapshot's structure was too large to keep")
		}
		if src, err = schemahistorydom.Decode(snap.SchemaGz); err != nil {
			return nil, apperr.Internal(err)
		}
		srcLabel = fmt.Sprintf("%s as of %s", srcDB.Name, snap.TakenAt.UTC().Format("2006-01-02 15:04 UTC"))
	} else if src, err = s.source.Schema(ctx, in.SourceDatabaseID.String()); err != nil {
		return nil, err
	}
	tgt, err := s.source.Schema(ctx, in.TargetDatabaseID.String())
	if err != nil {
		return nil, err
	}
	if src.Dialect != tgt.Dialect || src.Engine != tgt.Engine {
		return nil, apperr.Invalid("target_database_id",
			fmt.Sprintf("schemas compare only on the same engine (%s and %s)", src.Engine, tgt.Engine))
	}
	out := &Comparison{SourceHash: src.Hash(), TargetHash: tgt.Hash(), Warnings: []string{}}
	out.Same = out.SourceHash == out.TargetHash
	if src.Truncated || tgt.Truncated {
		out.Warnings = append(out.Warnings, "One of the databases has more objects than are compared; only the first ones are.")
	}
	out.Script = schemadiff.Migrate(tgt, src)
	if out.Changes == nil {
		out.Changes = []schemadiff.Change{}
	}
	header := fmt.Sprintf("-- Makes %s match %s.\n-- Generated by Fleetdock on %s. Review before running: steps marked REVIEW are\n-- commented out because they lose data.\n\n",
		tgtDB.Name, srcLabel, s.now().UTC().Format("2006-01-02 15:04 UTC"))
	if out.Same || len(out.Changes) == 0 {
		out.SQL = ""
	} else {
		out.SQL = header + out.SQL
	}
	return out, nil
}
