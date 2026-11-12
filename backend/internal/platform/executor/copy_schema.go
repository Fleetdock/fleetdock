package executor

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// runCopySchema copies a PostgreSQL schema to a new schema, in the same
// database or another one on the instance.
//
// A schema cannot be dumped under another name, so the copy goes through a
// scratch database: the source's structure (rows only from the copied
// schema) is loaded into it, so references into other schemas resolve; the
// schema is renamed there, which carries every reference held by OID (views,
// foreign keys, defaults, sequence ownership) to the new name; then only the
// renamed schema is dumped into the target. Text that names the old schema,
// such as function bodies, is copied unchanged.
func runCopySchema(ctx context.Context, p *Payload, sink LogSink) (*Result, error) {
	if p.Engine != "postgres" {
		return nil, errors.New("copying a schema needs PostgreSQL")
	}
	if p.Schema == "" || p.TargetSchema == "" || p.Database == "" || p.TargetDatabase == "" {
		return nil, errors.New("copy schema: source and target are required")
	}
	pg := &engine.Postgres{}

	schemas, err := pg.SchemaCounts(ctx, p.Conn, p.Database)
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	var others []string
	found := false
	for _, s := range schemas {
		if s.Name == p.Schema {
			found = true
		} else {
			others = append(others, s.Name)
		}
	}
	if !found {
		return nil, fmt.Errorf("schema %s does not exist in %s", p.Schema, p.Database)
	}

	targets, err := pg.SchemaCounts(ctx, p.Conn, p.TargetDatabase)
	if err != nil {
		return nil, fmt.Errorf("list schemas of %s: %w", p.TargetDatabase, err)
	}
	for _, s := range targets {
		if s.Name == p.TargetSchema {
			return nil, fmt.Errorf("schema %s already exists in %s", p.TargetSchema, p.TargetDatabase)
		}
	}

	conn, closeTunnel, err := cliConn(ctx, p.Conn, sink)
	if err != nil {
		return nil, err
	}
	defer closeTunnel()

	scratch, err := scratchName()
	if err != nil {
		return nil, err
	}
	sink.Log("info", "creating scratch database "+scratch)
	if err := pg.CreateDatabase(ctx, p.Conn, scratch, "", ""); err != nil {
		return nil, fmt.Errorf("create scratch database: %w", err)
	}
	defer func() {
		// The job's context may be done; the drop must still run.
		if err := pg.DropDatabase(context.WithoutCancel(ctx), p.Conn, scratch); err != nil {
			sink.Log("warn", "drop scratch database "+scratch+": "+err.Error())
		}
	}()

	what := "structure and rows"
	if p.StructureOnly {
		what = "structure"
	}
	sink.Log("info", fmt.Sprintf("loading the structure of %s and the %s of schema %s", p.Database, what, p.Schema))
	src, err := sourceNames(ctx, pg, p.Conn, p.Database, p.Schema)
	if err != nil {
		return nil, err
	}
	// The rows of other schemas stay behind, so a foreign key from the
	// copied schema into one of them cannot be checked here: it is loaded
	// NOT VALID, and checked again in the target.
	outward := func(table, _, line string) string {
		if !strings.HasPrefix(table, src.schema+".") || strings.Contains(line, " REFERENCES "+src.schema+".") ||
			strings.HasSuffix(line, " NOT VALID;\n") {
			return line
		}
		return strings.TrimSuffix(line, ";\n") + " NOT VALID;\n"
	}
	if err := pipeDump(ctx, pg, conn, p.Database, engine.PartialDump{SkipDataOf: others, SchemaOnly: p.StructureOnly}, scratch, outward, sink); err != nil {
		return nil, err
	}

	if p.TargetSchema != p.Schema {
		sink.Log("info", fmt.Sprintf("renaming %s to %s", p.Schema, p.TargetSchema))
		name := p.TargetSchema
		if err := pg.AlterSchema(ctx, p.Conn, scratch, p.Schema, engine.SchemaChange{Name: &name}); err != nil {
			return nil, fmt.Errorf("rename in the scratch database: %w", err)
		}
	}

	sink.Log("info", fmt.Sprintf("copying schema %s into %s", p.TargetSchema, p.TargetDatabase))
	// Foreign keys that were valid in the source are checked in the target.
	revalidate := func(_, name, line string) string {
		if src.notValid[name] {
			return line
		}
		if cut, ok := strings.CutSuffix(line, " NOT VALID;\n"); ok {
			return cut + ";\n"
		}
		return line
	}
	if err := pipeDump(ctx, pg, conn, scratch, engine.PartialDump{Schema: p.TargetSchema, SchemaOnly: p.StructureOnly}, p.TargetDatabase, revalidate, sink); err != nil {
		return nil, err
	}
	if p.TargetSchema != p.Schema {
		sink.Log("warn", "function bodies and other text that name "+p.Schema+" were copied unchanged")
	}

	tables, err := pg.CountTables(ctx, p.Conn, p.TargetDatabase)
	if err != nil {
		return nil, fmt.Errorf("copy verification failed: %w", err)
	}
	sink.Log("info", fmt.Sprintf("schema %s copied; %s now has %d tables", p.TargetSchema, p.TargetDatabase, tables))
	return &Result{OK: true, TableCount: tables}, nil
}

