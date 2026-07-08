package executor

import (
	"bytes"
	"strings"
	"sync"
	"time"
)

// LogLine is one buffered log entry with its assigned per-job sequence number.
type LogLine struct {
	Seq     int
	Level   string
	Message string
	Time    time.Time
}

// LogSink receives execution log lines. Implementations must be safe for
// concurrent use: os/exec copies a command's stderr on a separate goroutine
// from the one emitting step-level lines.
type LogSink interface {
	Log(level, message string)
}

// NopSink discards all lines. Use in tests and non-logging callers.
type NopSink struct{}

// Log implements LogSink.
func (NopSink) Log(string, string) {}

// BufferedSink assigns monotonic sequence numbers, buffers lines, and calls
// flush when the buffer reaches threshold or on an explicit Flush. It is the
// shared sink used by both executors; only the flush target differs (a DB
// write for the control-plane worker, an HTTP POST for the agent).
type BufferedSink struct {
	mu        sync.Mutex
	seq       int
	buf       []LogLine
	threshold int
	flush     func([]LogLine) error
	onErr     func(error)
}

// NewBufferedSink builds a sink. threshold is the buffered-line count that
// triggers an automatic flush (<= 0 flushes every line). flush receives a
// batch of buffered lines; onErr (optional) receives flush errors so the
// caller can log them without failing the operation.
func NewBufferedSink(threshold int, flush func([]LogLine) error, onErr func(error)) *BufferedSink {
	return &BufferedSink{threshold: threshold, flush: flush, onErr: onErr}
}

// Log appends a line and flushes when the buffer reaches the threshold.
func (s *BufferedSink) Log(level, message string) {
	if message == "" {
		return
	}
	s.mu.Lock()
	s.seq++
	s.buf = append(s.buf, LogLine{Seq: s.seq, Level: level, Message: message, Time: time.Now().UTC()})
	ready := s.threshold <= 0 || len(s.buf) >= s.threshold
	s.mu.Unlock()
	if ready {
		s.Flush()
	}
}

// Flush writes any buffered lines. Call it after Execute returns (and, for the
// agent, before posting terminal status) so lines persist ahead of the job
// being finalized. Flush failures are reported to onErr and the batch dropped;
// logs are best-effort and never fail the operation.
func (s *BufferedSink) Flush() {
	s.mu.Lock()
	if len(s.buf) == 0 {
		s.mu.Unlock()
		return
	}
	batch := s.buf
	s.buf = nil
	s.mu.Unlock()
	if err := s.flush(batch); err != nil && s.onErr != nil {
		s.onErr(err)
	}
}

// lineSink is an io.Writer that splits incoming bytes on newlines and emits
// each complete line to a LogSink at the given level, buffering a partial
// trailing line until the next write. It tees a command's stderr into the log
// stream; the caller still captures the raw stderr for the error message.
type lineSink struct {
	sink  LogSink
	level string
	buf   []byte
}

func newLineSink(sink LogSink, level string) *lineSink {
	return &lineSink{sink: sink, level: level}
}

// Write implements io.Writer. It never returns an error, so it is safe to use
// inside an io.MultiWriter that also writes to the real stderr buffer.
func (w *lineSink) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// Close emits any buffered, unterminated final line.
func (w *lineSink) Close() {
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}

func (w *lineSink) emit(line string) {
	line = strings.TrimRight(line, "\r")
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	// mariadb tools echo this on every invocation; it is noise, not output.
	if strings.Contains(line, "Using a password on the command line") {
		return
	}
	w.sink.Log(w.level, line)
}
