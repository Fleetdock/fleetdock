//go:build integration

package engine

import (
	"context"
	"strings"
	"testing"
)

// TestIntegrationPostgresExtensions installs an extension into a schema,
// lists it, refuses unknown ones and drops it.
func TestIntegrationPostgresExtensions(t *testing.T) {
	ctx := context.Background()
	root := itParams(t, "FLEETDOCK_IT_POSTGRES", "postgres")
	pg := &Postgres{}
	waitReady(t, pg, root)
	const db = "fd_it_ext"
	mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true)
	mustQuery(t, pg, root, "postgres", "CREATE DATABASE "+db, true)
	t.Cleanup(func() { mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true) })
	mustQuery(t, pg, root, db, "CREATE SCHEMA ext", true)

	find := func(name string) *Extension {
		list, err := pg.Extensions(ctx, root, db)
		if err != nil {
			t.Fatal(err)
		}
		for i := range list {
			if list[i].Name == name {
				return &list[i]
			}
		}
		return nil
	}
	if x := find("plpgsql"); x == nil || x.InstalledVersion == "" {
		t.Fatalf("plpgsql not listed as installed: %+v", x)
	}
	hs := find("hstore")
	if hs == nil || hs.InstalledVersion != "" || hs.DefaultVersion == "" || !hs.Trusted {
		t.Fatalf("hstore available = %+v", hs)
	}

	if err := pg.InstallExtension(ctx, root, db, "hstore", "ext", ""); err != nil {
		t.Fatalf("install: %v", err)
	}
	if hs = find("hstore"); hs.InstalledVersion != hs.DefaultVersion || hs.Schema != "ext" {
		t.Errorf("installed hstore = %+v", hs)
	}
	mustQuery(t, pg, root, db, "SELECT 'a=>1'::ext.hstore", false)
	if err := pg.UpdateExtension(ctx, root, db, "hstore", ""); err != nil {
		t.Errorf("update to the latest (already there): %v", err)
	}

	if err := pg.InstallExtension(ctx, root, db, "no_such_ext", "", ""); err == nil || !strings.Contains(err.Error(), "does not offer") {
		t.Errorf("unknown extension: %v", err)
	}
	if err := pg.InstallExtension(ctx, root, db, "citext", "", "1.0'; DROP TABLE x; --"); err == nil {
		t.Error("accepted a bad version")
	}
	if err := pg.DropExtension(ctx, root, db, "plpgsql", true); err == nil {
		t.Error("dropped plpgsql")
	}

	mustQuery(t, pg, root, db, "CREATE TABLE ext.kv (v ext.hstore)", true)
	if err := pg.DropExtension(ctx, root, db, "hstore", false); err == nil {
		t.Error("dropped an extension in use without cascade")
	}
	if err := pg.DropExtension(ctx, root, db, "hstore", true); err != nil {
		t.Fatalf("drop cascade: %v", err)
	}
	if hs = find("hstore"); hs.InstalledVersion != "" {
		t.Errorf("after drop: %+v", hs)
	}

	// A login without superuser gets a message that says why.
	mustQuery(t, pg, root, "postgres", "DROP ROLE IF EXISTS fd_it_ext_user", true)
	mustQuery(t, pg, root, "postgres", "CREATE ROLE fd_it_ext_user LOGIN PASSWORD 'ext-pw'", true)
	t.Cleanup(func() {
		mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true)
		mustQuery(t, pg, root, "postgres", "DROP ROLE IF EXISTS fd_it_ext_user", true)
	})
	mustQuery(t, pg, root, "postgres", "GRANT CREATE ON DATABASE "+db+" TO fd_it_ext_user", true)
	user := root
	user.User, user.Password = "fd_it_ext_user", "ext-pw"
	if err := pg.InstallExtension(ctx, user, db, "file_fdw", "", ""); err == nil || !strings.Contains(err.Error(), "superuser") {
		t.Errorf("untrusted extension as a plain login: %v", err)
	}
}
