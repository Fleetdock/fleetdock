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
// truncated stream is detected rather than half-restored.
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
	Database      string    `json:"database,omitempty"`
	ServerVersion string    `json:"server_version,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitzero"`

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
	if err := writeCHRecord(bw, chDumpRecord{Type: "manifest", Database: database, ServerVersion: version, CreatedAt: time.Now().UTC()}, nil); err != nil {
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
	return n, cw.Close()
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
// Restoring under a different name rewrites the source database's name in
// the definitions, and turns Replicated*MergeTree tables into their plain
// MergeTree form: kept replicated, the copy would join the original's
// replication and its restored rows would be inserted into production.
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
	log(fmt.Sprintf("backup of %s taken %s on ClickHouse %s", source, man.CreatedAt.Format(time.RFC3339), man.ServerVersion))

	var pending []*chPending
	defer func() {
		for _, pd := range pending {
			if pd.data != nil {
				_ = pd.data.Close()
				_ = os.Remove(pd.data.Name())
			}
		}
	}()

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
		create, notes := chRestoreDDL(rec.Create, source, database, renamed)
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

		if err := c.replaceObject(ctx, p, database, rec); err != nil {
			_ = payload.drain()
			return fmt.Errorf("create table %s: %w", rec.Name, err)
		}
		if len(rec.Columns) > 0 {
			n, err := c.insertNative(ctx, p, database, rec, payload)
			if err != nil {
				return fmt.Errorf("load table %s: %w", rec.Name, err)
			}
			log(fmt.Sprintf("table %s: %d rows", rec.Name, n))
		} else {
			if err := payload.drain(); err != nil {
				return err
			}
			log(fmt.Sprintf("table %s: definition (%s keeps no data here)", rec.Name, rec.Engine))
		}
	}

	// Dictionaries and views may depend on each other in any order: create
	// what can be created until a pass makes no progress.
	left := pending
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
	for _, pd := range pending {
		if pd.data == nil {
			continue
		}
		if _, err := pd.data.Seek(0, io.SeekStart); err != nil {
			return err
		}
		n, err := c.insertNative(ctx, p, database, pd.rec, bufio.NewReader(pd.data))
		if err != nil {
			return fmt.Errorf("load %s: %w", pd.rec.Name, err)
		}
		log(fmt.Sprintf("materialized view %s: %d rows", pd.rec.Name, n))
	}
	return nil
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

// insertNative loads a Native payload into an object's stored columns.
func (c *ClickHouse) insertNative(ctx context.Context, p ConnParams, database string, rec chDumpRecord, data io.Reader) (int64, error) {
	resp, err := c.open(ctx, p, chRequest{
		query:    fmt.Sprintf("INSERT INTO %s.%s (%s) FORMAT Native", quoteCHIdent(database), quoteCHIdent(rec.Name), chColumnList(rec.Columns)),
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

// chRestoreDDL adapts a CREATE statement to the target database.
func chRestoreDDL(create, source, target string, renamed bool) (string, []string) {
	if !renamed {
		return create, nil
	}
	var notes []string
	out := rewriteCHDatabase(create, source, target)
	out = chReplicatedRe.ReplaceAllStringFunc(out, func(m string) string {
		sub := chReplicatedRe.FindStringSubmatch(m)
		args := splitCHArgs(strings.TrimSuffix(strings.TrimPrefix(sub[2], "("), ")"))
		// The first two arguments, when given as literals, are the path and
		// replica; the rest belong to the MergeTree variant.
		rest := args
		if len(args) >= 2 && strings.HasPrefix(args[0], "'") && strings.HasPrefix(args[1], "'") {
			rest = args[2:]
		}
		notes = append(notes, "restored as non-replicated "+sub[1]+" (restoring under another name must not join the original's replication)")
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
			if lit == quoteCHString(source) && prevWord(i) == "DB" {
				lit = quoteCHString(target)
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
