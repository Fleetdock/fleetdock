package executor

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// runRestoreSchema restores a backup of one PostgreSQL schema into a
// database (p.Database) as p.TargetSchema.
//
// Under its own name the backup is applied as it is: it drops and recreates
// the objects it holds, so with Replace it replaces the schema's current
// objects. Under another name it goes through a scratch database, like a
// schema copy: the target's structure is loaded there (a schema of the
// backup's name is moved out of the way), then the backup, which is renamed
// and dumped into the target. Foreign keys into other schemas are loaded NOT
// VALID in the scratch database and checked again in the target.
func runRestoreSchema(ctx context.Context, p *Payload, sink LogSink) (*Result, error) {
	if p.Engine != "postgres" {
		return nil, errors.New("only PostgreSQL schemas can be restored on their own")
	}
	if p.Schema == "" || p.TargetSchema == "" || p.Database == "" {
		return nil, errors.New("restore schema: the backup's schema and the target are required")
	}
	pg := &engine.Postgres{}

	existing, err := pg.SchemaCounts(ctx, p.Conn, p.Database)
	if err != nil {
		return nil, fmt.Errorf("list schemas of %s: %w", p.Database, err)
	}
	for _, s := range existing {
		if s.Name == p.TargetSchema && (!p.Replace || p.TargetSchema != p.Schema) {
			return nil, fmt.Errorf("schema %s already exists in %s", p.TargetSchema, p.Database)
		}
	}

	artifact, err := fetchArtifact(ctx, p, sink)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = artifact.Close()
		_ = os.Remove(artifact.Name())
	}()
	gz, err := gzip.NewReader(artifact)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	defer gz.Close()

	conn, closeTunnel, err := cliConn(ctx, p.Conn, sink)
	if err != nil {
		return nil, err
	}
	defer closeTunnel()

	if p.TargetSchema == p.Schema {
		sink.Log("info", fmt.Sprintf("restoring schema %s into %s", p.Schema, p.Database))
		if err := loadSQL(ctx, pg, conn, p.Database, gz, nil, sink); err != nil {
			return nil, err
		}
		return countAfter(ctx, pg, p, sink)
	}

	scratch, err := scratchName()
	if err != nil {
		return nil, err
	}
	sink.Log("info", "creating scratch database "+scratch)
	if err := pg.CreateDatabase(ctx, p.Conn, scratch, "", ""); err != nil {
		return nil, fmt.Errorf("create scratch database: %w", err)
	}
	defer func() {
		if err := pg.DropDatabase(context.WithoutCancel(ctx), p.Conn, scratch); err != nil {
			sink.Log("warn", "drop scratch database "+scratch+": "+err.Error())
		}
	}()

	sink.Log("info", "loading the structure of "+p.Database)
	if err := pipeDump(ctx, pg, conn, p.Database, engine.PartialDump{SchemaOnly: true}, scratch, nil, sink); err != nil {
		return nil, err
	}
	for _, s := range existing {
		if s.Name == p.Schema {
			hold, err := scratchName()
			if err != nil {
				return nil, err
			}
			if err := pg.AlterSchema(ctx, p.Conn, scratch, p.Schema, engine.SchemaChange{Name: &hold}); err != nil {
				return nil, fmt.Errorf("make room in the scratch database: %w", err)
			}
		}
	}

	res, err := pg.Query(ctx, p.Conn, scratch, "SELECT quote_ident('"+strings.ReplaceAll(p.Schema, "'", "''")+"')", 1, false)
	if err != nil || len(res.Rows) == 0 || res.Rows[0][0] == nil {
		return nil, fmt.Errorf("quote schema name: %w", err)
	}
	quoted := *res.Rows[0][0]
	// Foreign keys from the backup into other schemas: their rows are not in
	// the scratch database.
	loosened := map[string]bool{}
	outward := func(table, name, line string) string {
		if !strings.HasPrefix(table, quoted+".") || strings.Contains(line, " REFERENCES "+quoted+".") ||
			strings.HasSuffix(line, " NOT VALID;\n") {
			return line
		}
		loosened[name] = true
		return strings.TrimSuffix(line, ";\n") + " NOT VALID;\n"
	}
	sink.Log("info", "loading the backup of schema "+p.Schema)
	if err := loadSQL(ctx, pg, conn, scratch, gz, outward, sink); err != nil {
		return nil, err
	}

	sink.Log("info", fmt.Sprintf("renaming %s to %s", p.Schema, p.TargetSchema))
	name := p.TargetSchema
	if err := pg.AlterSchema(ctx, p.Conn, scratch, p.Schema, engine.SchemaChange{Name: &name}); err != nil {
		return nil, fmt.Errorf("rename in the scratch database: %w", err)
	}
	revalidate := func(_, name, line string) string {
		if cut, ok := strings.CutSuffix(line, " NOT VALID;\n"); ok && loosened[name] {
			return cut + ";\n"
		}
		return line
	}
	sink.Log("info", fmt.Sprintf("restoring schema %s into %s", p.TargetSchema, p.Database))
	if err := pipeDump(ctx, pg, conn, scratch, engine.PartialDump{Schema: p.TargetSchema}, p.Database, revalidate, sink); err != nil {
		return nil, err
	}
	sink.Log("warn", "function bodies and other text that name "+p.Schema+" were restored unchanged")
	return countAfter(ctx, pg, p, sink)
}

func countAfter(ctx context.Context, pg *engine.Postgres, p *Payload, sink LogSink) (*Result, error) {
	tables, err := pg.CountTables(ctx, p.Conn, p.Database)
	if err != nil {
		return nil, fmt.Errorf("restore verification failed: %w", err)
	}
	sink.Log("info", fmt.Sprintf("schema %s restored; %s now has %d tables", p.TargetSchema, p.Database, tables))
	return &Result{OK: true, TableCount: tables}, nil
}

// fetchArtifact downloads a backup artifact to a temp file and checks its
// checksum when one is known, before anything is changed.
func fetchArtifact(ctx context.Context, p *Payload, sink LogSink) (*os.File, error) {
	sink.Log("info", "downloading backup artifact")
	tmp, err := downloadToTemp(ctx, p.GetURL)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*os.File, error) {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	if p.Checksum != "" {
		sink.Log("info", "verifying checksum")
		sum, err := hashFile(tmp)
		if err != nil {
			return fail(err)
		}
		if sum != p.Checksum {
			return fail(fmt.Errorf("checksum mismatch: backup artifact is corrupt (expected %s, got %s)", p.Checksum, sum))
		}
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return tmp, nil
}
