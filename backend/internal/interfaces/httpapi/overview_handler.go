package httpapi

import (
	"net/http"
	"time"

	agentapp "github.com/Fleetdock/fleetdock/backend/internal/app/agent"
	summaryapp "github.com/Fleetdock/fleetdock/backend/internal/app/summary"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	statsdom "github.com/Fleetdock/fleetdock/backend/internal/domain/stats"
)

// OverviewHandler exposes the dashboard summary and server metrics.
type OverviewHandler struct {
	summary *summaryapp.Service
	agents  *agentapp.Service
}

// NewOverviewHandler builds the overview handler.
func NewOverviewHandler(summary *summaryapp.Service, agents *agentapp.Service) *OverviewHandler {
	return &OverviewHandler{summary: summary, agents: agents}
}

type overviewResponse struct {
	Servers struct {
		Total   int `json:"total"`
		Online  int `json:"online"`
		Offline int `json:"offline"`
	} `json:"servers"`
	Instances struct {
		Total    int `json:"total"`
		Managed  int `json:"managed"`
		External int `json:"external"`
	} `json:"instances"`
	Databases struct {
		Total  int `json:"total"`
		Active int `json:"active"`
	} `json:"databases"`
	Backups struct {
		Completed24h int        `json:"completed_24h"`
		Failed24h    int        `json:"failed_24h"`
		LastBackupAt *time.Time `json:"last_backup_at,omitempty"`
	} `json:"backups"`
	Operations struct {
		Running   int `json:"running"`
		Failed24h int `json:"failed_24h"`
	} `json:"operations"`
	Automation struct {
		SchedulesEnabled int `json:"schedules_enabled"`
		ChannelsEnabled  int `json:"channels_enabled"`
		RulesEnabled     int `json:"rules_enabled"`
	} `json:"automation"`
}

func toOverviewResponse(s statsdom.Summary) overviewResponse {
	var out overviewResponse
	out.Servers.Total, out.Servers.Online, out.Servers.Offline = s.ServersTotal, s.ServersOnline, s.ServersOffline
	out.Instances.Total, out.Instances.Managed, out.Instances.External = s.InstancesTotal, s.InstancesManaged, s.InstancesExternal
	out.Databases.Total, out.Databases.Active = s.DatabasesTotal, s.DatabasesActive
	out.Backups.Completed24h, out.Backups.Failed24h, out.Backups.LastBackupAt = s.BackupsCompleted24h, s.BackupsFailed24h, s.LastBackupAt
	out.Operations.Running, out.Operations.Failed24h = s.OperationsRunning, s.OperationsFailed24h
	out.Automation.SchedulesEnabled, out.Automation.ChannelsEnabled, out.Automation.RulesEnabled = s.SchedulesEnabled, s.ChannelsEnabled, s.RulesEnabled
	return out
}

// Overview handles GET /v1/overview.
func (h *OverviewHandler) Overview(w http.ResponseWriter, r *http.Request) {
	s, err := h.summary.Get(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOverviewResponse(s))
}

type metricSample struct {
	CollectedAt       time.Time `json:"collected_at"`
	CPUPct            *float64  `json:"cpu_pct,omitempty"`
	MemUsedBytes      *int64    `json:"mem_used_bytes,omitempty"`
	MemTotalBytes     *int64    `json:"mem_total_bytes,omitempty"`
	DiskUsedBytes     *int64    `json:"disk_used_bytes,omitempty"`
	DiskTotalBytes    *int64    `json:"disk_total_bytes,omitempty"`
	ActiveConnections *int      `json:"active_connections,omitempty"`
}

func toMetricSample(h serverdom.HealthSample) metricSample {
	return metricSample{
		CollectedAt:       h.CollectedAt,
		CPUPct:            h.CPUPct,
		MemUsedBytes:      h.MemUsedBytes,
		MemTotalBytes:     h.MemTotalBytes,
		DiskUsedBytes:     h.DiskUsedBytes,
		DiskTotalBytes:    h.DiskTotalBytes,
		ActiveConnections: h.ActiveConnections,
	}
}

// ServerMetrics handles GET /v1/servers/{id}/metrics?hours=6.
func (h *OverviewHandler) ServerMetrics(w http.ResponseWriter, r *http.Request) {
	hours := atoiDefault(r.URL.Query().Get("hours"), 6)
	if hours <= 0 || hours > 168 {
		hours = 6
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	samples, err := h.agents.ServerMetrics(r.Context(), r.PathValue("id"), since)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]metricSample, 0, len(samples))
	for _, s := range samples {
		out = append(out, toMetricSample(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
