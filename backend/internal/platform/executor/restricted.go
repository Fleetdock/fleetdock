package executor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A SQL stream fed to psql or mariadb may hold client commands (\! in psql,
// system in mariadb) that run programs or touch files on the host running the
// tool: the control plane for external instances, the agent host for managed
// ones. The stream comes from a user's upload or from a database's own
// contents, so neither is trusted. guardRestore makes the client refuse those
// commands, and refuses to run a client too old to do so.

// toolOutput runs a client binary with args and returns its combined output.
// Tests replace it.
var toolOutput = func(ctx context.Context, bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	return string(out), err
}

// guardRestore returns the stream to feed bin (the restore client for
// engineName) so that client commands in it are refused, or an error when bin
// cannot refuse them.
func guardRestore(ctx context.Context, engineName, bin string, in io.Reader) (io.Reader, error) {
	switch engineName {
	case "postgres":
		return restrictPSQL(ctx, bin, in)
	case "mariadb", "mysql":
		if err := checkSandbox(ctx, bin); err != nil {
			return nil, err
		}
		return in, nil
	}
	return nil, fmt.Errorf("no safe restore client for engine %q", engineName)
}

// restrictPSQL wraps in between \restrict and \unrestrict with a random key,
// so psql refuses every meta-command in between. The stream cannot leave
// restricted mode without the key. pg_dump's own \restrict lines are dropped
// by pgDumpCompat before this.
func restrictPSQL(ctx context.Context, bin string, in io.Reader) (io.Reader, error) {
	out, err := toolOutput(ctx, bin, "--version")
	if err != nil {
		return nil, fmt.Errorf("check %s version: %s: %w", bin, firstLine(out), err)
	}
	major, minor, ok := parsePSQLVersion(out)
	if !ok {
		return nil, fmt.Errorf("cannot read the psql version from %q", strings.TrimSpace(out))
	}
	if need, ok := psqlRestrictSupported(major, minor); !ok {
		return nil, fmt.Errorf("psql %d.%d at %s is too old to restore safely: it cannot refuse client commands in the SQL; upgrade it to %s or later", major, minor, bin, need)
	}
	key, err := restrictKey()
	if err != nil {
		return nil, err
	}
	return io.MultiReader(
		strings.NewReader("\\restrict "+key+"\n"),
		in,
		strings.NewReader("\n\\unrestrict "+key+"\n"),
	), nil
}

var psqlVersionRe = regexp.MustCompile(`\(PostgreSQL\)\s+(\d+)(?:\.(\d+))?`)

// parsePSQLVersion reads "psql (PostgreSQL) 17.6 (Debian 17.6-1)" and the
// like. A pre-release such as "18beta1" reads as 18.0.
func parsePSQLVersion(s string) (major, minor int, ok bool) {
	m := psqlVersionRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	if m[2] != "" {
		minor, _ = strconv.Atoi(m[2])
	}
	return major, minor, true
}

// psqlRestrictMinor is the first minor release of each major that has
// \restrict (the CVE-2025-8714 releases); every major from 18 on has it.
var psqlRestrictMinor = map[int]int{13: 22, 14: 19, 15: 14, 16: 10, 17: 6}

// psqlRestrictSupported reports whether psql major.minor has \restrict, and
// otherwise the oldest version to upgrade to.
func psqlRestrictSupported(major, minor int) (string, bool) {
	if major >= 18 {
		return "", true
	}
	if need, ok := psqlRestrictMinor[major]; ok {
		return fmt.Sprintf("%d.%d", major, need), minor >= need
	}
	return "17.6", false
}

// restrictKey returns a random alphanumeric key for \restrict.
func restrictKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// checkSandbox makes sure bin has the --sandbox mode RestoreArgs turns on:
// MariaDB's client from 10.5.25, 10.6.18, 10.11.8 and 11.4.2. Oracle's mysql
// client has none. The mode is read from --help, because --version exits
// before unknown options are reported.
func checkSandbox(ctx context.Context, bin string) error {
	help, err := toolOutput(ctx, bin, "--help")
	if err == nil && sandboxOptionRe.MatchString(help) {
		return nil
	}
	ver, _ := toolOutput(ctx, bin, "--version")
	return fmt.Errorf("%s (%s) cannot restore safely: it has no --sandbox mode to refuse client commands in the SQL; install the MariaDB client 10.5.25, 10.6.18, 10.11.8, 11.4.2 or later",
		bin, strings.TrimSpace(firstLine(ver)))
}

var sandboxOptionRe = regexp.MustCompile(`(?m)^\s*--sandbox\b`)
