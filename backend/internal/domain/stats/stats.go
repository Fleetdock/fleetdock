// Package stats is the domain model for aggregate control-plane statistics
// shown on the overview dashboard.
package stats

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Summary is a snapshot of fleet-wide counts.
type Summary struct {
	ServersTotal   int
	ServersOnline  int
	ServersOffline int

	InstancesTotal    int
	InstancesManaged  int
	InstancesExternal int

	DatabasesTotal  int
	DatabasesActive int

	BackupsCompleted24h int
	BackupsFailed24h    int
	LastBackupAt        *time.Time

	OperationsRunning   int
	OperationsFailed24h int

	SchedulesEnabled  int
	ChannelsEnabled   int
	RulesEnabled      int
	DestinationsTotal int
}

// Attention kinds: problems the overview surfaces, most urgent first.
const (
	KindServerOffline       = "server_offline"
	KindInstanceUnreachable = "instance_unreachable"
	KindBackupFailed        = "backup_failed"
	KindBackupCheckFailed   = "backup_check_failed"
	KindNoRecentBackup      = "no_recent_backup"
	KindDatabaseMissing     = "database_missing"
	KindOperationFailed     = "operation_failed"
)

// Severities of an attention item.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
)

// Attention is one problem that needs a person to look at it. ServerID and
// DatabaseID locate it for per-resource authorization (either may be Nil).
type Attention struct {
	Kind         string
	Severity     string
	ResourceType string
	ResourceID   uuid.UUID
	Name         string
	Message      string
	Since        time.Time
	ServerID     uuid.UUID
	DatabaseID   uuid.UUID
}

// AttentionPerm is the read permission needed to see an attention item.
func AttentionPerm(kind string) string {
	switch kind {
	case KindServerOffline:
		return "server:read"
	case KindInstanceUnreachable:
		return "instance:read"
	case KindDatabaseMissing:
		return "database:read"
	case KindOperationFailed:
		return "operation:read"
	default:
		return "backup:read"
	}
}

// Repository is the persistence port for aggregate statistics.
type Repository interface {
	Summary(ctx context.Context) (Summary, error)
	// Attention returns up to limit open problems, critical first, newest first.
	Attention(ctx context.Context, limit int) ([]Attention, error)
}
