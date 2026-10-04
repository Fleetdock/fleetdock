package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/Fleetdock/fleetdock/backend/internal/app/agent"
	summaryapp "github.com/Fleetdock/fleetdock/backend/internal/app/summary"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
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
	// Setup tracks the first-run steps; the dashboard shows a checklist until
	// every count is non-zero.
	Setup struct {
		Servers      int `json:"servers"`
		Instances    int `json:"instances"`
		Destinations int `json:"destinations"`
		Schedules    int `json:"schedules"`
		Channels     int `json:"channels"`
	} `json:"setup"`
	Attention []attentionItem `json:"attention"`
}

type attentionItem struct {
	Kind         string    `json:"kind"`
	Severity     string    `json:"severity"`
	ResourceType string    `json:"resource_type"`
	ResourceID   uuid.UUID `json:"resource_id"`
	Name         string    `json:"name"`
	Message      string    `json:"message"`
	Since        time.Time `json:"since"`
}

// maxAttention bounds the attention list; the repository is asked for more so
// that items the caller may not see can be dropped without starving the list.
const maxAttention = 20

func toOverviewResponse(s statsdom.Summary) overviewResponse {
	var out overviewResponse
	out.Servers.Total, out.Servers.Online, out.Servers.Offline = s.ServersTotal, s.ServersOnline, s.ServersOffline
	out.Instances.Total, out.Instances.Managed, out.Instances.External = s.InstancesTotal, s.InstancesManaged, s.InstancesExternal
	out.Databases.Total, out.Databases.Active = s.DatabasesTotal, s.DatabasesActive
	out.Backups.Completed24h, out.Backups.Failed24h, out.Backups.LastBackupAt = s.BackupsCompleted24h, s.BackupsFailed24h, s.LastBackupAt
	out.Operations.Running, out.Operations.Failed24h = s.OperationsRunning, s.OperationsFailed24h
	out.Automation.SchedulesEnabled, out.Automation.ChannelsEnabled, out.Automation.RulesEnabled = s.SchedulesEnabled, s.ChannelsEnabled, s.RulesEnabled
	out.Setup.Servers, out.Setup.Instances, out.Setup.Destinations = s.ServersTotal, s.InstancesTotal, s.DestinationsTotal
	out.Setup.Schedules, out.Setup.Channels = s.SchedulesEnabled, s.ChannelsEnabled
	out.Attention = []attentionItem{}
	return out
}

// Overview handles GET /v1/overview.
func (h *OverviewHandler) Overview(w http.ResponseWriter, r *http.Request) {
	s, err := h.summary.Get(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := toOverviewResponse(s)
	items, err := h.summary.Attention(r.Context(), 5*maxAttention)
	if err != nil {
		writeError(w, err)
		return
	}
	p := principalFrom(r.Context())
	for _, a := range items {
		if len(out.Attention) == maxAttention {
			break
		}
		if !p.CanOn(statsdom.AttentionPerm(a.Kind), attentionAncestry(a)) {
			continue
		}
		out.Attention = append(out.Attention, attentionItem{
			Kind: a.Kind, Severity: a.Severity, ResourceType: a.ResourceType, ResourceID: a.ResourceID,
			Name: a.Name, Message: a.Message, Since: a.Since,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// attentionAncestry mirrors authzapp.Resolver for an attention item, from the
// lineage the query already returned (no extra round-trips).
func attentionAncestry(a statsdom.Attention) authz.Ancestry {
	var anc authz.Ancestry
	if a.DatabaseID != uuid.Nil {
		anc.Covers = append(anc.Covers, authz.Scope{Type: authz.ScopeDatabase, ID: a.DatabaseID})
	}
	if a.ServerID != uuid.Nil {
		anc.Covers = append(anc.Covers, authz.Scope{Type: authz.ScopeServer, ID: a.ServerID})
	}
	return anc
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
