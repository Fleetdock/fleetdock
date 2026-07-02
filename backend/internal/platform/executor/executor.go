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

	"github.com/mariadb-cp/db-manager/backend/internal/platform/engine"
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
}

// Result is the outcome data of one operation.
type Result struct {
	OK         bool                  `json:"ok"`
	Version    string                `json:"version,omitempty"`
	Databases  []engine.DatabaseInfo `json:"databases,omitempty"`
	SizeBytes  int64                 `json:"size_bytes,omitempty"`
	Checksum   string                `json:"checksum,omitempty"`
	StorageURL string                `json:"storage_url,omitempty"`
}

// Execute runs the operation named by jobType with the given payload.
func Execute(ctx context.Context, jobType string, p *Payload) (json.RawMessage, error) {
	eng, err := engine.For(p.Engine)
	if err != nil {
		return nil, err
	}

	var res Result
	switch jobType {
	case "test_connection":
		version, err := eng.Ping(ctx, p.Conn)
		if err != nil {
			return nil, fmt.Errorf("connection failed: %w", err)
		}
		res = Result{OK: true, Version: version}

	case "create_database":
		if err := eng.CreateDatabase(ctx, p.Conn, p.Database, p.Charset, p.Collation); err != nil {
			return nil, err
		}
		res = Result{OK: true}

	case "delete_database":
		if err := eng.DropDatabase(ctx, p.Conn, p.Database); err != nil {
			return nil, err
		}
		res = Result{OK: true}

	case "import_databases":
		dbs, err := eng.ListDatabases(ctx, p.Conn)
		if err != nil {
			return nil, err
		}
		res = Result{OK: true, Databases: dbs}

	case "backup":
		out, err := runBackup(ctx, eng, p)
		if err != nil {
			return nil, err
		}
		res = *out

	case "restore":
		if err := runRestore(ctx, eng, p); err != nil {
			return nil, err
		}
		res = Result{OK: true}

	default:
		return nil, fmt.Errorf("executor: unsupported operation type %q", jobType)
	}
	return json.Marshal(res)
}

// runBackup dumps one database, gzips it into a temp file (hashing as it
// goes), then uploads it via the presigned PUT URL.
func runBackup(ctx context.Context, eng engine.Client, p *Payload) (*Result, error) {
	binaries, args := eng.DumpArgs(p.Conn, p.Database)
	bin, err := lookPath(binaries)
	if err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp("", "dbm-backup-*.sql.gz")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	hasher := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(tmp, hasher))

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = gz
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("dump failed: %s: %w", firstLine(stderr.String()), err)
	}
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

	return &Result{
		OK:         true,
		SizeBytes:  size,
		Checksum:   hex.EncodeToString(hasher.Sum(nil)),
		StorageURL: p.StorageURL,
	}, nil
}

// runRestore downloads a dump via the presigned GET URL and pipes it into
// the engine's restore command, creating the target database first.
func runRestore(ctx context.Context, eng engine.Client, p *Payload) error {
	if err := eng.CreateDatabase(ctx, p.Conn, p.Database, p.Charset, p.Collation); err != nil {
		return fmt.Errorf("create target database: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.GetURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}
	defer gz.Close()

	binaries, args := eng.RestoreArgs(p.Conn, p.Database)
	bin, err := lookPath(binaries)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = gz
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restore failed: %s: %w", firstLine(stderr.String()), err)
	}
	return nil
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
