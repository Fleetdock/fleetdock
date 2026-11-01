package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Insights is the performance surface of an engine: index usage, the
// server's busiest statements and query plans. Statistics the server does not
// collect (performance_schema off, pg_stat_statements not installed) are
// reported as unavailable with a note on how to turn them on, not as errors.
type Insights interface {
	// IndexReport lists the database's indexes with how often each was used
	// since statistics were last reset.
	IndexReport(ctx context.Context, p ConnParams, database string) (*IndexReport, error)
	// TopQueries lists the statements that took the most time server-wide,
	// optionally only those run in one database.
	TopQueries(ctx context.Context, p ConnParams, req TopQueriesRequest) (*TopQueries, error)
	// Explain returns the plan of one statement. With analyze the statement
	// is executed (read-only transaction, then rolled back) and the plan
	// carries actual row counts and timings; callers allow it only for reads.
	Explain(ctx context.Context, p ConnParams, database, stmt string, analyze bool) (*ExplainResult, error)
}

// IndexDef is one index with its usage.
type IndexDef struct {
	Schema  string   `json:"schema,omitempty"`
	Table   string   `json:"table"`
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Primary bool     `json:"primary"`
	// Partial (PostgreSQL WHERE clause) and Expression (an indexed
	// expression instead of a column) indexes are never called redundant:
	// their columns do not describe what they cover.
	Partial    bool   `json:"partial"`
	Expression bool   `json:"expression"`
	Method     string `json:"method"`
	SizeBytes  *int64 `json:"size_bytes,omitempty"`
	// Scans is how many times the index was read; nil when the server does
	// not track it.
	Scans *int64 `json:"scans,omitempty"`
}

// IndexReport is a database's indexes plus whether usage is known.
type IndexReport struct {
	Indexes []IndexDef `json:"indexes"`
	// UsageAvailable is false when the server keeps no per-index counters.
	UsageAvailable bool `json:"usage_available"`
	// StatsSince is when the counters started (server start or reset).
	StatsSince *time.Time `json:"stats_since,omitempty"`
	Note       string     `json:"note,omitempty"`
}

// IndexFinding is an index worth dropping, with the statement that does it.
type IndexFinding struct {
	Index IndexDef `json:"index"`
	// Reason says why; for a redundant index it names the covering one.
	Reason string `json:"reason"`
	// CoveredBy is the index that makes a redundant one unnecessary.
	CoveredBy string `json:"covered_by,omitempty"`
	DropSQL   string `json:"drop_sql"`
}

// IndexInsights is the analysis of an IndexReport.
type IndexInsights struct {
	Unused    []IndexFinding `json:"unused"`
	Redundant []IndexFinding `json:"redundant"`
	IndexReport
}

// AnalyzeIndexes finds unused and redundant indexes. dialect is "postgres"
// or "mysql" and only shapes the DROP statements.
//
// Redundant: an index whose columns repeat another index's, or are a
// leftmost prefix of another index of the same method on the same table —
// the longer index answers the same lookups. Unique, primary, partial and
// expression indexes are never redundant by prefix (uniqueness and the
// predicate are behaviour, not just speed); two identical indexes are, and
// the one whose name sorts later is reported.
//
// Unused: zero scans since StatsSince, not unique (unique indexes enforce
// constraints even when never read), not already reported as redundant.
func AnalyzeIndexes(rep *IndexReport, dialect string) *IndexInsights {
	out := &IndexInsights{Unused: []IndexFinding{}, Redundant: []IndexFinding{}, IndexReport: *rep}
	if out.Indexes == nil {
		out.Indexes = []IndexDef{}
	}
	byTable := map[string][]IndexDef{}
	var order []string
	for _, ix := range rep.Indexes {
		k := ix.Schema + "\x00" + ix.Table
		if _, ok := byTable[k]; !ok {
			order = append(order, k)
		}
		byTable[k] = append(byTable[k], ix)
	}
	redundant := map[string]bool{}
	for _, k := range order {
		ixs := byTable[k]
		for _, a := range ixs {
			if a.Primary || a.Partial || a.Expression || len(a.Columns) == 0 {
				continue
			}
			var cover *IndexDef
			for _, b := range ixs {
				if a.Name == b.Name || b.Partial || b.Expression || !strings.EqualFold(a.Method, b.Method) {
					continue
				}
				same := slices.Equal(a.Columns, b.Columns)
				prefix := len(a.Columns) < len(b.Columns) && slices.Equal(a.Columns, b.Columns[:len(a.Columns)])
				var covered bool
				switch {
				case same && a.Unique && !b.Unique:
					covered = false // a enforces uniqueness b does not
				case same && a.Unique == b.Unique:
					// Identical twins: keep the primary key, else the
					// alphabetically first.
					covered = b.Primary || (!a.Primary && a.Name > b.Name)
				case same:
					covered = true // b is unique, a is not
				case prefix:
					covered = !a.Unique
				}
				if covered && (cover == nil || coverRank(b) < coverRank(*cover) ||
					(coverRank(b) == coverRank(*cover) && b.Name < cover.Name)) {
					b := b
					cover = &b
				}
			}
			if cover == nil {
				continue
			}
			redundant[k+"\x00"+a.Name] = true
			out.Redundant = append(out.Redundant, IndexFinding{
				Index:     a,
				Reason:    fmt.Sprintf("%s covers the same lookups", cover.Name),
				CoveredBy: cover.Name,
				DropSQL:   dropIndexSQL(a, dialect),
			})
		}
	}
	if rep.UsageAvailable {
		for _, ix := range rep.Indexes {
			if ix.Unique || ix.Primary || ix.Scans == nil || *ix.Scans != 0 || redundant[ix.Schema+"\x00"+ix.Table+"\x00"+ix.Name] {
				continue
			}
			out.Unused = append(out.Unused, IndexFinding{
				Index:   ix,
				Reason:  "never used since statistics started",
				DropSQL: dropIndexSQL(ix, dialect),
			})
		}
	}
	return out
}

