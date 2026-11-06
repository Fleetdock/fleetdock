package executor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
		if spec.TLS != nil {
			sink.Log("info", "with TLS (certificate valid until "+spec.TLS.NotAfter+")")
		}
		return provisionContainer(ctx, p.Engine, spec, imageRef(p.Engine, spec), sink)
	case "configure_tls":
		return configureTLS(ctx, p.Engine, spec, sink)
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
// (removed immediately) so it never appears in the process list. With
// spec.TLS the certificate is written on the host and mounted read-only, and
// the engine is started with TLS on.
func provisionContainer(ctx context.Context, eng string, spec *ProvisionSpec, image string, sink LogSink) (*Result, error) {
	if spec.TLS != nil && eng == "clickhouse" {
		return nil, fmt.Errorf("TLS is not supported yet for ClickHouse servers Fleetdock creates")
	}
	ec := engineContainerFor(eng, spec.Version)
	var tlsDir string
	if spec.TLS != nil {
		dir, err := writeTLSFiles(spec.ContainerName, spec.TLS)
		if err != nil {
			return nil, err
		}
		tlsDir = dir
	}

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
	if tlsDir != "" {
		args = append(args, "-v", tlsDir+":"+tlsMountPath+":ro")
	}
	args = append(append(args, ec.runArgs...), image)
	if tlsDir != "" {
		args = append(args, tlsArgs(eng)...)
	}
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
			return nil, err
		}
		sink.Log("info", "server is ready")
	}
	res := &Result{OK: true, ContainerID: containerID}
	if spec.TLS != nil {
		res.TLSNotAfter = spec.TLS.NotAfter
	}
	return res, nil
}

// tlsMountPath is where the certificate directory is mounted in the container.
const tlsMountPath = "/etc/fleetdock-tls"

// engineUID is the user the official postgres, mysql and mariadb images run
// the engine as. PostgreSQL refuses a private key that user can't own.
const engineUID = 999

// tlsArgs are the engine arguments that turn TLS on with the mounted files.
// They replace the image's default command: the postgres entrypoint takes
// "postgres …", and the mysql/mariadb entrypoints prepend the server binary
// to arguments starting with "-".
func tlsArgs(eng string) []string {
	cert := tlsMountPath + "/server.crt"
	key := tlsMountPath + "/server.key"
	ca := tlsMountPath + "/ca.crt"
	if eng == "postgres" {
		return []string{"postgres", "-c", "ssl=on", "-c", "ssl_cert_file=" + cert, "-c", "ssl_key_file=" + key, "-c", "ssl_ca_file=" + ca}
	}
	return []string{"--ssl-cert=" + cert, "--ssl-key=" + key, "--ssl-ca=" + ca}
}

// tlsBaseDir is where the agent keeps instance certificates on the host.
func tlsBaseDir() string {
	dir := os.Getenv("FLEETDOCK_STATE_DIR")
	if dir == "" {
		dir = "/var/lib/fleetdock-agent"
	}
	return filepath.Join(dir, "tls")
}

// writeTLSFiles writes an instance's certificate, key and CA certificate to
// its directory on the host (replacing earlier ones) and hands them to the
// engine's user. Returns the directory to mount.
func writeTLSFiles(container string, t *TLSFiles) (string, error) {
	if t.CertPEM == "" || t.KeyPEM == "" {
		return "", fmt.Errorf("TLS certificate is missing")
	}
	dir := filepath.Join(tlsBaseDir(), container)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create TLS directory: %w", err)
	}
	files := []struct {
		name string
		data string
		mode os.FileMode
	}{
		{"server.crt", t.CertPEM, 0o644},
		{"server.key", t.KeyPEM, 0o600},
		{"ca.crt", t.CAPEM, 0o644},
	}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		tmp := path + ".new"
		if err := os.WriteFile(tmp, []byte(f.data), f.mode); err != nil {
			return "", fmt.Errorf("write %s: %w", f.name, err)
		}
		// Without root (a local install) the files keep their owner; Docker
		// Desktop maps it for the container.
		if err := os.Chown(tmp, engineUID, engineUID); err != nil && !errors.Is(err, fs.ErrPermission) {
			_ = os.Remove(tmp)
			return "", fmt.Errorf("chown %s: %w", f.name, err)
		}
		if err := os.Chmod(tmp, f.mode); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			return "", fmt.Errorf("install %s: %w", f.name, err)
		}
	}
	return dir, nil
}

