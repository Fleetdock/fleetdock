package engine

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ClickHouse dump format ("FDCH1"), written by Dump and read by Restore.
//
// ClickHouse ships no logical-dump tool the way MySQL and PostgreSQL do, so
// Fleetdock writes its own over the HTTP interface: each object's CREATE
// statement, and each table's rows in ClickHouse's Native format (columnar,
// typed, the server's own wire format).
//
// Layout: the magic line, then records. A record is one JSON header line
// followed by a payload in length-prefixed chunks (4-byte big-endian length,
// then that many bytes), ended by a zero length. The first record is the
// manifest and the last is "end", which carries the object count, so a
// truncated stream fails the restore. A table cut short is never swapped in,
// but tables restored before the cut stay restored (see Restore).
//
// Consistency: each table is read by one query, which sees one snapshot of
// that table; there is no snapshot across tables (ClickHouse has none).

const chDumpMagic = "FDCH1\n"

// chHiddenSecret is how the server prints a masked credential.
const chHiddenSecret = "'[HIDDEN]'"

// chDumpChunk bounds one payload chunk.
const chDumpChunk = 1 << 20

// chDumpRecord is a record header.
type chDumpRecord struct {
	Type string `json:"type"` // manifest | object | end

	// manifest
	Database      string `json:"database,omitempty"`
	ServerVersion string `json:"server_version,omitempty"`
	// ServerUUID identifies the server the backup was taken on, so a restore
	// can tell "back in place" from "onto another server" (empty in backups
	// from before it was recorded, and on servers without serverUUID()).
	ServerUUID string    `json:"server_uuid,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitzero"`

	// object
	Kind   string `json:"kind,omitempty"` // table | dictionary | view | materialized_view
	Name   string `json:"name,omitempty"`
	Engine string `json:"engine,omitempty"`
	Create string `json:"create,omitempty"`
	// Columns are the stored (insertable) columns the payload carries, in
	// payload order; empty when the object has no data in the dump.
	Columns []string `json:"columns,omitempty"`

	// end
	Objects int `json:"objects,omitempty"`
}

// chStreamingEngine reports engines that consume from an external stream
// (a Kafka consumer group, a queue, S3Queue's processed-files registry, a
// PostgreSQL replication slot). A second copy of such a table competes with
// the original for the same messages, so one is only recreated by a restore
// back into its own database on its own server.
func chStreamingEngine(engine string) bool {
	switch engine {
	case "Kafka", "RabbitMQ", "NATS", "S3Queue", "AzureQueue", "MaterializedPostgreSQL":
		return true
	}
	return false
}

// chDataEngines are the table engines whose rows live in ClickHouse and are
// dumped. Others (Kafka, URL, S3, MySQL, Distributed, Merge, Null, ...) keep
// their data elsewhere, or none, and are dumped as definitions only.
func chDataEngine(engine string) bool {
	switch engine {
	case "Log", "TinyLog", "StripeLog", "Memory":
		return true
	}
	return strings.HasSuffix(engine, "MergeTree")
}

// DumpExtension: the dump is not SQL, so it gets its own extension.
func (c *ClickHouse) DumpExtension() string { return ".fdch.gz" }

type chDumpObject struct {
	name, engine, create, uuid string
}

