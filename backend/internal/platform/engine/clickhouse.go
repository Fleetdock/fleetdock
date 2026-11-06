package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ClickHouse is reached over its HTTP interface (8123, or 8443 with TLS)
// rather than the native protocol: every request is self-contained, the
// server enforces read-only mode per request (readonly=2), values are bound
// as typed query parameters, and the dial goes through the same netsafe /
// SSH-tunnel dialer as the other engines — with no extra driver.
//
// ClickHouse has no transactions and no row-level keys, so the data browser
// is read-only on it: DataEditor's write methods return ErrReadOnlyEngine.
func init() { Register("clickhouse", &ClickHouse{}) }

// ClickHouse implements Client, Admin, DataEditor, Monitor, Catalog and
// ObjectReader over the ClickHouse HTTP interface.
type ClickHouse struct{}

// ErrReadOnlyEngine is returned by row-editing calls on engines whose data
// browser is read-only.
var ErrReadOnlyEngine = errors.New("this engine's tables are read-only in the data browser; change data with the SQL console")

// ClickHouseError is an exception reported by the server.
type ClickHouseError struct {
	Code    int
	Message string
}

func (e *ClickHouseError) Error() string { return e.Message }

// ClickHouse exception codes Fleetdock reacts to.
const (
	chWrongPassword        = 193
	chRequiredPassword     = 194
	chIPAddressNotAllowed  = 195
	chUnknownUser          = 192
	chAccessDenied         = 497
	chAuthenticationFailed = 516
)

// chResultFormat is the output format for result sets: one JSON array per
// line, the first two carrying column names and types. Unlike the *Strings
// formats it renders NULL as null, so NULL and the string "NULL" stay apart.
const chResultFormat = "JSONCompactEachRowWithNamesAndTypes"

// chTimeout bounds administrative requests that carry no statement timeout.
const chTimeout = 60 * time.Second

// chRequest is one statement sent to the server.
type chRequest struct {
	query    string
	database string
	// params are bound to {name:Type} placeholders as param_<name>.
	params map[string]string
	// settings are ClickHouse settings for this statement only.
	settings map[string]string
	// readOnly makes the server refuse anything but reads (readonly=2 still
	// lets the statement change settings such as max_threads).
	readOnly bool
	// sessionID pins consecutive requests to one server session.
	sessionID string
	// queryID names the statement in system.processes so it can be killed.
	queryID string
	// body, when set, is the statement's input data (INSERT ... FORMAT);
	// the statement then travels in the URL.
	body io.Reader
}

// chResponse is an open response: the caller reads the body and closes it.
type chResponse struct {
	body *http.Response
	r    *bufio.Reader
	// format is the output format the server actually used; a statement with
	// its own FORMAT clause overrides chResultFormat.
	format string
}

func (r *chResponse) Close() error {
	// Closing an unread body drops the connection, which makes the server
	// cancel a still-running read (cancel_http_readonly_queries_on_client_close).
	return r.body.Body.Close()
}

// writtenRows reads written_rows from the X-ClickHouse-Summary header.
func (r *chResponse) writtenRows() int64 {
	var s struct {
		WrittenRows string `json:"written_rows"`
	}
	if err := json.Unmarshal([]byte(r.body.Header.Get("X-ClickHouse-Summary")), &s); err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(s.WrittenRows, 10, 64)
	return n
}

// header reads the two header lines of chResultFormat: names, then types.
// It returns nil slices when the statement produced no result set.
func (r *chResponse) header() (names, types []string, err error) {
	if r.format != chResultFormat {
		return nil, nil, nil
	}
	line, err := r.line()
	if err != nil || line == nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(line, &names); err != nil {
		return nil, nil, fmt.Errorf("clickhouse: unexpected result header: %w", err)
	}
	line, err = r.line()
	if err != nil {
		return nil, nil, err
	}
	if line != nil {
		if err := json.Unmarshal(line, &types); err != nil {
			return nil, nil, fmt.Errorf("clickhouse: unexpected result header: %w", err)
		}
	}
	return names, types, nil
}

// line returns the next non-empty line, or nil at the end of the body.
func (r *chResponse) line() ([]byte, error) {
	for {
		b, err := r.r.ReadBytes('\n')
		if len(bytes.TrimSpace(b)) > 0 {
			return bytes.TrimRight(b, "\r\n"), nil
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, nil
			}
			return nil, err
		}
	}
}

// row decodes the next data row into display strings; ok is false at the end.
// The server reports an exception that happens mid-stream as a trailing text
// line, which is returned as an error.
func (r *chResponse) row(maxLen int) (row []*string, ok bool, err error) {
	line, err := r.line()
	if err != nil || line == nil {
		return nil, false, err
	}
	var cells []json.RawMessage
	if err := json.Unmarshal(line, &cells); err != nil {
		if e := parseCHError(string(line)); e != nil {
			return nil, false, e
		}
		return nil, false, fmt.Errorf("clickhouse: unexpected result row: %w", err)
	}
	row = make([]*string, len(cells))
	for i, c := range cells {
		row[i] = chCell(c, maxLen)
	}
	return row, true, nil
}

