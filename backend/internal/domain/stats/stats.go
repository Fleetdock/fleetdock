// Package stats is the domain model for aggregate control-plane statistics
// shown on the overview dashboard.
package stats

import (
	"context"
	"time"
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

	SchedulesEnabled int
	ChannelsEnabled  int
	RulesEnabled     int
}

// Repository is the persistence port for aggregate statistics.
type Repository interface {
	Summary(ctx context.Context) (Summary, error)
}