// Dump writes database's tables, dictionaries and views, and the rows of its
// data-holding tables and materialized views, to w.
func (c *ClickHouse) Dump(ctx context.Context, p ConnParams, database string, w io.Writer, log func(string)) error {
	if !validDBName(database) {
		return fmt.Errorf("invalid database name")
	}
	version, err := c.Ping(ctx, p)
	if err != nil {
		return err
	}
	list := chRequest{
		query: `
			SELECT name, engine, create_table_query, toString(uuid)
			FROM system.tables
			WHERE database = {db:String} AND NOT is_temporary
			ORDER BY name`,
		params: map[string]string{"db": database},
		// Definitions hold credentials (dictionary sources, MySQL/S3/Kafka
		// tables) that the server masks unless it is configured to show them
		// (display_secrets_in_show_and_select); ask for them, and fall back
		// on servers that do not know the setting.
		settings: map[string]string{"format_display_secrets_in_show_and_select": "1"},
	}
	rows, err := c.rows(ctx, p, list)
	if err != nil {
		list.settings = nil
		if rows, err = c.rows(ctx, p, list); err != nil {
			return err
		}
	}
	var objs []chDumpObject
	inner := map[string]bool{}
	for _, r := range rows {
		o := chDumpObject{name: str(r[0]), engine: str(r[1]), create: str(r[2]), uuid: str(r[3])}
		if strings.HasPrefix(o.name, ".inner") {
			// A materialized view's own storage: dumped through the view.
			inner[o.name] = true
			continue
		}
		objs = append(objs, o)
	}

	bw := bufio.NewWriterSize(w, 64<<10)
	if _, err := bw.WriteString(chDumpMagic); err != nil {
		return err
	}
	man := chDumpRecord{Type: "manifest", Database: database, ServerVersion: version, ServerUUID: c.serverUUID(ctx, p), CreatedAt: time.Now().UTC()}
	if err := writeCHRecord(bw, man, nil); err != nil {
		return err
	}

	// Tables first, then dictionaries, then views: the order Restore
	// creates them in.
	rank := func(o chDumpObject) int {
		switch o.engine {
		case "Dictionary":
			return 1
		case "View", "LiveView", "WindowView", "MaterializedView":
			return 2
		}
		return 0
	}
	var ordered []chDumpObject
	for pass := 0; pass < 3; pass++ {
		for _, o := range objs {
			if rank(o) == pass {
				ordered = append(ordered, o)
			}
		}
	}

	for _, o := range ordered {
		if strings.Contains(o.create, chHiddenSecret) {
			log(fmt.Sprintf("warning: %s: the server hides the credentials in its definition, so the backup cannot "+
				"include them; after a restore, recreate it with them (or enable display_secrets_in_show_and_select "+
				"on the server and grant displaySecretsInShowAndSelect to this login)", o.name))
		}
		rec := chDumpRecord{Type: "object", Name: o.name, Engine: o.engine, Create: o.create}
		withData := false
		switch o.engine {
		case "Dictionary":
			rec.Kind = "dictionary"
		case "View", "LiveView", "WindowView":
			rec.Kind = "view"
		case "MaterializedView":
			rec.Kind = "materialized_view"
			// Only a view that stores its own rows has data to dump; one
			// with TO writes into a table dumped on its own.
			withData = inner[".inner_id."+o.uuid] || inner[".inner."+o.name]
		default:
			rec.Kind = "table"
			withData = chDataEngine(o.engine)
		}
		if !withData {
			if err := writeCHRecord(bw, rec, nil); err != nil {
				return err
			}
			log(fmt.Sprintf("%s %s: definition", rec.Kind, o.name))
			continue
		}
		cols, err := c.storedColumns(ctx, p, database, o.name)
		if err != nil {
			return fmt.Errorf("%s: %w", o.name, err)
		}
		rec.Columns = cols
		n, err := c.dumpData(ctx, p, database, o.name, cols, bw, rec)
		if err != nil {
			return fmt.Errorf("%s: %w", o.name, err)
		}
		log(fmt.Sprintf("%s %s: %s of data", rec.Kind, o.name, humanBytes(n)))
	}
	if err := writeCHRecord(bw, chDumpRecord{Type: "end", Objects: len(ordered)}, nil); err != nil {
		return err
	}
	return bw.Flush()
}

// storedColumns lists the columns an INSERT accepts, in table order: not
// MATERIALIZED, ALIAS or EPHEMERAL ones, which the server computes.
func (c *ClickHouse) storedColumns(ctx context.Context, p ConnParams, database, table string) ([]string, error) {
	rows, err := c.rows(ctx, p, chRequest{
		query: `
			SELECT name FROM system.columns
			WHERE database = {db:String} AND table = {t:String}
			  AND default_kind NOT IN ('MATERIALIZED', 'ALIAS', 'EPHEMERAL')
			ORDER BY position`,
		params: map[string]string{"db": database, "t": table},
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, str(r[0]))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no insertable columns")
	}
	return out, nil
}

func chColumnList(cols []string) string {
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = quoteCHIdent(c)
	}
	return strings.Join(q, ", ")
}