// chCell renders one JSON cell: a string as itself, null as SQL NULL, and
// anything else (numbers, arrays, maps, tuples) as its JSON text, which keeps
// 64-bit integers and decimals exact.
func chCell(c json.RawMessage, maxLen int) *string {
	if len(c) == 0 || string(c) == "null" {
		return nil
	}
	var s string
	if c[0] == '"' {
		if err := json.Unmarshal(c, &s); err != nil {
			s = string(c)
		}
	} else {
		s = string(c)
	}
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return &s
}

// chErrorRe matches "Code: 81. DB::Exception: ..." in an error body.
var chErrorRe = regexp.MustCompile(`Code:\s*(\d+)\.\s*(?:DB::Exception:\s*)?(.*)`)

// parseCHError extracts a server exception from response text, or nil.
func parseCHError(text string) *ClickHouseError {
	m := chErrorRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	code, _ := strconv.Atoi(m[1])
	msg := strings.TrimSpace(m[2])
	// Drop the trailing "(version 25.8.1.1 (official build))".
	if i := strings.LastIndex(msg, " (version "); i > 0 {
		msg = msg[:i]
	}
	return &ClickHouseError{Code: code, Message: msg}
}

// scheme picks HTTP or HTTPS for a TLS mode. The HTTP interface has no
// in-band TLS negotiation, so "prefer" means HTTPS only on the conventional
// HTTPS ports.
func chScheme(p ConnParams) string {
	switch p.TLSMode {
	case TLSRequire, TLSVerifyFull:
		return "https"
	case TLSDisable:
		return "http"
	}
	if p.Port == 8443 || p.Port == 443 {
		return "https"
	}
	return "http"
}

// client builds an HTTP client that dials through netsafe or the SSH tunnel.
// Keep-alives are off: each request opens its own connection and closes it
// with the response, the same lifetime as the other engines' connections.
func (c *ClickHouse) client(p ConnParams) *http.Client {
	dial := p.dialFunc()
	tr := &http.Transport{
		DialContext:           func(ctx context.Context, network, addr string) (net.Conn, error) { return dial(ctx, network, addr) },
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 0, // a long statement sends headers only when it starts streaming
		DisableKeepAlives:     true,
		DisableCompression:    true,
	}
	if chScheme(p) == "https" {
		tr.TLSClientConfig = &tls.Config{
			ServerName:         p.Host,
			InsecureSkipVerify: p.TLSMode != TLSVerifyFull, //nolint:gosec // "require" means encrypted, unverified, as with libpq
			MinVersion:         tls.VersionTLS12,
		}
	}
	return &http.Client{Transport: tr}
}

// open sends a statement and returns the response for reading. Server
// exceptions are returned as *ClickHouseError.
func (c *ClickHouse) open(ctx context.Context, p ConnParams, req chRequest) (*chResponse, error) {
	q := url.Values{}
	q.Set("default_format", chResultFormat)
	q.Set("output_format_json_quote_decimals", "1")
	q.Set("output_format_json_named_tuples_as_objects", "0")
	// Without this the server reports progress and exceptions only at the
	// end; with it a client disconnect cancels a running read.
	q.Set("cancel_http_readonly_queries_on_client_close", "1")
	if req.database != "" {
		q.Set("database", req.database)
	}
	if req.readOnly {
		q.Set("readonly", "2")
	}
	if t := p.StatementTimeout; t > 0 {
		q.Set("max_execution_time", strconv.Itoa(int(math.Ceil(t.Seconds()))))
	}
	if req.sessionID != "" {
		q.Set("session_id", req.sessionID)
		q.Set("session_timeout", "60")
	}
	if req.queryID != "" {
		q.Set("query_id", req.queryID)
	}
	for k, v := range req.settings {
		q.Set(k, v)
	}
	for k, v := range req.params {
		q.Set("param_"+k, v)
	}
	var body io.Reader = strings.NewReader(req.query)
	if req.body != nil {
		q.Set("query", req.query)
		body = req.body
	}
	u := url.URL{
		Scheme:   chScheme(p),
		Host:     net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Path:     "/",
		RawQuery: q.Encode(),
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), body)
	if err != nil {
		return nil, err
	}
	// Credentials in headers, never in the URL, so they cannot leak into
	// server or proxy access logs.
	hreq.Header.Set("X-ClickHouse-User", p.User)
	hreq.Header.Set("X-ClickHouse-Key", p.Password)
	if req.body == nil {
		hreq.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}

	resp, err := c.client(p).Do(hreq)
	if err != nil {
		// Report the transport failure without the request URL, which
		// carries the statement's parameter values.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return nil, fmt.Errorf("clickhouse: %w", uerr.Err)
		}
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if e := parseCHError(string(body)); e != nil {
			return nil, e
		}
		if code, err := strconv.Atoi(resp.Header.Get("X-ClickHouse-Exception-Code")); err == nil {
			return nil, &ClickHouseError{Code: code, Message: strings.TrimSpace(string(body))}
		}
		return nil, fmt.Errorf("clickhouse: HTTP %d: %s", resp.StatusCode, firstLineOf(string(body)))
	}
	return &chResponse{body: resp, r: bufio.NewReaderSize(resp.Body, 64<<10), format: resp.Header.Get("X-ClickHouse-Format")}, nil
}

func firstLineOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// exec runs a statement that returns no rows worth reading.
func (c *ClickHouse) exec(ctx context.Context, p ConnParams, req chRequest) error {
	ctx, cancel := withDefaultTimeout(ctx, chTimeout)
	defer cancel()
	resp, err := c.open(ctx, p, req)
	if err != nil {
		return err
	}
	return resp.Close()
}

// rows runs a read and returns every row as display strings (no truncation).
func (c *ClickHouse) rows(ctx context.Context, p ConnParams, req chRequest) ([][]*string, error) {
	ctx, cancel := withDefaultTimeout(ctx, chTimeout)
	defer cancel()
	req.readOnly = true
	resp, err := c.open(ctx, p, req)
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	if _, _, err := resp.header(); err != nil {
		return nil, err
	}
	var out [][]*string
	for {
		row, ok, err := resp.row(0)
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		out = append(out, row)
	}
}

func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

// str dereferences a cell, with "" for NULL.
func str(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func i64(v *string) int64 {
	n, _ := strconv.ParseInt(str(v), 10, 64)
	return n
}

// quoteCHIdent quotes an identifier with backticks.
func quoteCHIdent(name string) string {
	r := strings.NewReplacer(`\`, `\\`, "`", "\\`")
	return "`" + r.Replace(name) + "`"
}

// quoteCHString renders a string literal.
func quoteCHString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(s) + "'"
}

// chSystemDatabases are engine-owned databases. system is listed (and
// importable for browsing) but never dropped; the information_schema
// variants are virtual and left out.
var chSystemDatabases = map[string]bool{"system": true}

var chVirtualDatabases = map[string]bool{"INFORMATION_SCHEMA": true, "information_schema": true}

// newQueryID returns a random positive int64 as a query id. Numeric ids let
// the Monitor report Fleetdock's own statements with their real id, which is
// what console cancellation passes back to KillProcess.
func newQueryID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.BigEndian.Uint64(b[:]) >> 1)
}

// Ping verifies connectivity and returns the server version.
func (c *ClickHouse) Ping(ctx context.Context, p ConnParams) (string, error) {
	rows, err := c.rows(ctx, p, chRequest{query: "SELECT version()"})
	if err != nil {
		return "", err
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return "", fmt.Errorf("clickhouse: empty version reply")
	}
	return str(rows[0][0]), nil
}

// ClickHouse has no per-database character set or collation: strings are
// bytes, by convention UTF-8, compared byte by byte. These are what its
// databases are recorded with.
const (
	CHCharset   = "UTF-8"
	CHCollation = "binary"
)

// ListDatabases returns the databases, without the virtual
// information_schema ones.
func (c *ClickHouse) ListDatabases(ctx context.Context, p ConnParams) ([]DatabaseInfo, error) {
	rows, err := c.rows(ctx, p, chRequest{query: "SELECT name FROM system.databases ORDER BY name"})
	if err != nil {
		return nil, err
	}
	out := make([]DatabaseInfo, 0, len(rows))
	for _, r := range rows {
		name := str(r[0])
		if chVirtualDatabases[name] {
			continue
		}
		out = append(out, DatabaseInfo{Name: name, Charset: CHCharset, Collation: CHCollation, System: chSystemDatabases[name]})
	}
	return out, nil
}

// CreateDatabase creates a database; charset and collation do not apply.
func (c *ClickHouse) CreateDatabase(ctx context.Context, p ConnParams, name, _, _ string) error {
	if !validDBName(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	return c.exec(ctx, p, chRequest{query: "CREATE DATABASE IF NOT EXISTS " + quoteCHIdent(name)})
}

// DropDatabase drops a database by validated identifier.
func (c *ClickHouse) DropDatabase(ctx context.Context, p ConnParams, name string) error {
	if !validDBName(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	if chSystemDatabases[name] || chVirtualDatabases[name] {
		return fmt.Errorf("refusing to drop system database %q", name)
	}
	return c.exec(ctx, p, chRequest{query: "DROP DATABASE IF EXISTS " + quoteCHIdent(name)})
}

// CountTables returns the number of tables (and views) in a database.
func (c *ClickHouse) CountTables(ctx context.Context, p ConnParams, database string) (int, error) {
	rows, err := c.rows(ctx, p, chRequest{
		query:  "SELECT count() FROM system.tables WHERE database = {db:String} AND NOT is_temporary",
		params: map[string]string{"db": database},
	})
	if err != nil {
		return 0, err
	}
	return int(i64(rows[0][0])), nil
}

// DumpArgs: logical backups of ClickHouse are not supported yet; the backup
// service refuses them before a job is created.
func (c *ClickHouse) DumpArgs(ConnParams, string) ([]string, []string, []string) {
	return nil, nil, nil
}

// RestoreArgs: see DumpArgs.
func (c *ClickHouse) RestoreArgs(ConnParams, string) ([]string, []string, []string) {
	return nil, nil, nil
}
