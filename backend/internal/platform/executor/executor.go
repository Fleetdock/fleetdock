// Package executor runs operations against database instances. It is shared
// by the agent binary (managed instances) and the control-plane worker
// (external instances): both receive the same enriched Payload and produce
// the same Result.
package executor

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel"
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
	conn, closeTunnel, err := cliConn(ctx, p.Conn, sink)
	if err != nil {
		return nil, err
	}
	defer closeTunnel()
	binaries, args, env := eng.DumpArgs(conn, p.Database)
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

	var stream io.Reader = gz
	if p.Engine == "postgres" {
		stream = newPGDumpCompat(gz)
	}

	conn, closeTunnel, err := cliConn(ctx, p.Conn, sink)
	if err != nil {
		return nil, err
	}
	defer closeTunnel()
	binaries, args, env := eng.RestoreArgs(conn, p.Database)
	bin, err := lookPath(binaries)
	if err != nil {
		return nil, err
	}
	sink.Log("info", "restoring SQL stream")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = stream
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

// cliConn returns the parameters the dump/restore CLI tools should use. The
// tools dial the database themselves, so an SSH tunnel is exposed to them as
// a local 127.0.0.1 port forward that lives until the returned close runs.
func cliConn(ctx context.Context, c engine.ConnParams, sink LogSink) (engine.ConnParams, func(), error) {
	if c.SSH == nil {
		return c, func() {}, nil
	}
	sink.Log("info", fmt.Sprintf("opening SSH tunnel via %s@%s", c.SSH.User, c.SSH.Host))
	local, closeFn, err := sshtunnel.Forward(ctx, c.SSH, net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	if err != nil {
		return c, nil, err
	}
	host, ps, _ := net.SplitHostPort(local)
	port, _ := strconv.Atoi(ps)
	c.Host, c.Port, c.SSH = host, port, nil
	return c, closeFn, nil
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

// firstLine summarises a tool's stderr for an error message: the first line
// that reports an error, else the last non-warning line. Client tools print
// warnings (password on the command line, TLS verification) before the real
// error, and those must not stand in for it.
func firstLine(s string) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	for _, l := range lines {
		if strings.Contains(l, "ERROR") || strings.HasPrefix(strings.ToLower(l), "error") {
			return l
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if strings.HasPrefix(l, "WARNING") || strings.Contains(l, "Using a password on the command line") {
			continue
		}
		return l
	}
	return "command failed"
}

// pgDumpCompat drops the session settings newer pg_dump versions emit that
// older servers reject. pg_dump 17 writes "SET transaction_timeout = 0;",
// which a PostgreSQL 16 or older server refuses as an unknown parameter — and
// with ON_ERROR_STOP that would abort every restore into such a server. The
// setting only matters during the restore itself, so dropping it is safe.
type pgDumpCompat struct {
	r       *bufio.Reader
	pending []byte
}

func newPGDumpCompat(r io.Reader) io.Reader {
	return &pgDumpCompat{r: bufio.NewReaderSize(r, 64<<10)}
}

var pgDumpDropLines = [][]byte{
	[]byte("SET transaction_timeout = 0;"),
}

func (c *pgDumpCompat) Read(p []byte) (int, error) {
	for len(c.pending) == 0 {
		line, err := c.r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// A very long line (bulk COPY data): pass it through as is.
			c.pending = append(c.pending[:0], line...)
			break
		}
		if len(line) > 0 && !droppable(line) {
			c.pending = append(c.pending[:0], line...)
		}
		if err != nil {
			if len(c.pending) > 0 {
				break
			}
			return 0, err
		}
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func droppable(line []byte) bool {
	t := bytes.TrimSpace(line)
	for _, d := range pgDumpDropLines {
		if bytes.Equal(t, d) {
			return true
		}
	}
	return false
}