// dumpData streams one table's rows in Native format into a record.
func (c *ClickHouse) dumpData(ctx context.Context, p ConnParams, database, table string, cols []string, w *bufio.Writer, rec chDumpRecord) (int64, error) {
	resp, err := c.open(ctx, p, chRequest{
		query:    fmt.Sprintf("SELECT %s FROM %s.%s FORMAT Native", chColumnList(cols), quoteCHIdent(database), quoteCHIdent(table)),
		database: database, readOnly: true,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Close()
	if err := writeCHHeader(w, rec); err != nil {
		return 0, err
	}
	// A query that fails mid-stream makes the server write "__exception__"
	// and the error, then abort the chunked response: the copy fails, and
	// the tail holds the server's message.
	tail := &tailBuffer{max: 8 << 10}
	cw := &chChunkWriter{w: w}
	n, err := io.Copy(io.MultiWriter(cw, tail), resp.r)
	if err != nil {
		if i := strings.LastIndex(tail.String(), "__exception__"); i >= 0 {
			if e := parseCHError(tail.String()[i:]); e != nil {
				return n, e
			}
		}
		return n, fmt.Errorf("reading rows: %w", err)
	}
	// Older servers report a failure that happens mid-stream by appending
	// the exception text and ending the response normally; without this the
	// broken dump would be stored as a good backup.
	if e := chTrailingException(tail.String()); e != nil {
		return n, fmt.Errorf("the server failed while sending rows: %w", e)
	}
	return n, cw.Close()
}

// chTrailingExceptionRe matches an exception message at the very end of a
// response, in the current ("Code: 241. DB::Exception: ...") and the older
// ("Code: 241, e.displayText() = DB::Exception: ...") spelling.
var chTrailingExceptionRe = regexp.MustCompile(`Code: (\d+)[.,] (?:e\.displayText\(\) = )?DB::Exception: ([^\n]*)\n?$`)

// chTrailingException returns the exception a response ends with, or nil.
func chTrailingException(tail string) *ClickHouseError {
	m := chTrailingExceptionRe.FindStringSubmatch(tail)
	if m == nil {
		return nil
	}
	code, _ := strconv.Atoi(m[1])
	return &ClickHouseError{Code: code, Message: strings.TrimSpace(m[2])}
}

// serverUUID returns the server's UUID, or "" when it cannot be read.
func (c *ClickHouse) serverUUID(ctx context.Context, p ConnParams) string {
	rows, err := c.rows(ctx, p, chRequest{query: "SELECT toString(serverUUID())"})
	if err != nil || len(rows) == 0 || len(rows[0]) == 0 {
		return ""
	}
	if u := str(rows[0][0]); u != "00000000-0000-0000-0000-000000000000" {
		return u
	}
	return ""
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(t.buf) }

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func writeCHHeader(w *bufio.Writer, rec chDumpRecord) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// writeCHRecord writes a record with an in-memory payload (nil = none).
func writeCHRecord(w *bufio.Writer, rec chDumpRecord, payload []byte) error {
	if err := writeCHHeader(w, rec); err != nil {
		return err
	}
	cw := &chChunkWriter{w: w}
	if len(payload) > 0 {
		if _, err := cw.Write(payload); err != nil {
			return err
		}
	}
	return cw.Close()
}

// chChunkWriter frames what is written to it as length-prefixed chunks.
type chChunkWriter struct {
	w *bufio.Writer
}

func (c *chChunkWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n := min(len(p), chDumpChunk)
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(n))
		if _, err := c.w.Write(hdr[:]); err != nil {
			return written, err
		}
		if _, err := c.w.Write(p[:n]); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}

// Close writes the terminating zero-length chunk.
func (c *chChunkWriter) Close() error {
	_, err := c.w.Write([]byte{0, 0, 0, 0})
	return err
}

// chChunkReader reads one record's payload until its terminating chunk.
type chChunkReader struct {
	r    *bufio.Reader
	left uint32
	done bool
}

func (c *chChunkReader) Read(p []byte) (int, error) {
	if c.done {
		return 0, io.EOF
	}
	if c.left == 0 {
		var hdr [4]byte
		if _, err := io.ReadFull(c.r, hdr[:]); err != nil {
			return 0, errTruncatedDump(err)
		}
		c.left = binary.BigEndian.Uint32(hdr[:])
		if c.left == 0 {
			c.done = true
			return 0, io.EOF
		}
		if c.left > chDumpChunk {
			return 0, fmt.Errorf("corrupt dump: chunk of %d bytes", c.left)
		}
	}
	if uint32(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= uint32(n)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, err
	}
	if n == 0 && errors.Is(err, io.EOF) {
		return 0, errTruncatedDump(err)
	}
	return n, nil
}

// drain skips the rest of the payload.
func (c *chChunkReader) drain() error {
	_, err := io.Copy(io.Discard, c)
	return err
}

func errTruncatedDump(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("the backup is truncated")
	}
	return err
}

