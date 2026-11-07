// Package metrics keeps the control plane's own counters and writes them in
// the Prometheus text format for GET /metrics. It has no dependencies: a few
// counters, one histogram and gauges read at scrape time are all Fleetdock
// needs, and the exposition format is plain text.
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	mu        sync.Mutex
	requests  = map[[2]string]uint64{} // method, status class -> count
	jobs      = map[[2]string]uint64{} // job type, status -> count
	durations = newHistogram([]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30})

	workerTick atomic.Int64 // unix seconds of the worker loop's last turn
	started    = time.Now()
)

type histogram struct {
	bounds []float64
	counts []uint64 // per bucket, not cumulative
	sum    float64
	n      uint64
}

func newHistogram(bounds []float64) *histogram {
	return &histogram{bounds: bounds, counts: make([]uint64, len(bounds)+1)}
}

func (h *histogram) observe(v float64) {
	i := sort.SearchFloat64s(h.bounds, v)
	h.counts[i]++
	h.sum += v
	h.n++
}

// ObserveRequest records one HTTP request.
func ObserveRequest(method string, status int, d time.Duration) {
	class := strconv.Itoa(status/100) + "xx"
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
	default:
		method = "other"
	}
	mu.Lock()
	requests[[2]string{method, class}]++
	durations.observe(d.Seconds())
	mu.Unlock()
}

// JobFinished records a finished operation (backup, restore, ...).
func JobFinished(jobType, status string) {
	mu.Lock()
	jobs[[2]string{jobType, status}]++
	mu.Unlock()
}

// WorkerTick marks the background worker as alive.
func WorkerTick() { workerTick.Store(time.Now().Unix()) }

// WorkerStalled reports whether the worker has not turned for longer than
// limit. A process that started less than limit ago is never stalled.
func WorkerStalled(limit time.Duration) bool {
	if time.Since(started) < limit {
		return false
	}
	last := workerTick.Load()
	return last == 0 || time.Since(time.Unix(last, 0)) > limit
}

// Gauge is one value read at scrape time.
type Gauge struct {
	Name   string // without the fleetdock_ prefix
	Help   string
	Labels map[string]string
	Value  float64
}

// Write renders every metric. Gauges with the same name must be adjacent.
func Write(w io.Writer, version, commit string, gauges []Gauge) error {
	var b strings.Builder
	family := func(name, typ, help string) {
		fmt.Fprintf(&b, "# HELP fleetdock_%s %s\n# TYPE fleetdock_%s %s\n", name, help, name, typ)
	}
	family("build_info", "gauge", "The running build; the value is always 1.")
	fmt.Fprintf(&b, "fleetdock_build_info{version=%s,commit=%s} 1\n", quote(version), quote(commit))
	family("start_time_seconds", "gauge", "When this process started, as a Unix time.")
	fmt.Fprintf(&b, "fleetdock_start_time_seconds %d\n", started.Unix())

	mu.Lock()
	family("http_requests_total", "counter", "HTTP requests served, by method and status class.")
	for _, k := range sortedKeys(requests) {
		fmt.Fprintf(&b, "fleetdock_http_requests_total{method=%s,code=%s} %d\n", quote(k[0]), quote(k[1]), requests[k])
	}
	family("http_request_duration_seconds", "histogram", "Time to serve HTTP requests.")
	var cum uint64
	for i, bound := range durations.bounds {
		cum += durations.counts[i]
		fmt.Fprintf(&b, "fleetdock_http_request_duration_seconds_bucket{le=%s} %d\n", quote(fmtFloat(bound)), cum)
	}
	cum += durations.counts[len(durations.bounds)]
	fmt.Fprintf(&b, "fleetdock_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", cum)
	fmt.Fprintf(&b, "fleetdock_http_request_duration_seconds_sum %s\n", fmtFloat(durations.sum))
	fmt.Fprintf(&b, "fleetdock_http_request_duration_seconds_count %d\n", durations.n)
	family("operations_finished_total", "counter", "Operations (backups, restores, provisioning, ...) finished by this process, by type and result.")
	for _, k := range sortedKeys(jobs) {
		fmt.Fprintf(&b, "fleetdock_operations_finished_total{type=%s,status=%s} %d\n", quote(k[0]), quote(k[1]), jobs[k])
	}
	mu.Unlock()

	family("worker_last_run_seconds", "gauge", "When the background worker last ran, as a Unix time (0: not running here).")
	fmt.Fprintf(&b, "fleetdock_worker_last_run_seconds %d\n", workerTick.Load())

	last := ""
	for _, g := range gauges {
		if g.Name != last {
			family(g.Name, "gauge", g.Help)
			last = g.Name
		}
		fmt.Fprintf(&b, "fleetdock_%s%s %s\n", g.Name, labels(g.Labels), fmtFloat(g.Value))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func sortedKeys(m map[[2]string]uint64) [][2]string {
	keys := make([][2]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	return keys
}

func labels(l map[string]string) string {
	if len(l) == 0 {
		return ""
	}
	keys := make([]string, 0, len(l))
	for k := range l {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + quote(l[k])
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func fmtFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "+Inf"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}
