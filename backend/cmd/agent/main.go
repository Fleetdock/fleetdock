// Command agent is the Fleetdock server agent. It enrolls with the control
// plane using a single-use registration token, then heartbeats and executes
// operations (database create/drop, backups, restores) against local
// database instances.
//
// Configuration (environment):
//
//	FLEETDOCK_URL        control plane base URL (required)
//	FLEETDOCK_TOKEN      registration token (required until enrolled)
//	FLEETDOCK_STATE_DIR  state directory (default /var/lib/fleetdock-agent)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TajBrains/fleetdock/backend/internal/platform/executor"
)

const version = "0.1.0"

type state struct {
	ServerID   string `json:"server_id"`
	AgentToken string `json:"agent_token"`
}

type agent struct {
	baseURL string
	http    *http.Client
	state   state
	stateFP string
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	baseURL := strings.TrimSuffix(os.Getenv("FLEETDOCK_URL"), "/")
	if baseURL == "" {
		return errors.New("FLEETDOCK_URL is required")
	}
	stateDir := os.Getenv("FLEETDOCK_STATE_DIR")
	if stateDir == "" {
		stateDir = "/var/lib/fleetdock-agent"
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}

	a := &agent{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
		stateFP: filepath.Join(stateDir, "state.json"),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := a.loadState(); err != nil {
		slog.Info("no saved state; enrolling with control plane")
		if err := a.register(ctx); err != nil {
			return fmt.Errorf("registration failed: %w", err)
		}
	}
	slog.Info("agent ready", "server_id", a.state.ServerID, "control_plane", a.baseURL)

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	claim := time.NewTicker(5 * time.Second)
	defer claim.Stop()

	a.sendHeartbeat(ctx) // immediately on boot
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return nil
		case <-heartbeat.C:
			a.sendHeartbeat(ctx)
		case <-claim.C:
			for a.claimAndRun(ctx) {
			}
		}
	}
}

// ---- enrollment ----

func (a *agent) register(ctx context.Context) error {
	token := os.Getenv("FLEETDOCK_TOKEN")
	if token == "" {
		return errors.New("FLEETDOCK_TOKEN is required for first-time registration")
	}
	hostname, _ := os.Hostname()
	osName := detectOS()

	body := map[string]any{
		"token":         token,
		"hostname":      hostname,
		"os":            osName,
		"agent_version": version,
	}
	if addr := primaryAddress(a.baseURL); addr != "" {
		body["address"] = addr
	}
	var resp struct {
		ServerID   string `json:"server_id"`
		ServerName string `json:"server_name"`
		AgentToken string `json:"agent_token"`
	}
	if err := a.post(ctx, "/agent/v1/register", body, &resp, false); err != nil {
		return err
	}
	a.state = state{ServerID: resp.ServerID, AgentToken: resp.AgentToken}
	if err := a.saveState(); err != nil {
		return err
	}
	slog.Info("enrolled", "server_name", resp.ServerName, "server_id", resp.ServerID)
	return nil
}

func (a *agent) loadState() error {
	raw, err := os.ReadFile(a.stateFP)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &a.state); err != nil {
		return err
	}
	if a.state.AgentToken == "" {
		return errors.New("empty state")
	}
	return nil
}

func (a *agent) saveState() error {
	raw, err := json.Marshal(a.state)
	if err != nil {
		return err
	}
	return os.WriteFile(a.stateFP, raw, 0o600)
}

// ---- heartbeat ----

func (a *agent) sendHeartbeat(ctx context.Context) {
	info := map[string]any{
		"agent_version": version,
		"os":            detectOS(),
		"docker_ok":     dockerOK(),
	}
	if v := mariadbVersion(); v != "" {
		info["mariadb_version"] = v
	}
	if used, total, err := memInfo(); err == nil {
		info["mem_used_bytes"] = used
		info["mem_total_bytes"] = total
	}
	if used, total, err := diskInfo("/"); err == nil {
		info["disk_used_bytes"] = used
		info["disk_total_bytes"] = total
	}
	if addr := primaryAddress(a.baseURL); addr != "" {
		info["address"] = addr
	}
	if err := a.post(ctx, "/agent/v1/heartbeat", info, nil, true); err != nil {
		slog.Warn("heartbeat failed", "error", err.Error())
	}
}