// chPending is an object created after the tables, whose data (for a
// materialized view) was spooled to a temporary file.
type chPending struct {
	rec  chDumpRecord
	data *os.File
}

// Restore loads a dump into database. Objects the dump contains replace
// same-named ones already there (as a MySQL dump's DROP TABLE IF EXISTS
// does); other objects are left alone.
//
// A table is loaded into a temporary table and swapped in once all its rows
// are there, so a failed or truncated load leaves the original untouched and
// no materialized view (in this database or another) sees the inserts: they
// would otherwise add the restored rows to its aggregates a second time.
// Each table is swapped in as soon as it is loaded; the dump's trailer is
// only checked at the end, so a dump truncated between tables leaves the
// tables before the cut restored.
//
// Restoring anywhere but back into the same database on the same server:
//   - Replicated*MergeTree tables become their plain MergeTree form, so the
//     copy never joins the original's replication;
//   - streaming tables (Kafka, RabbitMQ, NATS, S3Queue, ...) and the views
//     reading from them are left out, so the copy never consumes the
//     original's messages.
//
// Restoring under the same name, a Replicated table is left as it is: put
// back on the server it came from, it would rejoin its replicas and
// replicate the restored rows to all of them on top of their own.
func (c *ClickHouse) Restore(ctx context.Context, p ConnParams, database string, r io.Reader, log func(string)) (err error) {
	if !validDBName(database) {
		return fmt.Errorf("invalid database name")
	}
	br := bufio.NewReaderSize(r, 64<<10)
	magic := make([]byte, len(chDumpMagic))
	if _, err := io.ReadFull(br, magic); err != nil || string(magic) != chDumpMagic {
		return fmt.Errorf("this is not a Fleetdock ClickHouse backup")
	}
	man, err := readCHHeader(br)
	if err != nil {
		return err
	}
	if man.Type != "manifest" {
		return fmt.Errorf("corrupt dump: missing manifest")
	}
	if err := (&chChunkReader{r: br}).drain(); err != nil {
		return err
	}
	source := man.Database
	renamed := source != database
	targetUUID := c.serverUUID(ctx, p)
	knownServers := man.ServerUUID != "" && targetUUID != ""
	sameServer := knownServers && man.ServerUUID == targetUUID
	inPlace := !renamed && sameServer
	// A copy elsewhere must not join the original's replication. Under the
	// same name on an unknown server the table is left alone instead.
	deReplicate := renamed || (knownServers && !sameServer)
	log(fmt.Sprintf("backup of %s taken %s on ClickHouse %s", source, man.CreatedAt.Format(time.RFC3339), man.ServerVersion))
	if !inPlace {
		log("restoring as a copy: streaming tables (Kafka, RabbitMQ, NATS, S3Queue, ...) and views reading from them are left out, " +
			"so the copy cannot consume the original's messages")
	}

	var pending []*chPending
	defer func() {
		for _, pd := range pending {
			if pd.data != nil {
				_ = pd.data.Close()
				_ = os.Remove(pd.data.Name())
			}
		}
	}()

	var skipped []string // tables left out; objects reading from them are too
	objects := 0
	for {
		rec, err := readCHHeader(br)
		if err != nil {
			return err
		}
		payload := &chChunkReader{r: br}
		if rec.Type == "end" {
			if err := payload.drain(); err != nil {
				return err
			}
			if rec.Objects != objects {
				return fmt.Errorf("corrupt dump: %d objects read, %d expected", objects, rec.Objects)
			}
			break
		}
		if rec.Type != "object" || !validTableName(rec.Name) {
			return fmt.Errorf("corrupt dump: unexpected record %q", rec.Type)
		}
		objects++

		if reason := chSkipReason(rec, inPlace, deReplicate, skipped); reason != "" {
			if err := payload.drain(); err != nil {
				return err
			}
			if rec.Kind == "table" {
				skipped = append(skipped, rec.Name)
			}
			log(fmt.Sprintf("%s %s: not restored (%s)", strings.ReplaceAll(rec.Kind, "_", " "), rec.Name, reason))
			continue
		}

		create, notes := chRestoreDDL(rec.Create, source, database, renamed, deReplicate)
		if strings.Contains(create, chHiddenSecret) {
			notes = append(notes, "warning: restored without its credentials (the server hid them when the backup was taken); recreate it with them")
		}
		for _, n := range notes {
			log(rec.Name + ": " + n)
		}
		rec.Create = create

		if rec.Kind != "table" {
			// Created once every table exists; a materialized view's rows
			// wait in a temporary file until then.
			pd := &chPending{rec: rec}
			pending = append(pending, pd)
			if len(rec.Columns) > 0 {
				f, err := os.CreateTemp("", "fleetdock-ch-restore-*")
				if err != nil {
					return err
				}
				pd.data = f
				if _, err := io.Copy(f, payload); err != nil {
					return err
				}
			} else if err := payload.drain(); err != nil {
				return err
			}
			continue
		}

		if len(rec.Columns) == 0 {
			if err := payload.drain(); err != nil {
				return err
			}
			if err := c.replaceObject(ctx, p, database, rec); err != nil {
				return fmt.Errorf("create table %s: %w", rec.Name, err)
			}
			log(fmt.Sprintf("table %s: definition (%s keeps no data here)", rec.Name, rec.Engine))
			continue
		}
		n, err := c.restoreTable(ctx, p, database, rec, payload)
		if err != nil {
			_ = payload.drain()
			return fmt.Errorf("table %s: %w", rec.Name, err)
		}
		log(fmt.Sprintf("table %s: %d rows", rec.Name, n))
	}

	// Views reading from a table that was left out cannot be created (and
	// must not be: a view on a Kafka table consumes when queried).
	var create []*chPending
	for _, pd := range pending {
		if t := chReadsFrom(pd.rec.Create, skipped); t != "" {
			log(fmt.Sprintf("%s %s: not restored (it reads from %s, which was left out)", strings.ReplaceAll(pd.rec.Kind, "_", " "), pd.rec.Name, t))
			continue
		}
		create = append(create, pd)
	}

	// Dictionaries and views may depend on each other in any order: create
	// what can be created until a pass makes no progress.
	left := create
	for len(left) > 0 {
		var retry []*chPending
		var lastErr error
		for _, pd := range left {
			if err := c.replaceObject(ctx, p, database, pd.rec); err != nil {
				lastErr = fmt.Errorf("create %s %s: %w", strings.ReplaceAll(pd.rec.Kind, "_", " "), pd.rec.Name, err)
				retry = append(retry, pd)
				continue
			}
			log(fmt.Sprintf("%s %s: created", pd.rec.Kind, pd.rec.Name))
		}
		if len(retry) == len(left) {
			return lastErr
		}
		left = retry
	}
	for _, pd := range create {
		if pd.data == nil {
			continue
		}
		if _, err := pd.data.Seek(0, io.SeekStart); err != nil {
			return err
		}
		// Into the view's own storage, not through the view: inserting into
		// the view would also fire views built on it, which get their own
		// rows from the dump.
		storage, err := c.innerTable(ctx, p, database, pd.rec.Name)
		if err != nil {
			return fmt.Errorf("load %s: %w", pd.rec.Name, err)
		}
		n, err := c.insertNative(ctx, p, database, storage, pd.rec.Columns, bufio.NewReader(pd.data))
		if err != nil {
			return fmt.Errorf("load %s: %w", pd.rec.Name, err)
		}
		log(fmt.Sprintf("materialized view %s: %d rows", pd.rec.Name, n))
	}
	return nil
}