// sourceInfo is what the dump edits need to know about the copied schema:
// its name as pg_dump writes it, and its foreign keys that are NOT VALID.
type sourceInfo struct {
	schema   string
	notValid map[string]bool
}

func sourceNames(ctx context.Context, pg *engine.Postgres, conn engine.ConnParams, database, schema string) (*sourceInfo, error) {
	lit := "'" + strings.ReplaceAll(schema, "'", "''") + "'"
	res, err := pg.Query(ctx, conn, database, `SELECT quote_ident(`+lit+`), quote_ident(c.conname)
		FROM pg_namespace n LEFT JOIN pg_constraint c
		  ON c.connamespace = n.oid AND c.contype = 'f' AND NOT c.convalidated
		WHERE n.nspname = `+lit, 10000, false)
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", schema, err)
	}
	info := &sourceInfo{notValid: map[string]bool{}}
	for _, row := range res.Rows {
		if row[0] != nil {
			info.schema = *row[0]
		}
		if row[1] != nil {
			info.notValid[*row[1]] = true
		}
	}
	if info.schema == "" {
		return nil, fmt.Errorf("schema %s does not exist in %s", schema, database)
	}
	return info, nil
}

// editForeignKeys passes a plain pg_dump through, handing each line that
// adds a foreign key to edit with the table it is on (as pg_dump writes it)
// and the constraint's name. Rows of COPY blocks are never touched.
func editForeignKeys(r io.Reader, edit func(table, name, line string) string) *io.PipeReader {
	pr, pw := io.Pipe()
	go func() {
		br := bufio.NewReader(r)
		inCopy := false
		table := ""
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				switch {
				case inCopy:
					inCopy = line != "\\.\n"
				case strings.HasPrefix(line, "COPY ") && strings.HasSuffix(line, " FROM stdin;\n"):
					inCopy = true
				case strings.HasPrefix(line, "ALTER TABLE "):
					table = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(line, "ALTER TABLE "), "ONLY "), "\n")
				default:
					if name, ok := foreignKeyName(line); ok {
						line = edit(table, name, line)
					}
				}
				if _, werr := io.WriteString(pw, line); werr != nil {
					pw.CloseWithError(werr)
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				pw.CloseWithError(err)
				return
			}
		}
	}()
	return pr
}

// foreignKeyName reads the constraint name from a pg_dump line such as
// "    ADD CONSTRAINT orders_customer_fkey FOREIGN KEY (customer) REFERENCES …;".
func foreignKeyName(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, "    ADD CONSTRAINT ")
	if !ok || !strings.HasSuffix(line, ";\n") {
		return "", false
	}
	name, _, ok := strings.Cut(rest, " FOREIGN KEY (")
	return name, ok
}

// scratchName names a throwaway database the way backup copies do; database
// discovery ignores the fds_ prefix.
func scratchName() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "fds_" + hex.EncodeToString(b), nil
}

// pipeDump dumps part of from and loads it into to, in one transaction that
// stops at the first error. The dump goes to a temp file first: psql commits
// whatever it read once its input ends, so a pg_dump failing halfway must
// never reach it.
func pipeDump(ctx context.Context, pg *engine.Postgres, conn engine.ConnParams, from string, part engine.PartialDump, to string,
	edit func(table, name, line string) string, sink LogSink) error {
	dumpBins, dumpArgs, dumpEnv := pg.PartialDumpArgs(conn, from, part)
	dumpBin, err := lookPath(dumpBins)
	if err != nil {
		return err
	}
	restoreBins, restoreArgs, restoreEnv := pg.RestoreArgs(conn, to)
	restoreBin, err := lookPath(restoreBins)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "fleetdock-schema-*.sql")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	dump := exec.CommandContext(ctx, dumpBin, dumpArgs...)
	dump.Env = append(os.Environ(), dumpEnv...)
	dump.Stdout = tmp
	var dumpErr strings.Builder
	dumpTee := newLineSink(sink, "stderr")
	dump.Stderr = io.MultiWriter(&dumpErr, dumpTee)
	err = dump.Run()
	dumpTee.Close()
	if err != nil {
		return fmt.Errorf("dump of %s failed: %s: %w", from, firstLine(dumpErr.String()), err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}

	restore := exec.CommandContext(ctx, restoreBin, restoreArgs...)
	restore.Env = append(os.Environ(), restoreEnv...)
	edited := editForeignKeys(newPGDumpCompat(tmp), edit)
	defer edited.Close()
	restore.Stdin = edited
	// psql's output (SET, CREATE TABLE, COPY n) is noise here.
	restore.Stdout = io.Discard
	var restoreErr strings.Builder
	restoreTee := newLineSink(sink, "stderr")
	restore.Stderr = io.MultiWriter(&restoreErr, restoreTee)
	err = restore.Run()
	restoreTee.Close()
	if err != nil {
		return fmt.Errorf("load into %s failed: %s: %w", to, firstLine(restoreErr.String()), err)
	}
	return nil
}
