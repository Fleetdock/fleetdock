package dbadminapp

import (
	"context"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

func insights(admin engine.Admin) (engine.Insights, error) {
	in, ok := admin.(engine.Insights)
	if !ok {
		return nil, apperr.Invalid("engine", "this engine does not support performance insights")
	}
	return in, nil
}

// IndexInsights reports a database's unused and redundant indexes. Usage
// counters live in server-wide statistics the data login usually cannot
// read, so this connects as the instance's admin login (the route requires
// the same rights as browsing the database).
func (s *Service) IndexInsights(ctx context.Context, databaseID string) (*engine.IndexInsights, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	in, err := insights(admin)
	if err != nil {
		return nil, err
	}
	inst, err := s.instances.GetByID(ctx, db.InstanceID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	conn.Database = db.Name
	rep, err := in.IndexReport(cctx, conn, db.Name)
	if err != nil {
		return nil, apperr.FromEngine(err, "database")
	}
	dialect := "mysql"
	if inst.Engine == instancedom.EnginePostgres {
		dialect = "postgres"
	}
	return engine.AnalyzeIndexes(rep, dialect), nil
}

// TopQueries lists the instance's most expensive statements.
func (s *Service) TopQueries(ctx context.Context, instanceID string, req engine.TopQueriesRequest) (*engine.TopQueries, error) {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	in, err := insights(admin)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	out, err := in.TopQueries(cctx, conn, req)
	return out, apperr.FromEngine(err, "instance")
}

// ExplainInput is one EXPLAIN request.
type ExplainInput struct {
	DatabaseID string
	SQL        string
	// Analyze executes the statement for real timings; reads only.
	Analyze bool
	// AllowWrite: the caller may write to the database. Explaining a write
	// never runs it, but its plan needs the write privileges, so the
	// read-write login (and the caller's right to it) is used.
	AllowWrite bool
}

// Explain returns a statement's plan, connecting as the data login.
func (s *Service) Explain(ctx context.Context, in ExplainInput) (*engine.ExplainResult, error) {
	stmts := engine.SplitStatements(in.SQL)
	if len(stmts) != 1 {
		return nil, apperr.Invalid("sql", "explain takes exactly one statement")
	}
	read := engine.IsReadOnly(stmts[0])
	if in.Analyze && !read {
		return nil, apperr.Invalid("analyze", "only reads can be analyzed: analyzing runs the statement")
	}
	mode := dbaccessdom.ModeRead
	if !read {
		if !in.AllowWrite {
			return nil, apperr.Forbidden("explaining a write needs write access to the database")
		}
		mode = dbaccessdom.ModeWrite
	}
	var out *engine.ExplainResult
	err := s.scoped(ctx, in.DatabaseID, mode, queryTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			ins, err := insights(admin)
			if err != nil {
				return err
			}
			out, err = ins.Explain(ctx, conn, db.Name, stmts[0], in.Analyze)
			return err
		})
	return out, err
}
