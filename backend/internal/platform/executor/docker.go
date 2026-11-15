package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// runProvision performs a Docker container lifecycle action for a managed
// instance. It runs on the agent (which has Docker); the control-plane worker
// never receives these jobs.
func runProvision(ctx context.Context, jobType string, p *Payload, sink LogSink) (*Result, error) {
	if p.Provision == nil {
		return nil, fmt.Errorf("provision spec is missing")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("docker is not installed on this server (required for provisioning)")
	}
	spec := p.Provision

	switch jobType {
	case "provision_instance":
		sink.Log("info", "provisioning container "+spec.ContainerName)
		return provisionContainer(ctx, p.Engine, spec, sink)
	case "start_instance":
		sink.Log("info", "starting container "+spec.ContainerName)
		return &Result{OK: true}, dockerSimple(ctx, "start", spec.ContainerName)
	case "stop_instance":
		sink.Log("info", "stopping container "+spec.ContainerName)
		return &Result{OK: true}, dockerSimple(ctx, "stop", spec.ContainerName)
	case "restart_instance":
		sink.Log("info", "restarting container "+spec.ContainerName)
		return &Result{OK: true}, dockerSimple(ctx, "restart", spec.ContainerName)
	case "remove_instance":
		sink.Log("info", "removing container "+spec.ContainerName)
		return removeContainer(ctx, spec)
	}
	return nil, fmt.Errorf("unsupported provision operation %q", jobType)
}

// engineContainer holds the Docker specifics of a database engine image.
type engineContainer struct {
	rootEnv      string // env var carrying the superuser password
	dataPath     string // container path to persist as a volume
	internalPort int    // port the engine listens on inside the container
	// env is extra, non-secret environment for the container.
	env []string
	// runArgs are extra `docker run` flags.
	runArgs []string
	// readyCmd, when set, is run inside the container until it succeeds:
	// provisioning reports success only once the engine answers.
	readyCmd []string
}

// readyTimeout bounds the wait for a new container's readyCmd.
var readyTimeout = 90 * time.Second

func engineContainerFor(eng, version string) engineContainer {
	switch eng {
	case "clickhouse":
		return engineContainer{
			rootEnv: "CLICKHOUSE_PASSWORD", dataPath: "/var/lib/clickhouse", internalPort: 8123,
			// The default user manages accounts and grants with SQL, which
			// Fleetdock's console roles and credentials need.
			env:      []string{"CLICKHOUSE_USER=default", "CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1"},
			runArgs:  []string{"--ulimit", "nofile=262144:262144"},
			readyCmd: []string{"wget", "-qO-", "http://127.0.0.1:8123/ping"},
		}
	case "postgres":
		// From 18 the official image keeps its data under
		// /var/lib/postgresql/<major>/docker and refuses to start with a
		// volume on the old /var/lib/postgresql/data path.
		if major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0]); err == nil && major >= 18 {
			return engineContainer{rootEnv: "POSTGRES_PASSWORD", dataPath: "/var/lib/postgresql", internalPort: 5432}
		}
		return engineContainer{rootEnv: "POSTGRES_PASSWORD", dataPath: "/var/lib/postgresql/data", internalPort: 5432}
	case "mysql":
		return engineContainer{rootEnv: "MYSQL_ROOT_PASSWORD", dataPath: "/var/lib/mysql", internalPort: 3306}
	default: // mariadb
		return engineContainer{rootEnv: "MARIADB_ROOT_PASSWORD", dataPath: "/var/lib/mysql", internalPort: 3306}
	}
}

