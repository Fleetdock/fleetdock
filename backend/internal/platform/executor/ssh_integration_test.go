//go:build integration

package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel/sshtunneltest"
)

// TestIntegrationBackupRestoreThroughSSHTunnel runs pg_dump and psql against
// PostgreSQL through an in-process bastion: the CLI tools reach it over the
// local port forward. Needs FLEETDOCK_IT_PG_TOOLS_POSTGRES (host:port of a
// server matching the local pg_dump major version).
func TestIntegrationBackupRestoreThroughSSHTunnel(t *testing.T) {
	addr := os.Getenv("FLEETDOCK_IT_PG_TOOLS_POSTGRES")
	if addr == "" {
		t.Skip("FLEETDOCK_IT_PG_TOOLS_POSTGRES not set")
	}
	dbHost, dbPort, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(dbPort)
	srv := sshtunneltest.Start(t, sshtunneltest.HostKey(t), "tunnel-pw", nil)
	sshHost, sshPort, _ := net.SplitHostPort(srv.Addr)
	sp, _ := strconv.Atoi(sshPort)
	conn := engine.ConnParams{Host: dbHost, Port: port, User: "postgres", Password: os.Getenv("FLEETDOCK_IT_PASSWORD"),
		TLSMode: engine.TLSDisable,
		SSH:     &sshtunnel.Config{Host: sshHost, Port: sp, User: "jump", Credentials: sshtunnel.Credentials{Password: "tunnel-pw"}}}

	var (
		mu       sync.Mutex
		artifact []byte
	)
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPut {
			artifact, _ = io.ReadAll(r.Body)
			return
		}
		_, _ = w.Write(artifact)
	}))
	defer store.Close()

	ctx := context.Background()
	pg, _ := engine.For("postgres")
	admin, _ := engine.AdminFor("postgres")
	src, dst := "ssh_src", "ssh_dst"
	_ = pg.DropDatabase(ctx, conn, src)
	_ = pg.DropDatabase(ctx, conn, dst)
	if err := pg.CreateDatabase(ctx, conn, src, "", ""); err != nil {
		t.Fatalf("create source: %v", err)
	}
	t.Cleanup(func() { _ = pg.DropDatabase(ctx, conn, src); _ = pg.DropDatabase(ctx, conn, dst) })
	for _, q := range []string{"CREATE TABLE t (id int)", "INSERT INTO t VALUES (1)"} {
		if _, err := admin.Query(ctx, conn, src, q, 10, true); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	raw, err := Execute(ctx, "backup", &Payload{Engine: "postgres", Conn: conn, Database: src, PutURL: store.URL}, NopSink{})
	if err != nil {
		t.Fatalf("backup through tunnel: %v", err)
	}
	var res Result
	_ = json.Unmarshal(raw, &res)
	if !res.OK || len(artifact) == 0 || !bytes.Equal(artifact[:2], []byte{0x1f, 0x8b}) {
		t.Fatalf("backup result = %+v, artifact %d bytes", res, len(artifact))
	}

	raw, err = Execute(ctx, "restore", &Payload{Engine: "postgres", Conn: conn, Database: dst, GetURL: store.URL, Checksum: res.Checksum}, NopSink{})
	if err != nil {
		t.Fatalf("restore through tunnel: %v", err)
	}
	_ = json.Unmarshal(raw, &res)
	if res.TableCount != 1 {
		t.Fatalf("restored tables = %d, want 1", res.TableCount)
	}
}
