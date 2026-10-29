// Package fleetapp runs one SQL script on many databases ("fleet query"):
// validation and authorization when a run is created, a background runner
// that works through its targets a few at a time, and merged results.
package fleetapp

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	dbadminapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbadmin"
	"github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	fleetdom "github.com/Fleetdock/fleetdock/backend/internal/domain/fleet"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// Ports.
type (
	// DatabaseLookup resolves databases.
	DatabaseLookup interface {
		GetByID(ctx context.Context, id uuid.UUID) (*databasedom.Database, error)
	}
	// InstanceLookup resolves instances.
	InstanceLookup interface {
		GetByID(ctx context.Context, id uuid.UUID) (*instancedom.Instance, error)
	}
	// Ancestry resolves a resource's place in the server → instance →
	// database tree, for scoped permission checks.
	Ancestry interface {
		Ancestry(ctx context.Context, resType authz.ResourceType, resID uuid.UUID) (authz.Ancestry, error)
	}
	// DataAccess reports how a database's interactive work connects.
	DataAccess interface {
		DataAccess(ctx context.Context, databaseID string) (dbadminapp.DataAccessInfo, error)
	}
	// Principals rebuilds a user's permissions for work done later.
	Principals interface {
		PrincipalForUser(ctx context.Context, userID uuid.UUID, scopes []string) (*authapp.Principal, error)
	}
	// Queries runs console scripts.
	Queries interface {
		Query(ctx context.Context, in dbadminapp.QueryInput) (*dbadminapp.QueryOutput, error)
		CancelQuery(ctx context.Context, databaseID, queryID string, userID uuid.UUID) error
	}
	// Auditor records who started a run.
	Auditor interface {
		Record(ctx context.Context, actor *uuid.UUID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any) error
	}
)

// Deps are the service's collaborators.
type Deps struct {
	Repo       fleetdom.Repository
	Databases  DatabaseLookup
	Instances  InstanceLookup
	Ancestry   Ancestry
	DataAccess DataAccess
	Principals Principals
	Queries    Queries
	Audit      Auditor
}

// Service creates and manages fleet runs.
type Service struct {
	Deps
	now func() time.Time
}

// NewService builds the service.
func NewService(d Deps) *Service { return &Service{Deps: d, now: time.Now} }

// CreateInput describes a new run.
type CreateInput struct {
	SQL            string
	DatabaseIDs    []string
	Concurrency    int
	StopOnError    *bool
	Canary         *bool
	TimeoutSeconds int
	RowLimit       int
}

func clamp(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Create validates and authorizes a run for the principal and queues it.
// A script with writes stops at the first failure and pauses after the
// first database (canary) unless told otherwise.
func (s *Service) Create(ctx context.Context, p *authapp.Principal, in CreateInput) (*fleetdom.Run, error) {
	stmts := engine.SplitStatements(in.SQL)
	if len(stmts) == 0 {
		return nil, apperr.Invalid("sql", "the script is empty")
	}
	writes := false
	for _, st := range stmts {
		if !engine.IsReadOnly(st) {
			writes = true
		}
	}
	if len(in.DatabaseIDs) == 0 {
		return nil, apperr.Invalid("database_ids", "choose at least one database")
	}
	if len(in.DatabaseIDs) > fleetdom.MaxTargets {
		return nil, apperr.Invalid("database_ids", fmt.Sprintf("at most %d databases per run", fleetdom.MaxTargets))
	}
	seen := map[uuid.UUID]bool{}
	targets := make([]fleetdom.Target, 0, len(in.DatabaseIDs))
	instances := map[uuid.UUID]*instancedom.Instance{}
	for _, raw := range in.DatabaseIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, apperr.Invalid("database_ids", "every id must be a valid UUID")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		db, err := s.Databases.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := s.authorize(ctx, p, id, writes); err != nil {
			return nil, apperr.Forbidden(fmt.Sprintf("%s: %s", db.Name, apperr.EngineMessage(err)))
		}
		inst := instances[db.InstanceID]
		if inst == nil {
			if inst, err = s.Instances.GetByID(ctx, db.InstanceID); err != nil {
				return nil, err
			}
			instances[db.InstanceID] = inst
		}
		dbID := id
		targets = append(targets, fleetdom.Target{DatabaseID: &dbID, DatabaseName: db.Name, InstanceName: inst.Name, Engine: string(inst.Engine)})
	}
	run := &fleetdom.Run{
		CreatedBy:      p.UserID,
		SQL:            in.SQL,
		Writes:         writes,
		Concurrency:    clamp(in.Concurrency, 1, fleetdom.MaxConcurrency, 4),
		StopOnError:    writes,
		Canary:         writes && len(targets) > 1,
		TimeoutSeconds: clamp(in.TimeoutSeconds, 1, fleetdom.MaxTimeoutSeconds, 60),
		RowLimit:       clamp(in.RowLimit, 1, fleetdom.MaxRowLimit, 100),
		TokenScopes:    p.TokenScopes(),
	}
	if in.StopOnError != nil {
		run.StopOnError = *in.StopOnError
	}
	if in.Canary != nil {
		run.Canary = *in.Canary && len(targets) > 1
	}
	if err := s.Repo.Create(ctx, run, targets); err != nil {
		return nil, err
	}
	if s.Audit != nil {
		_ = s.Audit.Record(ctx, &p.UserID, "fleet_run.create", "fleet_run", &run.ID, map[string]any{
			"databases": len(targets), "writes": writes, "statements": len(stmts),
		})
	}
	return run, nil
}

