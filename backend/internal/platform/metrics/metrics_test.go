package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestWrite(t *testing.T) {
	ObserveRequest("GET", 200, 30*time.Millisecond)
	ObserveRequest("GET", 204, 2*time.Second)
	ObserveRequest("BREW", 500, time.Millisecond)
	JobFinished("backup", "failed")
	WorkerTick()

	var b strings.Builder
	err := Write(&b, "1.0.0", "abc", []Gauge{
		{Name: "servers", Help: "Servers by status.", Labels: map[string]string{"status": "online"}, Value: 3},
		{Name: "servers", Help: "Servers by status.", Labels: map[string]string{"status": "offline"}, Value: 1},
		{Name: "databases", Help: "Databases.", Value: 12},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		`fleetdock_build_info{version="1.0.0",commit="abc"} 1`,
		`fleetdock_http_requests_total{method="GET",code="2xx"} 2`,
		`fleetdock_http_requests_total{method="other",code="5xx"} 1`,
		`fleetdock_http_request_duration_seconds_bucket{le="0.05"} 2`,
		`fleetdock_http_request_duration_seconds_bucket{le="+Inf"} 3`,
		`fleetdock_http_request_duration_seconds_count 3`,
		`fleetdock_operations_finished_total{type="backup",status="failed"} 1`,
		`fleetdock_servers{status="online"} 3`,
		`fleetdock_databases 12`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "# TYPE fleetdock_servers gauge") != 1 {
		t.Error("a gauge family must be declared once")
	}
	if WorkerStalled(time.Minute) {
		t.Error("the worker just ran")
	}
}