// configureTLS turns TLS on for an existing instance, or renews its
// certificate: the container is recreated from the same image, volume and
// port with the new certificate mounted. The old container is kept (renamed
// and stopped) until the new one runs, and put back if it fails to start.
// A stopped instance stays stopped.
func configureTLS(ctx context.Context, eng string, spec *ProvisionSpec, sink LogSink) (*Result, error) {
	if spec.TLS == nil {
		return nil, fmt.Errorf("TLS certificate is missing")
	}
	out, err := runDocker(ctx, "inspect", "--format", "{{.Config.Image}}|{{.State.Running}}", spec.ContainerName)
	if err != nil {
		return nil, fmt.Errorf("container %s not found: %w", spec.ContainerName, err)
	}
	parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
	image, running := parts[0], len(parts) == 2 && parts[1] == "true"
	if image == "" {
		return nil, fmt.Errorf("could not read the image of %s", spec.ContainerName)
	}

	old := spec.ContainerName + "-pre-tls"
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", old).Run()
	sink.Log("info", "stopping "+spec.ContainerName+" to restart it with TLS")
	if running {
		if _, err := runDocker(ctx, "stop", spec.ContainerName); err != nil {
			return nil, err
		}
	}
	if _, err := runDocker(ctx, "rename", spec.ContainerName, old); err != nil {
		if running {
			_ = dockerSimple(ctx, "start", spec.ContainerName)
		}
		return nil, err
	}
	// The data volume isn't used again by the image's first-start setup, so
	// the root password only matters for a fresh volume.
	res, err := provisionContainer(ctx, eng, spec, image, sink)
	if err == nil {
		err = waitRunning(ctx, spec.ContainerName)
	}
	if err != nil {
		sink.Log("error", "the container did not start with TLS, putting the previous one back: "+err.Error())
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", spec.ContainerName).Run()
		if _, rerr := runDocker(ctx, "rename", old, spec.ContainerName); rerr == nil && running {
			_ = dockerSimple(ctx, "start", spec.ContainerName)
		}
		return nil, err
	}
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", old).Run()
	if !running {
		_ = dockerSimple(ctx, "stop", spec.ContainerName)
	}
	sink.Log("info", "TLS is on (certificate valid until "+spec.TLS.NotAfter+")")
	return res, nil
}

// waitRunning checks the new container is still running a few seconds after
// it started: a bad TLS setup makes the engine exit at once.
func waitRunning(ctx context.Context, container string) error {
	for i := 0; i < 5; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
		// --restart unless-stopped brings a crashing engine back up, so a
		// restart counts as a failure too.
		out, err := runDocker(ctx, "inspect", "--format", "{{.State.Running}}|{{.RestartCount}}", container)
		if err != nil {
			return err
		}
		if strings.TrimSpace(out) != "true|0" {
			logs, _ := exec.CommandContext(ctx, "docker", "logs", "--tail", "5", container).CombinedOutput()
			return fmt.Errorf("container exited: %s", strings.TrimSpace(lastLine(string(logs))))
		}
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// imageRef is the image reference to run for spec.
func imageRef(eng string, spec *ProvisionSpec) string {
	image := spec.Image
	if image == "" {
		image = eng // engine name doubles as the Docker Hub image (e.g. "mariadb")
	}
	tag := spec.Version
	if spec.Tag != "" {
		tag = spec.Tag
	}
	if tag != "" {
		image = image + ":" + tag
	}
	return image
}

// waitReady runs cmd in the container until it succeeds, the container
// stops, or readyTimeout passes.
func waitReady(ctx context.Context, container string, cmd []string) error {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	for {
		if _, err := runDocker(ctx, append([]string{"exec", container}, cmd...)...); err == nil {
			return nil
		}
		if out, err := runDocker(ctx, "inspect", "-f", "{{.State.Running}}", container); err == nil && strings.TrimSpace(out) == "false" {
			logs, _ := runDocker(ctx, "logs", "--tail", "5", container)
			return fmt.Errorf("the container stopped while starting: %s", firstLine(logs))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the server did not become ready within %s", readyTimeout)
		case <-time.After(time.Second):
		}
	}
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