// chSkipReason says why an object is left out of a restore, or "".
func chSkipReason(rec chDumpRecord, inPlace, deReplicate bool, skipped []string) string {
	if rec.Kind != "table" {
		return "" // decided once the tables are done (chReadsFrom)
	}
	switch {
	case chStreamingEngine(rec.Engine) && !inPlace:
		return "a " + rec.Engine + " table consumes from its source; a copy would take the original's messages"
	case strings.HasPrefix(rec.Engine, "Replicated") && !deReplicate:
		return "a replicated table restored under its own name would rejoin its replicas and replicate the restored rows to all of them; " +
			"restore under another name and copy the rows over"
	}
	return ""
}

// chReadsFrom returns the first of tables that a definition mentions, or "".
func chReadsFrom(create string, tables []string) string {
	for _, t := range tables {
		re := regexp.MustCompile("(?:^|[^\\w])[`\"]?" + regexp.QuoteMeta(t) + "[`\"]?(?:[^\\w]|$)")
		if re.MatchString(create) {
			return t
		}
	}
	return ""
}

// restoreTable loads a table's rows into a temporary table and swaps it in
// for the existing one (or renames it into place).
func (c *ClickHouse) restoreTable(ctx context.Context, p ConnParams, database string, rec chDumpRecord, data io.Reader) (int64, error) {
	tmp := chRestoreTempName(rec.Name)
	tmpCreate, err := chRenameCreate(rec.Create, tmp)
	if err != nil {
		return 0, err
	}
	qdb := quoteCHIdent(database)
	qt, qtmp := qdb+"."+quoteCHIdent(rec.Name), qdb+"."+quoteCHIdent(tmp)
	drop := func(name string) error {
		return c.exec(ctx, p, chRequest{
			query: "DROP TABLE IF EXISTS " + name + " SYNC", database: database,
			settings: map[string]string{"check_table_dependencies": "0", "check_referential_table_dependencies": "0"},
		})
	}
	if err := drop(qtmp); err != nil {
		return 0, err
	}
	if err := c.exec(ctx, p, chRequest{query: tmpCreate, database: database}); err != nil {
		return 0, fmt.Errorf("create: %w", err)
	}
	n, err := c.insertNative(ctx, p, database, tmp, rec.Columns, data)
	if err != nil {
		_ = drop(qtmp)
		return 0, fmt.Errorf("load: %w", err)
	}
	rows, err := c.rows(ctx, p, chRequest{
		query:  "SELECT count() FROM system.tables WHERE database = {db:String} AND name = {t:String}",
		params: map[string]string{"db": database, "t": rec.Name},
	})
	if err != nil {
		_ = drop(qtmp)
		return 0, err
	}
	if i64(rows[0][0]) == 0 {
		err = c.exec(ctx, p, chRequest{query: "RENAME TABLE " + qtmp + " TO " + qt, database: database})
	} else if err = c.exec(ctx, p, chRequest{query: "EXCHANGE TABLES " + qtmp + " AND " + qt, database: database}); err == nil {
		err = drop(qtmp) // now the old table
	} else {
		// EXCHANGE needs an Atomic database; an Ordinary one gets the
		// same through two renames.
		old := qdb + "." + quoteCHIdent(chRestoreTempName(rec.Name+"_old"))
		if err = c.exec(ctx, p, chRequest{query: "RENAME TABLE " + qt + " TO " + old + ", " + qtmp + " TO " + qt, database: database}); err == nil {
			err = drop(old)
		}
	}
	if err != nil {
		_ = drop(qtmp)
		return 0, fmt.Errorf("swap in: %w", err)
	}
	return n, nil
}

