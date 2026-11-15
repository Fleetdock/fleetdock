package engine

import (
	"context"
	"time"
)

// Maintenance is PostgreSQL's housekeeping: sequences, materialized view
// refreshes, VACUUM / ANALYZE / REINDEX, table statistics and partitions.
type Maintenance interface {
	// Sequence reads a sequence's settings and current value.
	Sequence(ctx context.Context, p ConnParams, database, schema, name string) (*SequenceInfo, error)
	// SetSequence sets a sequence's value (setval). With called false the
	// next nextval returns value itself; with called true, the one after.
	SetSequence(ctx context.Context, p ConnParams, database, schema, name string, value int64, called bool) error
	// RefreshMaterializedView recomputes a materialized view.
	// Concurrently keeps it readable meanwhile; it needs a unique index.
	RefreshMaterializedView(ctx context.Context, p ConnParams, database, schema, name string, concurrently bool) error
	// Maintain runs VACUUM, VACUUM ANALYZE, ANALYZE or REINDEX on one table,
	// or on every table of a schema when table is empty, and returns what
	// it ran.
	Maintain(ctx context.Context, p ConnParams, database string, op MaintenanceOp, schema, table string) ([]string, error)
	// TableStats reads the size, live and dead rows and the last vacuum and
	// analyze of tables, in one schema or (empty) all of them.
	TableStats(ctx context.Context, p ConnParams, database, schema string) ([]TableStat, error)
	// Partitions lists the partitions of a partitioned table.
	Partitions(ctx context.Context, p ConnParams, database, schema, table string) ([]Partition, error)
}

// MaintenanceOp is a housekeeping command.
type MaintenanceOp string

const (
	OpVacuum        MaintenanceOp = "vacuum"
	OpVacuumAnalyze MaintenanceOp = "vacuum_analyze"
	OpAnalyze       MaintenanceOp = "analyze"
	OpReindex       MaintenanceOp = "reindex"
)

// Valid reports a known op.
func (o MaintenanceOp) Valid() bool {
	switch o {
	case OpVacuum, OpVacuumAnalyze, OpAnalyze, OpReindex:
		return true
	}
	return false
}

// SequenceInfo is a sequence and where it stands.
type SequenceInfo struct {
	Schema    string `json:"schema"`
	Name      string `json:"name"`
	DataType  string `json:"data_type"`
	Start     int64  `json:"start"`
	Increment int64  `json:"increment"`
	Min       int64  `json:"min"`
	Max       int64  `json:"max"`
	Cycle     bool   `json:"cycle"`
	// LastValue is the last value handed out; nil before the first nextval.
	LastValue *int64 `json:"last_value"`
	// NextValue is what the next nextval returns (unless another session
	// takes it first, or values are cached), nil when the sequence is used up.
	NextValue *int64 `json:"next_value"`
	// OwnedBy is the column the sequence belongs to ("schema.table.column"),
	// as with a serial or identity column.
	OwnedBy string `json:"owned_by,omitempty"`
}

// TableStat is a table's size and housekeeping state.
type TableStat struct {
	Schema          string     `json:"schema"`
	Name            string     `json:"name"`
	TotalBytes      int64      `json:"total_bytes"`
	LiveRows        int64      `json:"live_rows"`
	DeadRows        int64      `json:"dead_rows"`
	LastVacuum      *time.Time `json:"last_vacuum"`
	LastAutovacuum  *time.Time `json:"last_autovacuum"`
	LastAnalyze     *time.Time `json:"last_analyze"`
	LastAutoanalyze *time.Time `json:"last_autoanalyze"`
}

// Partition is one partition of a partitioned table.
type Partition struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	// Bound is the partition's bound, e.g. FOR VALUES FROM ('2026-01-01') TO ('2026-02-01').
	Bound      string `json:"bound"`
	TotalBytes int64  `json:"total_bytes"`
	Rows       int64  `json:"rows"`
	// Partitioned is set when the partition is split further.
	Partitioned bool `json:"partitioned"`
}
