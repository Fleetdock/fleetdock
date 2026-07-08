package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// runProvision performs a Docker container lifecycle action for a managed
// instance. It runs on the agent (which has Docker); the control-plane worker
// never receives these jobs.
func runProvision(ctx context.Context, jobType string, p *Payload) (*Result, error) {
	if p.Provision == nil {
		return nil, fmt.Errorf("provision spec is missing")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("docker is not installed on this server (required for provisioning)")
	}
	spec := p.Provision

	switch jobType {
	case "provision_instance":
		return provisionContainer(ctx, p.Engine, spec)
	case "start_instance":
		return &Result{OK: true}, dockerSimple(ctx, "start", spec.ContainerName)
	case "stop_instance":
		return &Result{OK: true}, dockerSimple(ctx, "stop", spec.ContainerName)
	case "restart_instance":
		return &Result{OK: true}, dockerSimple(ctx, "restart", spec.ContainerName)
	case "remove_instance":
		return removeContainer(ctx, spec)
	}
	return nil, fmt.Errorf("unsupported provision operation %q", jobType)
}

// engineContainer holds the Docker specifics of a database engine image.
type engineContainer struct {
	rootEnv      string // env var carrying the superuser password
	dataPath     string // container path to persist as a volume
	internalPort int    // port the engine listens on inside the container
}

func engineContainerFor(eng string) engineContainer {
	switch eng {
	case "postgres":
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
func provisionContainer(ctx context.Context, eng string, spec *ProvisionSpec) (*Result, error) {
	image := spec.Image
	if image == "" {
		image = eng // engine name doubles as the Docker Hub image (e.g. "mariadb")
	}
	if spec.Version != "" {
		image = image + ":" + spec.Version
	}
	ec := engineContainerFor(eng)

	// Best-effort cleanup of any stale container with the same name.
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", spec.ContainerName).Run()

	envFile, err := writeEnvFile(ec.rootEnv + "=" + spec.RootPassword)
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
		"--label", "app=db-manager",
		image,
	}
	out, err := runDocker(ctx, args...)
	if err != nil {
		return nil, err
	}
	containerID := strings.TrimSpace(out)
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}
	return &Result{OK: true, ContainerID: containerID}, nil
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

func writeEnvFile(line string) (string, error) {
	f, err := os.CreateTemp("", "dbm-env-*")
	if err != nil {
		return "", fmt.Errorf("create env file: %w", err)
	}
	if err := os.Chmod(f.Name(), 0o600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
