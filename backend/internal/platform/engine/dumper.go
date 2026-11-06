package engine

import (
	"context"
	"io"
)

// Dumper is implemented by engines whose logical backups Fleetdock produces
// in-process instead of through a client tool (DumpArgs / RestoreArgs). The
// executor gzips, hashes and uploads what Dump writes exactly as it does a
// tool's output, and hands Restore the decompressed stream.
type Dumper interface {
	// Dump writes a logical dump of one database to w. log receives progress
	// lines for the operation's log.
	Dump(ctx context.Context, p ConnParams, database string, w io.Writer, log func(string)) error
	// Restore loads a stream written by Dump into database, which may have
	// a different name from the one dumped. The database already exists.
	Restore(ctx context.Context, p ConnParams, database string, r io.Reader, log func(string)) error
	// DumpExtension is the file extension of a gzip-compressed dump
	// (".sql.gz" for the tool-based engines).
	DumpExtension() string
}

// DumpExtension returns the extension of an engine's compressed dump files.
func DumpExtension(engineName string) string {
	if c, err := For(engineName); err == nil {
		if d, ok := c.(Dumper); ok {
			return d.DumpExtension()
		}
	}
	return ".sql.gz"
}