// authorize checks that the principal may run the script on the database:
// database:read (and database:write for writes), plus instance:write where
// interactive work reaches the whole instance — the same rules as the SQL
// console.
func (s *Service) authorize(ctx context.Context, p *authapp.Principal, dbID uuid.UUID, writes bool) error {
	anc, err := s.Ancestry.Ancestry(ctx, authz.ResourceDatabase, dbID)
	if err != nil {
		return err
	}
	if !p.CanOn("database:read", anc) {
		return apperr.Forbidden("no access to this database")
	}
	if writes && !p.CanOn("database:write", anc) {
		return apperr.Forbidden("the script writes, which needs database:write")
	}
	info, err := s.DataAccess.DataAccess(ctx, dbID.String())
	if err != nil {
		return err
	}
	if need, why := info.NeedsInstanceWrite(); need {
		ianc, err := s.Ancestry.Ancestry(ctx, authz.ResourceInstance, info.InstanceID)
		if err != nil {
			return err
		}
		if !p.CanOn("instance:write", ianc) {
			return apperr.Forbidden(why)
		}
	}
	return nil
}

// owned returns the run if it belongs to the user (others' runs are not
// found).
func (s *Service) owned(ctx context.Context, userID, runID uuid.UUID) (*fleetdom.Run, error) {
	r, err := s.Repo.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	if r.CreatedBy != userID {
		return nil, apperr.NotFound("fleet run not found")
	}
	return r, nil
}

// List returns the user's recent runs.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]fleetdom.Run, error) {
	return s.Repo.ListByUser(ctx, userID, 50)
}

// Get returns one of the user's runs.
func (s *Service) Get(ctx context.Context, userID, runID uuid.UUID) (*fleetdom.Run, error) {
	return s.owned(ctx, userID, runID)
}

// Targets returns a run's targets with their results.
func (s *Service) Targets(ctx context.Context, userID, runID uuid.UUID) ([]fleetdom.Target, error) {
	if _, err := s.owned(ctx, userID, runID); err != nil {
		return nil, err
	}
	return s.Repo.Targets(ctx, runID, true)
}

// Resume continues a run paused after its canary.
func (s *Service) Resume(ctx context.Context, userID, runID uuid.UUID) (*fleetdom.Run, error) {
	if _, err := s.owned(ctx, userID, runID); err != nil {
		return nil, err
	}
	if err := s.Repo.Resume(ctx, runID); err != nil {
		return nil, err
	}
	return s.Repo.Get(ctx, runID)
}

// Cancel stops a run.
func (s *Service) Cancel(ctx context.Context, userID, runID uuid.UUID) (*fleetdom.Run, error) {
	if _, err := s.owned(ctx, userID, runID); err != nil {
		return nil, err
	}
	if err := s.Repo.Cancel(ctx, runID); err != nil {
		return nil, err
	}
	return s.Repo.Get(ctx, runID)
}

// Retry re-runs the targets of a finished run that did not succeed.
func (s *Service) Retry(ctx context.Context, userID, runID uuid.UUID) (*fleetdom.Run, error) {
	if _, err := s.owned(ctx, userID, runID); err != nil {
		return nil, err
	}
	if err := s.Repo.Retry(ctx, runID); err != nil {
		return nil, err
	}
	return s.Repo.Get(ctx, runID)
}

// TargetResults is what a target keeps of its run.
type TargetResults struct {
	Results []engine.QueryResult     `json:"results"`
	Error   *dbadminapp.BatchFailure `json:"error,omitempty"`
	// Trimmed: rows were dropped to keep the results within the size limit.
	Trimmed bool `json:"trimmed,omitempty"`
}

// WriteCSV writes the merged rows of a run: the first result set of every
// target that returned rows, in one table whose columns are the union of
// theirs, led by the database and its server.
func (s *Service) WriteCSV(ctx context.Context, userID, runID uuid.UUID, w io.Writer) error {
	targets, err := s.Targets(ctx, userID, runID)
	if err != nil {
		return err
	}
	type part struct {
		t   fleetdom.Target
		res engine.QueryResult
	}
	var parts []part
	var cols []string
	index := map[string]int{}
	for _, t := range targets {
		if t.Results == nil {
			continue
		}
		var tr TargetResults
		if json.Unmarshal(t.Results, &tr) != nil {
			continue
		}
		for _, r := range tr.Results {
			if len(r.Columns) == 0 {
				continue
			}
			for _, c := range r.Columns {
				if _, ok := index[c]; !ok {
					index[c] = len(cols)
					cols = append(cols, c)
				}
			}
			parts = append(parts, part{t: t, res: r})
			break
		}
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(append([]string{"database", "server"}, cols...)); err != nil {
		return err
	}
	for _, p := range parts {
		for _, row := range p.res.Rows {
			rec := make([]string, 2+len(cols))
			rec[0], rec[1] = p.t.DatabaseName, p.t.InstanceName
			for i, c := range p.res.Columns {
				if i < len(row) && row[i] != nil {
					rec[2+index[c]] = csvSafe(*row[i])
				}
			}
			if err := cw.Write(rec); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

// csvSafe defuses values a spreadsheet would run as a formula.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return "'" + v
		}
	}
	return v
}