// ---- job loop ----

type claimedJob struct {
	Job struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"job"`
	Payload *executor.Payload `json:"payload"`
}

// claimAndRun claims one job and executes it; reports whether a job ran.
func (a *agent) claimAndRun(ctx context.Context) bool {
	var cj claimedJob
	err := a.post(ctx, "/agent/v1/jobs/claim", map[string]any{}, &cj, true)
	if err != nil {
		if !errors.Is(err, errNoContent) {
			slog.Warn("claim failed", "error", err.Error())
		}
		return false
	}
	if cj.Job.ID == "" {
		return false
	}

	slog.Info("executing operation", "id", cj.Job.ID, "type", cj.Job.Type)
	execCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)

	// Ship logs back to the control plane in batches (threshold 25) to cut HTTP
	// round-trips; a final Flush below persists the tail before we report status.
	jobID := cj.Job.ID
	sink := executor.NewBufferedSink(25, func(lines []executor.LogLine) error {
		return a.postLogs(ctx, jobID, lines)
	}, func(err error) {
		slog.Warn("ship operation logs failed", "id", jobID, "error", err.Error())
	})

	result, execErr := executor.Execute(execCtx, cj.Job.Type, cj.Payload, sink)
	cancel()
	sink.Flush() // persist remaining logs before the job is finalized

	report := map[string]any{"status": "succeeded", "result": json.RawMessage(result)}
	if execErr != nil {
		slog.Warn("operation failed", "id", cj.Job.ID, "error", execErr.Error())
		report = map[string]any{"status": "failed", "error": execErr.Error()}
	} else {
		slog.Info("operation succeeded", "id", cj.Job.ID)
	}
	if err := a.post(ctx, "/agent/v1/jobs/"+cj.Job.ID+"/status", report, nil, true); err != nil {
		slog.Error("report job status failed", "id", cj.Job.ID, "error", err.Error())
	}
	return true // keep draining the queue either way
}

// postLogs ships a batch of execution log lines for a job to the control plane.
func (a *agent) postLogs(ctx context.Context, jobID string, lines []executor.LogLine) error {
	logs := make([]map[string]any, len(lines))
	for i, l := range lines {
		logs[i] = map[string]any{"seq": l.Seq, "level": l.Level, "message": l.Message}
	}
	return a.post(ctx, "/agent/v1/jobs/"+jobID+"/logs", map[string]any{"logs": logs}, nil, true)
}

// ---- http ----

var errNoContent = errors.New("no content")

func (a *agent) post(ctx context.Context, path string, body any, out any, authed bool) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if authed {
		req.Header.Set("Authorization", "Bearer "+a.state.AgentToken)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return errNoContent
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ---- host info ----

func detectOS() string {
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return runtime.GOOS
}

func dockerOK() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.Command("docker", "info").Run() == nil
}

func mariadbVersion() string {
	for _, bin := range []string{"mariadbd", "mariadb", "mysqld", "mysql"} {
		p, err := exec.LookPath(bin)
		if err != nil {
			continue
		}
		out, err := exec.Command(p, "--version").Output()
		if err != nil {
			continue
		}
		return strings.TrimSpace(string(out))
	}
	return ""
}

func memInfo() (used, total int64, err error) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	var totalKB, availKB int64
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseInt(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	if totalKB == 0 {
		return 0, 0, errors.New("no meminfo")
	}
	return (totalKB - availKB) * 1024, totalKB * 1024, nil
}

func diskInfo(path string) (used, total int64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	total = int64(st.Blocks) * int64(st.Bsize)
	free := int64(st.Bavail) * int64(st.Bsize)
	return total - free, total, nil
}
