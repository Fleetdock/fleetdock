//go:build integration

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestIntegrationInsights(t *testing.T) {
	for _, c := range []struct {
		env, user, maint string
		eng              interface {
			Admin
			Insights
			Client
		}
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", &MariaDB{}},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", &MariaDB{}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", &Postgres{}},
	} {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			const db = "fd_it_insights"
			isPG := c.user == "postgres"
			mustQuery(t, c.eng, root, c.maint, "DROP DATABASE IF EXISTS "+db, true)
			mustQuery(t, c.eng, root, c.maint, "CREATE DATABASE "+db, true)
			for _, s := range []string{
				"CREATE TABLE items (id int PRIMARY KEY, region varchar(10), num int, qty int)",
				"CREATE INDEX ix_region ON items (region)",
				"CREATE INDEX ix_region_num ON items (region, num)",
				"INSERT INTO items VALUES (1, 'eu', 1, 5), (2, 'us', 2, 7)",
			} {
				mustQuery(t, c.eng, root, db, s, true)
			}

			rep, err := c.eng.IndexReport(ctx, root, db)
			if err != nil {
				t.Fatalf("index report: %v", err)
			}
			if len(rep.Indexes) != 3 {
				t.Fatalf("indexes = %+v", rep.Indexes)
			}
			if !rep.UsageAvailable && rep.Note == "" {
				t.Error("unavailable usage must say why")
			}
			if c.env == "FLEETDOCK_IT_MARIADB" && !rep.UsageAvailable {
				// MariaDB's other source: the user-statistics counters.
				mustQuery(t, c.eng, root, c.maint, "SET GLOBAL userstat = 1", true)
				defer mustQuery(t, c.eng, root, c.maint, "SET GLOBAL userstat = 0", true)
				mustQuery(t, c.eng, root, db, "SELECT * FROM items WHERE region = 'eu'", false)
				us, err := c.eng.IndexReport(ctx, root, db)
				if err != nil || !us.UsageAvailable {
					t.Fatalf("userstat report = %+v, %v", us, err)
				}
			}
			dialect := "mysql"
			if isPG {
				dialect = "postgres"
			}
			ins := AnalyzeIndexes(rep, dialect)
			if len(ins.Redundant) != 1 || ins.Redundant[0].Index.Name != "ix_region" || ins.Redundant[0].CoveredBy != "ix_region_num" {
				t.Fatalf("redundant = %+v", ins.Redundant)
			}
			// The suggested statement really drops it.
			mustQuery(t, c.eng, root, db, strings.TrimSuffix(ins.Redundant[0].DropSQL, ";"), true)
			if rep, _ := c.eng.IndexReport(ctx, root, db); len(rep.Indexes) != 2 {
				t.Errorf("after drop: %+v", rep.Indexes)
			}

			p := root
			p.Database = db
			plan, err := c.eng.Explain(ctx, p, db, "SELECT * FROM items WHERE region = 'eu'", false)
			if err != nil || plan.Analyzed || !json.Valid(plan.Plan) {
				t.Fatalf("explain = %+v, %v", plan, err)
			}
			an, err := c.eng.Explain(ctx, p, db, "SELECT count(*) FROM items", true)
			if err != nil || !an.Analyzed || (an.Format == "mysql-tree") != (an.Text != "") {
				t.Fatalf("explain analyze = %+v, %v", an, err)
			}
			// Explaining a write never executes it.
			if _, err := c.eng.Explain(ctx, p, db, "DELETE FROM items", false); err != nil {
				t.Fatalf("explain delete: %v", err)
			}
			if res := mustQuery(t, c.eng, root, db, "SELECT count(*) FROM items", false); *res.Rows[0][0] != "2" {
				t.Fatalf("rows after explain = %s", *res.Rows[0][0])
			}
			if _, err := c.eng.Explain(ctx, p, db, "SELECT 1; SELECT 2", false); err == nil {
				t.Error("two statements must be refused")
			}

			top, err := c.eng.TopQueries(ctx, root, TopQueriesRequest{Database: db})
			if err != nil {
				t.Fatalf("top queries: %v", err)
			}
			if !top.Available && top.Note == "" {
				t.Error("unavailable statement statistics must say why")
			}
			t.Logf("usage available=%v note=%q; top available=%v n=%d note=%q", rep.UsageAvailable, rep.Note, top.Available, len(top.Queries), top.Note)
		})
	}
}