// chRestoreTempName is the temporary table a table's rows are loaded into.
func chRestoreTempName(name string) string { return ".fd_restore." + name }

// chRenameCreate rewrites the object name of a "CREATE TABLE db.name ..."
// statement (the form system.tables reports), keeping its database.
func chRenameCreate(create, name string) (string, error) {
	const prefix = "CREATE TABLE "
	if !strings.HasPrefix(create, prefix) {
		return "", fmt.Errorf("unexpected definition: %.40q", create)
	}
	i := len(prefix)
	skipIdent := func(i int) int {
		if i < len(create) && (create[i] == '`' || create[i] == '"') {
			q := create[i]
			for j := i + 1; j < len(create); j++ {
				switch create[j] {
				case '\\':
					j++
				case q:
					return j + 1
				}
			}
			return -1
		}
		j := i
		for j < len(create) && (create[j] == '_' || create[j] == '$' || create[j] >= '0' && create[j] <= '9' ||
			create[j] >= 'a' && create[j] <= 'z' || create[j] >= 'A' && create[j] <= 'Z') {
			j++
		}
		if j == i {
			return -1
		}
		return j
	}
	dbEnd := skipIdent(i)
	if dbEnd < 0 || dbEnd >= len(create) || create[dbEnd] != '.' {
		return "", fmt.Errorf("unexpected definition: %.40q", create)
	}
	nameEnd := skipIdent(dbEnd + 1)
	if nameEnd < 0 {
		return "", fmt.Errorf("unexpected definition: %.40q", create)
	}
	return create[:dbEnd+1] + quoteCHIdent(name) + create[nameEnd:], nil
}

// innerTable returns the table a materialized view stores its rows in: its
// .inner_id.<uuid> table (Atomic databases) or .inner.<name> (Ordinary).
func (c *ClickHouse) innerTable(ctx context.Context, p ConnParams, database, view string) (string, error) {
	rows, err := c.rows(ctx, p, chRequest{
		query: `
			SELECT name FROM system.tables
			WHERE database = {db:String}
			  AND name IN (concat('.inner_id.', (SELECT toString(uuid) FROM system.tables WHERE database = {db:String} AND name = {v:String})),
			               concat('.inner.', {v:String}))`,
		params: map[string]string{"db": database, "v": view},
	})
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("its storage table was not found")
	}
	return str(rows[0][0]), nil
}

