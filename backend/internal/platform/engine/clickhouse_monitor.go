package engine

import (
	"context"
	"fmt"
	"strconv"
)

// chProcessID maps a query id onto the int64 the Monitor contract uses.
// Fleetdock's own statements carry numeric query ids (see newQueryID), which
// are reported as themselves; any other id (a client's UUID) is reported as
// its hash, which KillProcess matches the same way.
const chProcessID = "if(toString(toInt64OrZero(query_id)) = query_id, toInt64OrZero(query_id), reinterpretAsInt64(cityHash64(query_id)))"

// Processes lists running queries. ClickHouse sessions hold no state worth
// showing between queries, so idle connections are not listed.
func (c *ClickHouse) Processes(ctx context.Context, p ConnParams) ([]Process, error) {
	rows, err := c.rows(ctx, p, chRequest{query: fmt.Sprintf(`
		SELECT %s, user, toString(address), current_database, toInt64(elapsed),
		       substring(query, 1, %d), if(is_cancelled, 'Cancelling', 'Query')
		FROM system.processes
		WHERE query_id != queryID()
		ORDER BY elapsed DESC`, chProcessID, maxProcessQuery)})
	if err != nil {
		return nil, err
	}
	out := make([]Process, 0, len(rows))
	for _, r := range rows {
		out = append(out, Process{
			ID: i64(r[0]), User: str(r[1]), Host: str(r[2]), Database: r[3],
			Seconds: i64(r[4]), Query: r[5], State: str(r[6]),
		})
	}
	return out, nil
}

// KillProcess cancels a running query. ClickHouse has no session to
// terminate, so connection=true does the same.
func (c *ClickHouse) KillProcess(ctx context.Context, p ConnParams, id int64, _ bool) error {
	return c.exec(ctx, p, chRequest{
		query:  fmt.Sprintf("KILL QUERY WHERE %s = {id:Int64} ASYNC", chProcessID),
		params: map[string]string{"id": strconv.FormatInt(id, 10)},
	})
}

// ServerStatus reports the server's current metrics plus version and uptime.
func (c *ClickHouse) ServerStatus(ctx context.Context, p ConnParams) ([]Setting, error) {
	rows, err := c.rows(ctx, p, chRequest{query: `
		SELECT 'Version', version()
		UNION ALL SELECT 'Uptime', toString(uptime())
		UNION ALL SELECT metric, toString(value) FROM (SELECT metric, value FROM system.metrics ORDER BY metric)`})
	if err != nil {
		return nil, err
	}
	return chSettings(rows), nil
}

// Variables reports the server configuration (system.server_settings).
func (c *ClickHouse) Variables(ctx context.Context, p ConnParams) ([]Setting, error) {
	rows, err := c.rows(ctx, p, chRequest{query: "SELECT name, value FROM system.server_settings ORDER BY name"})
	if err != nil {
		// Servers before 23.3 have no server_settings; show the session's
		// changed settings instead.
		rows, err = c.rows(ctx, p, chRequest{query: "SELECT name, value FROM system.settings WHERE changed ORDER BY name"})
		if err != nil {
			return nil, err
		}
	}
	return chSettings(rows), nil
}

func chSettings(rows [][]*string) []Setting {
	out := make([]Setting, 0, len(rows))
	for _, r := range rows {
		out = append(out, Setting{Name: str(r[0]), Value: str(r[1])})
	}
	return out
}

// DatabaseStats reports each database's size on disk (active parts) and how
// many queries are running in it.
func (c *ClickHouse) DatabaseStats(ctx context.Context, p ConnParams) ([]DatabaseStat, error) {
	rows, err := c.rows(ctx, p, chRequest{query: `
		SELECT d.name,
		       toInt64(ifNull(s.bytes, 0)),
		       toInt64(ifNull(q.n, 0))
		FROM system.databases d
		LEFT JOIN (SELECT database, sum(bytes_on_disk) AS bytes FROM system.parts WHERE active GROUP BY database) s
		       ON s.database = d.name
		LEFT JOIN (SELECT current_database AS database, count() AS n FROM system.processes GROUP BY database) q
		       ON q.database = d.name
		ORDER BY d.name
		SETTINGS join_use_nulls = 1`})
	if err != nil {
		return nil, err
	}
	out := make([]DatabaseStat, 0, len(rows))
	for _, r := range rows {
		if chVirtualDatabases[str(r[0])] {
			continue
		}
		out = append(out, DatabaseStat{Name: str(r[0]), SizeBytes: i64(r[1]), Connections: int(i64(r[2]))})
	}
	return out, nil
}
