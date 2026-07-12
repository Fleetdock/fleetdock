// Package executor runs operations against database instances. It is shared
// by the agent binary (managed instances) and the control-plane worker
// (external instances): both receive the same enriched Payload and produce
// the same Result.
package executor

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// Payload is the enriched, credential-bearing input for one operation.
type Payload struct {
	Engine     string            `json:"engine"`
	Conn       engine.ConnParams `json:"conn"`
	Database   string            `json:"database,omitempty"`
	Charset    string            `json:"charset,omitempty"`
	Collation  string            `json:"collation,omitempty"`
	PutURL     string            `json:"put_url,omitempty"`
	GetURL     string            `json:"get_url,omitempty"`
	BackupID   string            `json:"backup_id,omitempty"`
	StorageURL string            `json:"storage_url,omitempty"`
	Checksum   string            `json:"checksum,omitempty"` // expected sha256, verified on restore
	Provision  *ProvisionSpec    `json:"provision,omitempty"`
}

// ProvisionSpec describes a Docker container lifecycle action on a managed
// instance (the agent runs these; the control plane never receives them).
type ProvisionSpec struct {
	ContainerName string `json:"container_name"`
	Image         string `json:"image"`   // e.g. "mariadb"
	Version       string `json:"version"` // e.g. "11.4"
	Port          int    `json:"port"`
	RootPassword  string `json:"root_password,omitempty"` // provision only
	Volume        string `json:"volume"`
	RemoveVolume  bool   `json:"remove_volume,omitempty"` // remove only
}

// Result is the outcome data of one operation.
type Result struct {
	OK          bool                  `json:"ok"`
	Version     string                `json:"version,omitempty"`
	Databases   []engine.DatabaseInfo `json:"databases,omitempty"`
	SizeBytes   int64                 `json:"size_bytes,omitempty"`
	Checksum    string                `json:"checksum,omitempty"`
	StorageURL  string                `json:"storage_url,omitempty"`
	ContainerID string                `json:"container_id,omitempty"`
	TableCount  int                   `json:"table_count,omitempty"` // tables present after a restore
}

// Execute runs the operation named by jobType with the given payload. Progress
// and diagnostic lines are emitted to sink; pass NopSink{} to discard them.
func Execute(ctx context.Context, jobType string, p *Payload, sink LogSink) (json.RawMessage, error) {
	sink.Log("info", "starting "+jobType)

	// Container lifecycle operations run on the host via Docker and need no
	// database engine client.
	switch jobType {
	case "provision_instance", "start_instance", "stop_instance", "restart_instance", "remove_instance":
		res, err := runProvision(ctx, jobType, p, sink)
		if err != nil {
			return nil, err
		}
		return json.Marshal(res)
	}

	eng, err := engine.For(p.Engine)
	if err != nil {
		return nil, err
	}

	var res Result
	switch jobType {
	case "test_connection":
		sink.Log("info", "testing connection to "+p.Engine+" instance")
		version, err := eng.Ping(ctx, p.Conn)
		if err != nil {
			return nil, fmt.Errorf("connection failed: %w", err)
		}
		sink.Log("info", "connected: "+version)
		res = Result{OK: true, Version: version}

	case "create_database":
		sink.Log("info", "creating database "+p.Database)
		if err := eng.CreateDatabase(ctx, p.Conn, p.Database, p.Charset, p.Collation); err != nil {
			return nil, err
		}
		sink.Log("info", "database created")
		res = Result{OK: true}

	case "delete_database":
		sink.Log("info", "dropping database "+p.Database)
		if err := eng.DropDatabase(ctx, p.Conn, p.Database); err != nil {
			return nil, err
		}
		sink.Log("info", "database dropped")
		res = Result{OK: true}

	case "import_databases":
		sink.Log("info", "listing databases on the instance")
		dbs, err := eng.ListDatabases(ctx, p.Conn)
		if err != nil {
			return nil, err
		}
		sink.Log("info", fmt.Sprintf("found %d databases", len(dbs)))
		res = Result{OK: true, Databases: dbs}

	case "backup":
		out, err := runBackup(ctx, eng, p, sink)
		if err != nil {
			return nil, err
		}
		res = *out

	case "restore":
		out, err := runRestore(ctx, eng, p, sink)
		if err != nil {
			return nil, err
		}
		res = *out

	default:
		return nil, fmt.Errorf("executor: unsupported operation type %q", jobType)
	}
	return json.Marshal(res)
}

