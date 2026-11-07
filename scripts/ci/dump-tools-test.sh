#!/bin/sh
# Backups and restores shell out to the dump tools shipped in the image
# (mariadb-dump/mariadb for MySQL and MariaDB, pg_dump/psql for PostgreSQL).
# This runs them, from the image, against real servers of each supported
# engine: create a database with a table, a view, a trigger and a routine,
# dump it the way Fleetdock does, restore the dump into a fresh database and
# compare.
#
# Usage: scripts/ci/dump-tools-test.sh
#   TOOLS_IMAGE        image with the tools (default fleetdock:ci)
#   TOOLS_IMAGE_MYSQL  override for the MySQL/MariaDB tools
#   TOOLS_IMAGE_PG     override for the PostgreSQL tools
#   ENGINES            "mysql:8.4 mariadb:11.4 postgres:16 postgres:18" by default
set -eu

TOOLS_IMAGE="${TOOLS_IMAGE:-fleetdock:ci}"
TOOLS_IMAGE_MYSQL="${TOOLS_IMAGE_MYSQL:-$TOOLS_IMAGE}"
TOOLS_IMAGE_PG="${TOOLS_IMAGE_PG:-$TOOLS_IMAGE}"
ENGINES="${ENGINES:-mysql:8.4 mariadb:11.4 postgres:16 postgres:18}"
PW=dumptest-pw
started=""
cleanup() { for c in $started; do docker rm -f "$c" >/dev/null 2>&1 || true; done; }
trap cleanup EXIT

tools() { # IMAGE SCRIPT — run a shell script with the image's tools, on the host network
  docker run --rm --network host --entrypoint sh -e "MYSQL_PWD=$PW" -e "PGPASSWORD=$PW" "$1" -c "$2"
}

port=34500
for engine in $ENGINES; do
  port=$((port + 1))
  name="dumptest-$(echo "$engine" | tr ':.' '--')"
  case "$engine" in
    postgres:*)
      docker run -d --name "$name" -e POSTGRES_PASSWORD="$PW" -p "127.0.0.1:${port}:5432" "${engine}-alpine" >/dev/null
      started="$started $name"
      i=0; until docker exec "$name" pg_isready -U postgres >/dev/null 2>&1; do i=$((i+1)); [ $i -lt 60 ] || { echo "$engine did not start" >&2; exit 1; }; sleep 1; done
      sleep 2
      conn="-h 127.0.0.1 -p $port -U postgres"
      tools "$TOOLS_IMAGE_PG" "
        set -e
        psql $conn -qc 'CREATE DATABASE src' -c 'CREATE DATABASE dst'
        psql $conn -d src -q -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE orders (id serial PRIMARY KEY, total numeric(10,2) NOT NULL, note text);
INSERT INTO orders (total, note) SELECT g, 'line ' || g FROM generate_series(1, 500) g;
CREATE VIEW big_orders AS SELECT * FROM orders WHERE total > 250;
CREATE FUNCTION touch() RETURNS trigger LANGUAGE plpgsql AS \$\$ BEGIN NEW.note := coalesce(NEW.note, 'x'); RETURN NEW; END \$\$;
CREATE TRIGGER orders_touch BEFORE INSERT ON orders FOR EACH ROW EXECUTE FUNCTION touch();
SQL
        pg_dump $conn src > /tmp/dump.sql
        # What Fleetdock strips for older servers (executor.pgDumpCompat).
        grep -v '^SET transaction_timeout = 0;\$' /tmp/dump.sql | psql $conn -d dst -q -v ON_ERROR_STOP=1 >/dev/null
        got=\$(psql $conn -d dst -Atc \"SELECT count(*) || '/' || (SELECT count(*) FROM big_orders) FROM orders\")
        [ \"\$got\" = 500/250 ] || { echo \"restored \$got rows, want 500/250\"; exit 1; }
        psql $conn -d dst -qc 'INSERT INTO orders (total) VALUES (9999)'
        [ \"\$(psql $conn -d dst -Atc 'SELECT note FROM orders WHERE total = 9999')\" = x ] || { echo 'the trigger did not come back'; exit 1; }
        echo \"$engine: \$(pg_dump --version) ok\"
      "
      ;;
    mysql:*|mariadb:*)
      if [ "${engine%%:*}" = mysql ]; then env="-e MYSQL_ROOT_PASSWORD=$PW"; ping="mysqladmin ping -h127.0.0.1 -p$PW"
      else env="-e MARIADB_ROOT_PASSWORD=$PW"; ping="healthcheck.sh --connect --innodb_initialized"; fi
      # shellcheck disable=SC2086
      docker run -d --name "$name" $env -p "127.0.0.1:${port}:3306" "$engine" >/dev/null
      started="$started $name"
      i=0; until docker exec "$name" sh -c "$ping" >/dev/null 2>&1; do i=$((i+1)); [ $i -lt 120 ] || { echo "$engine did not start" >&2; exit 1; }; sleep 1; done
      sleep 3
      conn="--host=127.0.0.1 --port=$port --user=root"
      tools "$TOOLS_IMAGE_MYSQL" "
        set -e
        mariadb $conn -e 'CREATE DATABASE src; CREATE DATABASE dst'
        mariadb $conn src <<'SQL'
CREATE TABLE orders (id int AUTO_INCREMENT PRIMARY KEY, total decimal(10,2) NOT NULL, note varchar(100)) DEFAULT CHARSET=utf8mb4;
INSERT INTO orders (total, note) WITH RECURSIVE g(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM g WHERE n < 500) SELECT n, CONCAT('line ', n) FROM g;
CREATE VIEW big_orders AS SELECT * FROM orders WHERE total > 250;
CREATE TRIGGER orders_touch BEFORE INSERT ON orders FOR EACH ROW SET NEW.note = COALESCE(NEW.note, 'x');
CREATE PROCEDURE order_count(OUT n int) SELECT count(*) INTO n FROM orders;
SQL
        mariadb-dump $conn --single-transaction --quick --routines --triggers --events src > /tmp/dump.sql
        mariadb $conn dst < /tmp/dump.sql
        got=\$(mariadb $conn -N dst -e 'SELECT CONCAT(count(*), \"/\", (SELECT count(*) FROM big_orders)) FROM orders')
        [ \"\$got\" = 500/250 ] || { echo \"restored \$got rows, want 500/250\"; exit 1; }
        mariadb $conn dst -e 'INSERT INTO orders (total) VALUES (9999); CALL order_count(@n); SELECT @n' >/dev/null
        [ \"\$(mariadb $conn -N dst -e 'SELECT note FROM orders WHERE total = 9999')\" = x ] || { echo 'the trigger did not come back'; exit 1; }
        echo \"$engine: \$(mariadb-dump --version) ok\"
      "
      ;;
    *) echo "unknown engine $engine" >&2; exit 1 ;;
  esac
done
echo "Dump tools work against: $ENGINES"