// coverRank orders candidate covering indexes: the one to keep is the
// primary key, else a unique index, else any.
func coverRank(ix IndexDef) int {
	switch {
	case ix.Primary:
		return 0
	case ix.Unique:
		return 1
	}
	return 2
}

func dropIndexSQL(ix IndexDef, dialect string) string {
	if dialect == "postgres" {
		// CONCURRENTLY does not block writes; it cannot run inside a
		// transaction block, which the console does not open.
		return "DROP INDEX CONCURRENTLY " + qualifiedTable(ix.Schema, ix.Name) + ";"
	}
	return fmt.Sprintf("DROP INDEX %s ON %s;", quoteMySQLIdent(ix.Name), quoteMySQLIdent(ix.Table))
}

// TopQueriesRequest selects and orders TopQueries.
type TopQueriesRequest struct {
	// Database limits the list to statements run in that database ("" = all).
	Database string
	// Sort is "total" (default), "mean" or "calls".
	Sort  string
	Limit int
}

// TopQuery is one normalized statement (literals replaced by placeholders)
// with its accumulated cost.
type TopQuery struct {
	Database string  `json:"database"`
	Query    string  `json:"query"`
	Calls    int64   `json:"calls"`
	TotalMS  float64 `json:"total_ms"`
	MeanMS   float64 `json:"mean_ms"`
	Rows     int64   `json:"rows"`
	// RowsExamined (MySQL/MariaDB) and NoIndexUsed (statements that scanned
	// without a good index) point at missing indexes.
	RowsExamined *int64 `json:"rows_examined,omitempty"`
	NoIndexUsed  *int64 `json:"no_index_used,omitempty"`
	// CacheHitPct (PostgreSQL) is the share of blocks found in shared buffers.
	CacheHitPct *float64 `json:"cache_hit_pct,omitempty"`
}

// TopQueries is the statement statistics of a server.
type TopQueries struct {
	Available  bool       `json:"available"`
	Note       string     `json:"note,omitempty"`
	StatsSince *time.Time `json:"stats_since,omitempty"`
	Queries    []TopQuery `json:"queries"`
}

func (r TopQueriesRequest) normalized() TopQueriesRequest {
	switch r.Sort {
	case "mean", "calls":
	default:
		r.Sort = "total"
	}
	if r.Limit <= 0 || r.Limit > 200 {
		r.Limit = 50
	}
	return r
}

// ExplainResult is a query plan. Format names the plan's shape so the
// dashboard can draw it: "postgres" (EXPLAIN FORMAT JSON), "mysql" /
// "mariadb" (EXPLAIN FORMAT=JSON or MariaDB's ANALYZE FORMAT=JSON), or
// "mysql-tree" (MySQL's EXPLAIN ANALYZE, which has no JSON form) in Text.
type ExplainResult struct {
	Format   string          `json:"format"`
	Analyzed bool            `json:"analyzed"`
	Plan     json.RawMessage `json:"plan,omitempty"`
	Text     string          `json:"text,omitempty"`
}

// explainable rejects anything but a single statement that is not itself an
// EXPLAIN or ANALYZE (the server adds those).
func explainable(stmt string) (string, error) {
	stmts := SplitStatements(stmt)
	if len(stmts) != 1 {
		return "", fmt.Errorf("explain takes exactly one statement, not %d", len(stmts))
	}
	s := stmts[0]
	switch leadingKeyword(s) {
	case "EXPLAIN", "ANALYZE", "DESCRIBE", "DESC":
		return "", fmt.Errorf("enter the statement itself; Fleetdock adds EXPLAIN")
	}
	return s, nil
}

// InsightsFor returns the performance surface for the engine name.
func InsightsFor(name string) (Insights, error) {
	c, err := For(name)
	if err != nil {
		return nil, err
	}
	in, ok := c.(Insights)
	if !ok {
		return nil, fmt.Errorf("engine: %s does not support performance insights", name)
	}
	return in, nil
}
