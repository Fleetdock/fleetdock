package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	agentapp "github.com/TajBrains/fleetdock/backend/internal/app/agent"
	operationapp "github.com/TajBrains/fleetdock/backend/internal/app/operation"
	jobdom "github.com/TajBrains/fleetdock/backend/internal/domain/job"
	serverdom "github.com/TajBrains/fleetdock/backend/internal/domain/server"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

// AgentHandler implements the control-plane side of the agent protocol:
// enrollment, heartbeats, and the job claim/report loop. These endpoints
// live under /agent/v1 and use agent bearer tokens, not user JWTs.
type AgentHandler struct {
	agents *agentapp.Service
	ops    *operationapp.Service
}

// NewAgentHandler builds the agent-protocol handler.
func NewAgentHandler(agents *agentapp.Service, ops *operationapp.Service) *AgentHandler {
	return &AgentHandler{agents: agents, ops: ops}
}

type agentCtxKey int

const agentServerCtxKey agentCtxKey = iota

func agentServerFrom(ctx context.Context) *serverdom.Server {
	s, _ := ctx.Value(agentServerCtxKey).(*serverdom.Server)
	return s
}

// Auth authenticates agent requests via their bearer token.
func (h *AgentHandler) Auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srv, err := h.agents.Authenticate(r.Context(), bearerToken(r))
		if err != nil {
			writeError(w, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), agentServerCtxKey, srv)))
	}
}

// ---- Registration ----

type agentRegisterRequest struct {
	Token        string  `json:"token"`
	Hostname     string  `json:"hostname"`
	Address      *string `json:"address"`
	OS           *string `json:"os"`
	AgentVersion string  `json:"agent_version"`
}

// Register handles POST /agent/v1/register (public; guarded by the
// single-use registration token).
func (h *AgentHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req agentRegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	srv, agentToken, err := h.agents.Register(r.Context(), agentapp.RegisterInput{
		Token:        req.Token,
		Hostname:     req.Hostname,
		Address:      req.Address,
		OS:           req.OS,
		AgentVersion: req.AgentVersion,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"server_id":   srv.ID.String(),
		"server_name": srv.Name,
		"agent_token": agentToken,
	})
}

// ---- Heartbeat ----

type heartbeatRequest struct {
	AgentVersion      string   `json:"agent_version"`
	MariaDBVersion    *string  `json:"mariadb_version"`
	OS                *string  `json:"os"`
	CPUPct            *float64 `json:"cpu_pct"`
	MemUsedBytes      *int64   `json:"mem_used_bytes"`
	MemTotalBytes     *int64   `json:"mem_total_bytes"`
	DiskUsedBytes     *int64   `json:"disk_used_bytes"`
	DiskTotalBytes    *int64   `json:"disk_total_bytes"`
	ActiveConnections *int     `json:"active_connections"`
	DockerOK          *bool    `json:"docker_ok"`
}

// Heartbeat handles POST /agent/v1/heartbeat.
func (h *AgentHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	srv := agentServerFrom(r.Context())
	var req heartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	err := h.agents.Heartbeat(r.Context(), srv.ID, serverdom.HeartbeatInfo{
		AgentVersion:      req.AgentVersion,
		MariaDBVersion:    req.MariaDBVersion,
		OS:                req.OS,
		CPUPct:            req.CPUPct,
		MemUsedBytes:      req.MemUsedBytes,
		MemTotalBytes:     req.MemTotalBytes,
		DiskUsedBytes:     req.DiskUsedBytes,
		DiskTotalBytes:    req.DiskTotalBytes,
		ActiveConnections: req.ActiveConnections,
		DockerOK:          req.DockerOK,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- Job claim / report ----

// Claim handles POST /agent/v1/jobs/claim: returns the next pending job for
// this server with its enriched payload, or 204 if there is none.
func (h *AgentHandler) Claim(w http.ResponseWriter, r *http.Request) {
	srv := agentServerFrom(r.Context())
	j, payload, err := h.ops.Claim(r.Context(), &srv.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	if j == nil {
		writeJSON(w, http.StatusNoContent, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job": map[string]any{
			"id":   j.ID.String(),
			"type": string(j.Type),
		},
		"payload": payload,
	})
}

// jobLogsRequest carries a batch of execution log lines an agent produced
// while running a job. Seq is monotonic per job (assigned agent-side).
type jobLogsRequest struct {
	Logs []struct {
		Seq     int    `json:"seq"`
		Level   string `json:"level"`
		Message string `json:"message"`
	} `json:"logs"`
}

// AppendLogs handles POST /agent/v1/jobs/{id}/logs.
func (h *AgentHandler) AppendLogs(w http.ResponseWriter, r *http.Request) {
	srv := agentServerFrom(r.Context())
	jobID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, apperr.Invalid("id", "id must be a valid UUID"))
		return
	}
	j, err := h.ops.Get(r.Context(), jobID.String())
	if err != nil {
		writeError(w, err)
		return
	}
	if j.ServerID == nil || *j.ServerID != srv.ID {
		writeError(w, apperr.Forbidden("job does not belong to this agent"))
		return
	}

	var req jobLogsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	lines := make([]jobdom.JobLog, 0, len(req.Logs))
	for _, l := range req.Logs {
		lines = append(lines, jobdom.JobLog{JobID: jobID, Seq: l.Seq, Level: l.Level, Message: l.Message})
	}
	if err := h.ops.AppendLogs(r.Context(), jobID, lines); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type jobStatusRequest struct {
	Status   string          `json:"status"` // succeeded | failed | running
	Progress *int            `json:"progress"`
	Result   json.RawMessage `json:"result"`
	Error    *string         `json:"error"`
}

// UpdateJob handles POST /agent/v1/jobs/{id}/status.
func (h *AgentHandler) UpdateJob(w http.ResponseWriter, r *http.Request) {
	srv := agentServerFrom(r.Context())
	jobID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, apperr.Invalid("id", "id must be a valid UUID"))
		return
	}
	j, err := h.ops.Get(r.Context(), jobID.String())
	if err != nil {
		writeError(w, err)
		return
	}
	if j.ServerID == nil || *j.ServerID != srv.ID {
		writeError(w, apperr.Forbidden("job does not belong to this agent"))
		return
	}

	var req jobStatusRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	switch req.Status {
	case "running":
		if req.Progress != nil {
			if err := h.ops.UpdateProgress(r.Context(), jobID, *req.Progress); err != nil {
				writeError(w, err)
				return
			}
		}
	case "succeeded":
		if err := h.ops.Complete(r.Context(), jobID, jobdom.StatusSucceeded, req.Result, nil); err != nil {
			writeError(w, err)
			return
		}
	case "failed":
		if err := h.ops.Complete(r.Context(), jobID, jobdom.StatusFailed, req.Result, req.Error); err != nil {
			writeError(w, err)
			return
		}
	default:
		writeError(w, apperr.Invalid("status", "status must be running, succeeded or failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