func readCHHeader(br *bufio.Reader) (chDumpRecord, error) {
	line, err := br.ReadBytes('\n')
	if err != nil {
		return chDumpRecord{}, errTruncatedDump(err)
	}
	var rec chDumpRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return chDumpRecord{}, fmt.Errorf("corrupt dump: %w", err)
	}
	return rec, nil
}

// replaceObject drops a same-named object, then runs the object's CREATE.
func (c *ClickHouse) replaceObject(ctx context.Context, p ConnParams, database string, rec chDumpRecord) error {
	drop := "DROP TABLE"
	if rec.Kind == "dictionary" {
		drop = "DROP DICTIONARY"
	}
	if err := c.exec(ctx, p, chRequest{
		query:    fmt.Sprintf("%s IF EXISTS %s.%s SYNC", drop, quoteCHIdent(database), quoteCHIdent(rec.Name)),
		database: database,
		// A view on the table must not block replacing it.
		settings: map[string]string{"check_table_dependencies": "0", "check_referential_table_dependencies": "0"},
	}); err != nil {
		return err
	}
	return c.exec(ctx, p, chRequest{query: rec.Create, database: database})
}

// insertNative loads a Native payload into a table's stored columns.
func (c *ClickHouse) insertNative(ctx context.Context, p ConnParams, database, table string, cols []string, data io.Reader) (int64, error) {
	resp, err := c.open(ctx, p, chRequest{
		query:    fmt.Sprintf("INSERT INTO %s.%s (%s) FORMAT Native", quoteCHIdent(database), quoteCHIdent(table), chColumnList(cols)),
		database: database,
		body:     data,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Close()
	return resp.writtenRows(), nil
}

// chReplicatedRe matches a Replicated*MergeTree engine clause with its
// arguments (the ZooKeeper path and replica name come first).
var chReplicatedRe = regexp.MustCompile(`ENGINE = Replicated(\w*MergeTree)(\((?:[^()']|'(?:[^'\\]|\\.)*')*\))?`)

// chRestoreDDL adapts a CREATE statement to the target database: it names
// the target database when renamed, and drops replication when deReplicate.
func chRestoreDDL(create, source, target string, renamed, deReplicate bool) (string, []string) {
	var notes []string
	out := create
	if renamed {
		out = rewriteCHEngineDB(rewriteCHDatabase(out, source, target), source, target)
	}
	if !deReplicate {
		return out, notes
	}
	out = chReplicatedRe.ReplaceAllStringFunc(out, func(m string) string {
		sub := chReplicatedRe.FindStringSubmatch(m)
		args := splitCHArgs(strings.TrimSuffix(strings.TrimPrefix(sub[2], "("), ")"))
		// The first two arguments, when given as literals, are the path and
		// replica; the rest belong to the MergeTree variant.
		rest := args
		if len(args) >= 2 && strings.HasPrefix(args[0], "'") && strings.HasPrefix(args[1], "'") {
			rest = args[2:]
		}
		notes = append(notes, "restored as non-replicated "+sub[1]+" (a copy must not join the original's replication)")
		return "ENGINE = " + sub[1] + "(" + strings.Join(rest, ", ") + ")"
	})
	return out, notes
}

// splitCHArgs splits engine arguments at top-level commas.
func splitCHArgs(s string) []string {
	var out []string
	depth, start := 0, 0
	inStr := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case inStr:
			switch ch {
			case '\\':
				i++
			case '\'':
				inStr = false
			}
		case ch == '\'':
			inStr = true
		case ch == '(':
			depth++
		case ch == ')':
			depth--
		case ch == ',' && depth == 0:
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		out = append(out, t)
	}
	return out
}

// chInDictCall reports whether the literal at i is the first argument of a
// dictionary function (dictGet, dictGetOrDefault, dictHas, ...).
func chInDictCall(sql string, i int) bool {
	j := i
	for j > 0 && sql[j-1] == ' ' {
		j--
	}
	if j == 0 || sql[j-1] != '(' {
		return false
	}
	j--
	k := j
	for k > 0 && (sql[k-1] == '_' || sql[k-1] >= 'a' && sql[k-1] <= 'z' || sql[k-1] >= 'A' && sql[k-1] <= 'Z') {
		k--
	}
	return strings.HasPrefix(strings.ToLower(sql[k:j]), "dict")
}

// chEngineDBArg is the position of the database argument of engines that
// name another table's database.
var chEngineDBArg = map[string]int{"Buffer": 0, "Merge": 0, "Distributed": 1}

// chEngineRe finds those engines' argument lists.
var chEngineRe = regexp.MustCompile(`ENGINE = (Buffer|Merge|Distributed)\(`)

// rewriteCHEngineDB points a Buffer, Merge or Distributed engine at target
// when its database argument names source ('src', `src` or src).
func rewriteCHEngineDB(sql, source, target string) string {
	m := chEngineRe.FindStringSubmatchIndex(sql)
	if m == nil {
		return sql
	}
	open := m[1] - 1
	end := chMatchingParen(sql, open)
	if end < 0 {
		return sql
	}
	args := splitCHArgs(sql[open+1 : end])
	pos := chEngineDBArg[sql[m[2]:m[3]]]
	if pos >= len(args) {
		return sql
	}
	switch args[pos] {
	case quoteCHString(source):
		args[pos] = quoteCHString(target)
	case source, quoteCHIdent(source):
		args[pos] = quoteCHIdent(target)
	default:
		return sql
	}
	return sql[:open+1] + strings.Join(args, ", ") + sql[end:]
}

// chMatchingParen returns the index of the parenthesis closing the one at
// open, skipping string literals, or -1.
func chMatchingParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '\'':
			for i++; i < len(s) && s[i] != '\''; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// rewriteCHDatabase replaces the database qualifier source. with target. in
// a statement: plain, `quoted` or "quoted" identifiers immediately followed
// by a dot. String literals and comments are left alone, except the
// dictionary source clause DB 'source', which names the database as a
// string.
func rewriteCHDatabase(sql, source, target string) string {
	var b strings.Builder
	qt := quoteCHIdent(target)
	isWord := func(c byte) bool {
		return c == '_' || c == '$' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	prevWord := func(i int) string {
		j := i
		for j > 0 && sql[j-1] == ' ' {
			j--
		}
		k := j
		for k > 0 && isWord(sql[k-1]) {
			k--
		}
		return strings.ToUpper(sql[k:j])
	}
	for i := 0; i < len(sql); {
		ch := sql[i]
		switch {
		case ch == '\'':
			j := i + 1
			for j < len(sql) && sql[j] != '\'' {
				if sql[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(sql))
			lit := sql[i:j]
			switch {
			case lit == quoteCHString(source) && prevWord(i) == "DB":
				lit = quoteCHString(target)
			case strings.HasPrefix(lit, "'"+source+".") && chInDictCall(sql, i):
				// dictGet('db.dict', ...) names the dictionary as a string.
				lit = "'" + target + lit[len(source)+1:]
			}
			b.WriteString(lit)
			i = j
		case ch == '-' && i+1 < len(sql) && sql[i+1] == '-':
			j := strings.IndexByte(sql[i:], '\n')
			if j < 0 {
				j = len(sql) - i
			}
			b.WriteString(sql[i : i+j])
			i += j
		case ch == '/' && i+1 < len(sql) && sql[i+1] == '*':
			j := strings.Index(sql[i+2:], "*/")
			end := len(sql)
			if j >= 0 {
				end = i + 2 + j + 2
			}
			b.WriteString(sql[i:end])
			i = end
		case ch == '`' || ch == '"':
			j := i + 1
			for j < len(sql) && sql[j] != ch {
				if sql[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(sql))
			ident := sql[i:j]
			name := strings.NewReplacer(`\\`, `\`, "\\"+string(ch), string(ch)).Replace(ident[1 : len(ident)-1])
			if name == source && j < len(sql) && sql[j] == '.' {
				ident = qt
			}
			b.WriteString(ident)
			i = j
		case isWord(ch) && (i == 0 || !isWord(sql[i-1]) && sql[i-1] != '.'):
			j := i
			for j < len(sql) && isWord(sql[j]) {
				j++
			}
			word := sql[i:j]
			if word == source && j < len(sql) && sql[j] == '.' {
				word = qt
			}
			b.WriteString(word)
			i = j
		default:
			b.WriteByte(ch)
			i++
		}
	}
	return b.String()
}
