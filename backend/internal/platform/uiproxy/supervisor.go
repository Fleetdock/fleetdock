package uiproxy

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Supervisor owns the dashboard child process and the reverse proxy in front of
// it. The zero value is not usable; construct one with New.
type Supervisor struct {
	cfg     Config
	handler http.Handler

	mu   sync.Mutex
	cmd  *exec.Cmd
	proc *os.Process

	ready    atomic.Bool
	stopping atomic.Bool
	exited   chan struct{}
	once     sync.Once
}

// Addr is the loopback address the dashboard listens on.
func (s *Supervisor) Addr() string {
	return net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
}

// Ready reports whether the dashboard is currently accepting connections.
func (s *Supervisor) Ready() bool { return s.ready.Load() }

// Start spawns the dashboard and begins supervising it.
//
// It waits up to StartupTimeout for the child to accept connections, but a
// slow or failed start is logged rather than returned: a wedged dashboard must
// not take the API down with it. Handler serves 503 until the child is up.
func (s *Supervisor) Start(ctx context.Context) {
	if err := s.spawn(); err != nil {
		slog.Error("dashboard failed to start", "error", err.Error())
	}
	go s.supervise(ctx)

	if err := s.waitReady(ctx, s.cfg.StartupTimeout); err != nil {
		slog.Warn("dashboard not ready yet; serving 503 until it is",
			"addr", s.Addr(), "error", err.Error())
		return
	}
	s.ready.Store(true)
	slog.Info("dashboard ready", "addr", s.Addr())
}

// spawn starts one instance of the dashboard. The caller supervises it.
func (s *Supervisor) spawn() error {
	cmd := exec.Command(s.cfg.NodeBin, "server.js")
	cmd.Dir = s.cfg.Dir
	cmd.Env = append(os.Environ(),
		"NODE_ENV=production",
		"NEXT_TELEMETRY_DISABLED=1",
		// Next's standalone server binds process.env.HOSTNAME. Docker sets
		// HOSTNAME to the container id, which node cannot bind, so the child
		// would die immediately without this override.
		"HOSTNAME="+s.cfg.Host,
		"PORT="+strconv.Itoa(s.cfg.Port),
	)
	cmd.Stdout = &logWriter{level: slog.LevelInfo}
	cmd.Stderr = &logWriter{level: slog.LevelWarn}
	// Own process group: a Ctrl-C in a terminal must not reach node
	// independently of this process's ordered shutdown, and it lets Shutdown
	// signal the whole tree with a single kill.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start dashboard: %w", err)
	}

	s.mu.Lock()
	s.cmd, s.proc = cmd, cmd.Process
	s.mu.Unlock()
	slog.Info("dashboard started", "pid", cmd.Process.Pid, "addr", s.Addr())
	return nil
}

// supervise restarts the dashboard whenever it exits, until shutdown.
func (s *Supervisor) supervise(ctx context.Context) {
	delay := minRestartDelay
	for {
		s.mu.Lock()
		cmd := s.cmd
		s.mu.Unlock()

		if cmd != nil {
			err := cmd.Wait()
			if s.stopping.Load() || ctx.Err() != nil {
				s.once.Do(func() { close(s.exited) })
				return
			}
			s.ready.Store(false)
			slog.Error("dashboard exited; restarting", "error", errString(err), "in", delay.String())
		}

		select {
		case <-ctx.Done():
			s.once.Do(func() { close(s.exited) })
			return
		case <-time.After(delay):
		}

		if err := s.spawn(); err != nil {
			slog.Error("dashboard restart failed", "error", err.Error())
			delay = nextDelay(delay)
			continue
		}
		if err := s.waitReady(ctx, s.cfg.StartupTimeout); err != nil {
			slog.Warn("dashboard restarted but is not accepting connections", "error", err.Error())
			delay = nextDelay(delay)
			continue
		}
		s.ready.Store(true)
		slog.Info("dashboard recovered", "addr", s.Addr())
		delay = minRestartDelay
	}
}

// waitReady polls the loopback port until it accepts a connection. A TCP accept
// is enough — Next serves as soon as it is listening.
func (s *Supervisor) waitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		conn, err := net.DialTimeout("tcp", s.Addr(), 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("dashboard did not accept connections within %s: %w", timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Shutdown stops the dashboard. Call it after the HTTP server has drained, so
// in-flight proxied requests finish before their upstream disappears.
func (s *Supervisor) Shutdown(ctx context.Context) error {
	s.stopping.Store(true)
	s.ready.Store(false)

	s.mu.Lock()
	proc := s.proc
	s.mu.Unlock()
	if proc == nil {
		return nil
	}

	// Negative pid signals the whole process group created by Setpgid.
	if err := syscall.Kill(-proc.Pid, syscall.SIGTERM); err != nil {
		// Already gone is not a failure.
		if err != syscall.ESRCH {
			slog.Warn("signalling dashboard", "error", err.Error())
		}
	}

	select {
	case <-s.exited:
		slog.Info("dashboard stopped")
		return nil
	case <-ctx.Done():
		_ = syscall.Kill(-proc.Pid, syscall.SIGKILL)
		return fmt.Errorf("dashboard did not exit before deadline: %w", ctx.Err())
	}
}

func nextDelay(d time.Duration) time.Duration {
	d *= 2
	if d > maxRestartDelay {
		return maxRestartDelay
	}
	return d
}

func errString(err error) string {
	if err == nil {
		return "clean exit"
	}
	return err.Error()
}

// logWriter forwards the child's stdout/stderr into slog a line at a time, so
// dashboard output interleaves with the control plane's structured logs
// instead of bypassing them.
type logWriter struct {
	level slog.Level
	buf   bytes.Buffer
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		line := bytes.TrimRight(w.buf.Next(i+1), "\r\n")
		if len(line) > 0 {
			slog.Log(context.Background(), w.level, string(line), "source", "dashboard")
		}
	}
	return len(p), nil
}
