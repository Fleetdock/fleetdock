package executor

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// DumpPreflight reports whether a dump of engineName could start here: the
// engine is known and its dump tool is installed.
func DumpPreflight(engineName string) error {
	eng, err := engine.For(engineName)
	if err != nil {
		return err
	}
	binaries, _, _ := eng.DumpArgs(engine.ConnParams{}, "")
	_, err = lookPath(binaries)
	return err
}

// Dump streams a gzip-compressed logical dump of one database to w — the same
// dump a stored backup contains, but nothing is written to disk or storage.
//
// onStart runs once, just before the first byte is written, so an HTTP caller
// can still answer with an error status when the dump fails to start (bad
// login, unreachable host, missing tool). A failure after that leaves w with a
// truncated stream, which gzip's trailer check exposes when it is read.
func Dump(ctx context.Context, engineName string, c engine.ConnParams, database string, w io.Writer, onStart func()) error {
	eng, err := engine.For(engineName)
	if err != nil {
		return err
	}
	conn, closeTunnel, err := cliConn(ctx, c, NopSink{})
	if err != nil {
		return err
	}
	defer closeTunnel()
	binaries, args, env := eng.DumpArgs(conn, database)
	bin, err := lookPath(binaries)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	stderr := &cappedBuffer{max: 64 << 10}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start dump: %w", err)
	}

	// Wait for output before committing to a response: a dump that cannot
	// connect exits without writing anything.
	out := bufio.NewReaderSize(stdout, 64<<10)
	if _, err := out.Peek(1); err != nil {
		_, _ = io.Copy(io.Discard, out)
		if werr := cmd.Wait(); werr != nil {
			return fmt.Errorf("dump failed: %s", firstLine(stderr.String()))
		}
		return errors.New("dump failed: the dump tool produced no output")
	}

	onStart()
	gz := gzip.NewWriter(w)
	if _, err := io.Copy(gz, out); err != nil {
		// The client went away or the stream broke: stop the tool rather
		// than leave it blocked writing to a pipe nobody reads.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("stream dump: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("dump failed: %s", firstLine(stderr.String()))
	}
	return gz.Close()
}

// cappedBuffer keeps the first max bytes written to it and discards the rest,
// so a chatty tool cannot grow memory without bound.
type cappedBuffer struct {
	buf []byte
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.max - len(b.buf); room > 0 {
		if len(p) > room {
			b.buf = append(b.buf, p[:room]...)
		} else {
			b.buf = append(b.buf, p...)
		}
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string { return string(b.buf) }