// runBackup dumps one database, gzips it into a temp file (hashing as it
// goes), then uploads it via the presigned PUT URL.
func runBackup(ctx context.Context, eng engine.Client, p *Payload, sink LogSink) (*Result, error) {
	binaries, args, env := eng.DumpArgs(p.Conn, p.Database)
	bin, err := lookPath(binaries)
	if err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp("", "dbm-backup-*.sql.gz")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	hasher := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(tmp, hasher))

	sink.Log("info", "dumping database "+p.Database)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = gz
	var stderr strings.Builder
	tee := newLineSink(sink, "stderr")
	cmd.Stderr = io.MultiWriter(&stderr, tee)
	if err := cmd.Run(); err != nil {
		tee.Close()
		return nil, fmt.Errorf("dump failed: %s: %w", firstLine(stderr.String()), err)
	}
	tee.Close()
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}

	size, err := tmp.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	sink.Log("info", fmt.Sprintf("compressed %d bytes (sha256 %s)", size, checksum))

	sink.Log("info", "uploading backup artifact to storage")
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.PutURL, tmp)
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("upload failed: %s: %s", resp.Status, firstLine(string(body)))
	}
	sink.Log("info", "upload complete")

	return &Result{
		OK:         true,
		SizeBytes:  size,
		Checksum:   checksum,
		StorageURL: p.StorageURL,
	}, nil
}

// runRestore downloads a dump via the presigned GET URL to a temp file,
// verifies its checksum (when known) before mutating anything, creates the
// target database, restores the SQL stream, then counts the resulting tables
// as a sanity check.
func runRestore(ctx context.Context, eng engine.Client, p *Payload, sink LogSink) (*Result, error) {
	sink.Log("info", "downloading backup artifact")
	tmp, err := downloadToTemp(ctx, p.GetURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	// Integrity check before touching the target database.
	if p.Checksum != "" {
		sink.Log("info", "verifying checksum")
		sum, err := hashFile(tmp)
		if err != nil {
			return nil, err
		}
		if sum != p.Checksum {
			return nil, fmt.Errorf("checksum mismatch: backup artifact is corrupt (expected %s, got %s)", p.Checksum, sum)
		}
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	sink.Log("info", "creating target database "+p.Database)
	if err := eng.CreateDatabase(ctx, p.Conn, p.Database, p.Charset, p.Collation); err != nil {
		return nil, fmt.Errorf("create target database: %w", err)
	}

	gz, err := gzip.NewReader(tmp)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	defer gz.Close()

	binaries, args, env := eng.RestoreArgs(p.Conn, p.Database)
	bin, err := lookPath(binaries)
	if err != nil {
		return nil, err
	}
	sink.Log("info", "restoring SQL stream")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = gz
	var stderr strings.Builder
	tee := newLineSink(sink, "stderr")
	cmd.Stderr = io.MultiWriter(&stderr, tee)
	if err := cmd.Run(); err != nil {
		tee.Close()
		return nil, fmt.Errorf("restore failed: %s: %w", firstLine(stderr.String()), err)
	}
	tee.Close()

	// Verify the restore produced a schema.
	tables, err := eng.CountTables(ctx, p.Conn, p.Database)
	if err != nil {
		return nil, fmt.Errorf("restore verification failed: %w", err)
	}
	sink.Log("info", fmt.Sprintf("restore verified: %d tables", tables))
	return &Result{OK: true, TableCount: tables}, nil
}

func downloadToTemp(ctx context.Context, getURL string) (*os.File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	tmp, err := os.CreateTemp("", "dbm-restore-*.sql.gz")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, fmt.Errorf("download failed: %w", err)
	}
	return tmp, nil
}

func hashFile(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func lookPath(candidates []string) (string, error) {
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("none of %v found in PATH (install the mariadb client tools)", candidates)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	// mariadb-dump echoes the password warning; drop it.
	if strings.Contains(s, "Using a password on the command line") {
		return "command failed"
	}
	return s
}