// provisionContainer launches a database engine container with a named volume
// and a published port. The root password is passed via a temporary env-file
// (removed immediately) so it never appears in the process list.
func provisionContainer(ctx context.Context, eng string, spec *ProvisionSpec, sink LogSink) (*Result, error) {
	image := spec.Image
	if image == "" {
		image = eng // engine name doubles as the Docker Hub image (e.g. "mariadb")
	}
	if spec.Version != "" {
		image = image + ":" + spec.Version
	}
	ec := engineContainerFor(eng, spec.Version)

	// Best-effort cleanup of any stale container with the same name.
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", spec.ContainerName).Run()

	envFile, err := writeEnvFile(append([]string{ec.rootEnv + "=" + spec.RootPassword}, ec.env...)...)
	if err != nil {
		return nil, err
	}
	defer os.Remove(envFile)

	args := []string{
		"run", "-d",
		"--name", spec.ContainerName,
		"--restart", "unless-stopped",
		"--env-file", envFile,
		"-v", spec.Volume + ":" + ec.dataPath,
		"-p", strconv.Itoa(spec.Port) + ":" + strconv.Itoa(ec.internalPort),
		"--label", "app=fleetdock",
	}
	args = append(append(args, ec.runArgs...), image)
	out, err := runDocker(ctx, args...)
	if err != nil {
		return nil, err
	}
	containerID := strings.TrimSpace(out)
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}
	if len(ec.readyCmd) > 0 {
		sink.Log("info", "waiting for the server to accept connections")
		if err := waitReady(ctx, spec.ContainerName, ec.readyCmd); err != nil {
			// The operation fails, so nothing will ever manage this
			// container: left running (--restart unless-stopped) it would
			// hold the port with its admin login exposed. The volume is the
			// one just created for this new instance.
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, rerr := removeContainer(cctx, &ProvisionSpec{ContainerName: spec.ContainerName, Volume: spec.Volume, RemoveVolume: true}); rerr != nil {
				sink.Log("warn", "could not remove the failed container: "+rerr.Error())
			} else {
				sink.Log("info", "removed the container that failed to start")
			}
			return nil, err
		}
		sink.Log("info", "server is ready")
	}
	return &Result{OK: true, ContainerID: containerID}, nil
}

// waitReady runs cmd in the container until it succeeds, the container
// stops or starts crash-looping, or readyTimeout passes.
func waitReady(ctx context.Context, container string, cmd []string) error {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	for {
		if _, err := runDocker(ctx, append([]string{"exec", container}, cmd...)...); err == nil {
			return nil
		}
		if out, err := runDocker(ctx, "inspect", "-f", "{{.State.Running}} {{.State.Restarting}} {{.RestartCount}}", container); err == nil {
			if crashed(out) {
				return fmt.Errorf("the container stopped while starting: %s", lastLogLine(ctx, container))
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the server did not become ready within %s", readyTimeout)
		case <-time.After(time.Second):
		}
	}
}

// crashed reports, from "{{.State.Running}} {{.State.Restarting}}
// {{.RestartCount}}", a container that stopped or is crash-looping: with a
// restart policy one that keeps failing reads as running most of the time,
// so a restart already counts.
func crashed(inspect string) bool {
	f := strings.Fields(inspect)
	if len(f) != 3 {
		return false
	}
	return f[0] == "false" || f[1] == "true" || f[2] != "0"
}

// lastLogLine returns the container's most telling recent log line: the last
// one mentioning an error, or else the last one. Both streams are read, since
// servers such as ClickHouse log to stderr.
func lastLogLine(ctx context.Context, container string) string {
	out, _ := exec.CommandContext(ctx, "docker", "logs", "--tail", "50", container).CombinedOutput()
	return pickLogLine(string(out))
}

func pickLogLine(logs string) string {
	var last, lastErr string
	for _, l := range strings.Split(logs, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		last = l
		if lo := strings.ToLower(l); strings.Contains(lo, "error") || strings.Contains(lo, "exception") || strings.Contains(lo, "fatal") {
			lastErr = l
		}
	}
	line := last
	if lastErr != "" {
		line = lastErr
	}
	if len(line) > 500 {
		line = line[:500] + "…"
	}
	if line == "" {
		return "no log output"
	}
	return line
}

func removeContainer(ctx context.Context, spec *ProvisionSpec) (*Result, error) {
	if _, err := runDocker(ctx, "rm", "-f", spec.ContainerName); err != nil {
		return nil, err
	}
	if spec.RemoveVolume && spec.Volume != "" {
		// Volume removal is best-effort: the container removal is what matters.
		_ = exec.CommandContext(ctx, "docker", "volume", "rm", spec.Volume).Run()
	}
	return &Result{OK: true}, nil
}

func dockerSimple(ctx context.Context, action, container string) error {
	_, err := runDocker(ctx, action, container)
	return err
}

func runDocker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := firstLine(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("docker %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

func writeEnvFile(lines ...string) (string, error) {
	f, err := os.CreateTemp("", "dbm-env-*")
	if err != nil {
		return "", fmt.Errorf("create env file: %w", err)
	}
	if err := os.Chmod(f.Name(), 0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
