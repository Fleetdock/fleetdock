"use client";

import { useMemo, useState } from "react";

import { DataTable } from "@/components/data-table";
import { CreateTableModal } from "@/components/database/create-table-modal";
import { DBObjects } from "@/components/database/db-objects";

import { formatBytes as formatBytesOr } from "@/lib/format";
import { ApiError } from "@/lib/api";
import { useTables } from "@/lib/hooks";
import type { TableInfo } from "@/lib/types";
import { useDataTable } from "@/lib/use-data-table";
import { ChevronRight, Folder, Plus, Table2, X } from "lucide-react";

const formatBytes = (n: number) => formatBytesOr(n, "0 B");

// ---- Tables list ----

function compareTables(a: TableInfo, b: TableInfo, key: string) {
  if (key === "name" || key === "engine" || key === "schema") {
    return (a[key] ?? "").localeCompare(b[key] ?? "");
  }
  return (
    (a[key as "row_count" | "data_bytes" | "index_bytes"] as number) -
    (b[key as "row_count" | "data_bytes" | "index_bytes"] as number)
  );
}

type SchemaSummary = {
  name: string;
  tables: number;
  row_count: number;
  data_bytes: number;
  index_bytes: number;
};

function compareSchemas(a: SchemaSummary, b: SchemaSummary, key: string) {
  if (key === "name") return a.name.localeCompare(b.name);
  return (
    a[key as "tables" | "row_count" | "data_bytes" | "index_bytes"] -
    b[key as "tables" | "row_count" | "data_bytes" | "index_bytes"]
  );
}

// TablesBrowser owns the table list for the whole tab. A PostgreSQL database
// can spread its tables over several schemas, and a flat list mixes them into
// an ambiguous pile — same table name, different schema. When there is more
// than one schema the browser inserts a schema-picking step; when there is only
// one (always the case on MySQL/MariaDB, where a schema *is* a database) it
// goes straight to the tables and nothing about the old flow changes.
export function TablesBrowser({
  databaseId,
  schema,
  onOpenSchema,
  onCloseSchema,
  onOpenTable,
  canWrite = false,
}: {
  databaseId: string;
  schema: string | null;
  onOpenSchema: (schema: string) => void;
  onCloseSchema: () => void;
  onOpenTable: (table: string) => void;
  canWrite?: boolean;
}) {
  const { data, isLoading, error } = useTables(databaseId);
  const [creating, setCreating] = useState(false);
  const items = useMemo(() => data?.items ?? [], [data]);

  const schemas = useMemo(() => {
    const by = new Map<string, SchemaSummary>();
    for (const t of items) {
      const name = t.schema || "";
      let s = by.get(name);
      if (!s) {
        s = { name, tables: 0, row_count: 0, data_bytes: 0, index_bytes: 0 };
        by.set(name, s);
      }
      s.tables += 1;
      s.row_count += t.row_count;
      s.data_bytes += t.data_bytes;
      s.index_bytes += t.index_bytes;
    }
    return [...by.values()];
  }, [items]);

  const multiSchema = schemas.length > 1;

  // Where a new table goes: the open schema, or "public" on a PostgreSQL
  // database (schema-qualified table names), or nowhere on MySQL/MariaDB.
  const isPG = items.some((t) => t.engine === "postgres");
  const targetSchema = schema ?? (isPG ? "public" : null);

  return (
    <>
      {canWrite ? (
        <div className="flex justify-end" style={{ marginBottom: ".5rem" }}>
          <button className="btn btn-sm btn-primary" onClick={() => setCreating(true)}>
            <Plus size={15} /> Create table
          </button>
        </div>
      ) : null}
      {multiSchema && !schema ? (
        <SchemaList
          schemas={schemas}
          isLoading={isLoading}
          error={error ? (error as ApiError).message : undefined}
          onOpen={onOpenSchema}
        />
      ) : (
        <TablesSection
          items={multiSchema ? items.filter((t) => t.schema === schema) : items}
          isLoading={isLoading}
          error={error ? (error as ApiError).message : undefined}
          schema={multiSchema ? schema : null}
          onCloseSchema={onCloseSchema}
          onOpen={onOpenTable}
        />
      )}
      <DBObjects databaseId={databaseId} />
      {creating ? (
        <CreateTableModal
          databaseId={databaseId}
          schema={targetSchema}
          onClose={() => setCreating(false)}
          onCreated={onOpenTable}
        />
      ) : null}
    </>
  );
}

function SchemaList({
  schemas,
  isLoading,
  error,
  onOpen,
}: {
  schemas: SchemaSummary[];
  isLoading: boolean;
  error?: string;
  onOpen: (schema: string) => void;
}) {
  const [search, setSearch] = useState("");
  const table = useDataTable({
    items: schemas,
    search: {
      query: search,
      match: (s, q) => s.name.toLowerCase().includes(q),
    },
    sort: {
      key: "name",
      dir: "asc",
      compare: compareSchemas,
      defaultDir: (key) => (key === "name" ? "asc" : "desc"),
    },
  });

  return (
    <DataTable<SchemaSummary>
      columns={[
        {
          id: "name",
          header: "Schema",
          sortable: true,
          sortKey: "name",
          className: "font-medium flex items-center gap-2",
          render: (s) => (
            <>
              <Folder size={15} /> {s.name}
            </>
          ),
        },
        {
          id: "tables",
          header: "Tables",
          sortable: true,
          sortKey: "tables",
          className: "muted",
          render: (s) => s.tables.toLocaleString(),
        },
        {
          id: "row_count",
          header: "Rows (est.)",
          sortable: true,
          sortKey: "row_count",
          className: "muted",
          render: (s) => s.row_count.toLocaleString(),
        },
        {
          id: "data_bytes",
          header: "Size",
          sortable: true,
          sortKey: "data_bytes",
          className: "muted",
          render: (s) => formatBytes(s.data_bytes + s.index_bytes),
        },
        {
          id: "actions",
          header: "",
          align: "right",
          render: (s) => (
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => onOpen(s.name)}
            >
              Open <ChevronRight size={15} />
            </button>
          ),
        },
      ]}
      rows={table.rows}
      rowKey={(s) => s.name}
      isLoading={isLoading}
      loadingLabel="Connecting to the database server…"
      error={error}
      errorTitle="Could not reach the database server"
      emptyTitle="No schemas"
      emptyHint="This database has no tables yet."
      emptySearchTitle="No schemas match your search"
      emptySearchHint="Try a different name or clear the search."
      search={{
        value: search,
        onChange: (v) => {
          setSearch(v);
          table.setPage(1);
        },
        placeholder: "Search schemas…",
      }}
      sort={{ key: table.sortKey, dir: table.sortDir, onSort: table.setSort }}
      pagination={{
        page: table.page,
        pageCount: table.pageCount,
        onPage: table.setPage,
      }}
    />
  );
}

function TablesSection({
  items,
  isLoading,
  error,
  schema,
  onCloseSchema,
  onOpen,
}: {
  items: TableInfo[];
  isLoading: boolean;
  error?: string;
  schema: string | null;
  onCloseSchema: () => void;
  onOpen: (table: string) => void;
}) {
  const [search, setSearch] = useState("");

  // Inside a schema the name is unambiguous on screen, but the browse and
  // export APIs still need the qualified form to resolve it.
  const qualify = (t: TableInfo) => (schema ? `${t.schema}.${t.name}` : t.name);

  const table = useDataTable({
    items,
    search: {
      query: search,
      match: (t, q) => t.name.toLowerCase().includes(q),
    },
    sort: {
      key: "name",
      dir: "asc",
      compare: compareTables,
      defaultDir: (key) =>
        key === "name" || key === "engine" ? "asc" : "desc",
    },
  });

  return (
    <DataTable<TableInfo>
      columns={[
        {
          id: "name",
          header: "Table",
          sortable: true,
          sortKey: "name",
          className: "font-medium flex items-center gap-2",
          render: (t) => (
            <>
              <Table2 size={15} /> {t.name}
            </>
          ),
        },
        {
          id: "engine",
          header: "Engine",
          sortable: true,
          sortKey: "engine",
          className: "muted",
          render: (t) => t.engine,
        },
        {
          id: "row_count",
          header: "Rows (est.)",
          sortable: true,
          sortKey: "row_count",
          className: "muted",
          render: (t) => t.row_count.toLocaleString(),
        },
        {
          id: "data_bytes",
          header: "Data",
          sortable: true,
          sortKey: "data_bytes",
          className: "muted",
          render: (t) => formatBytes(t.data_bytes),
        },
        {
          id: "index_bytes",
          header: "Indexes",
          sortable: true,
          sortKey: "index_bytes",
          className: "muted",
          render: (t) => formatBytes(t.index_bytes),
        },
        {
          id: "actions",
          header: "",
          align: "right",
          render: (t) => (
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => onOpen(qualify(t))}
            >
              Browse <ChevronRight size={15} />
            </button>
          ),
        },
      ]}
      rows={table.rows}
      rowKey={(t) => `${t.schema}.${t.name}`}
      isLoading={isLoading}
      loadingLabel="Connecting to the database server…"
      error={error}
      errorTitle="Could not reach the database server"
      emptyTitle="No tables"
      emptyHint={
        schema
          ? `Schema "${schema}" has no base tables.`
          : "This database has no base tables yet."
      }
      emptySearchTitle="No tables match your search"
      emptySearchHint="Try a different name or clear the search."
      toolbar={
        schema ? (
          <div className="flex items-center gap-2">
            <button className="btn btn-sm" onClick={onCloseSchema}>
              <X size={15} /> All schemas
            </button>
            <span className="flex items-center gap-1 text-sm muted">
              <Folder size={14} /> {schema}
            </span>
          </div>
        ) : undefined
      }
      search={{
        value: search,
        onChange: (v) => {
          setSearch(v);
          table.setPage(1);
        },
        placeholder: schema ? `Search tables in ${schema}…` : "Search tables…",
      }}
      sort={{ key: table.sortKey, dir: table.sortDir, onSort: table.setSort }}
      pagination={{
        page: table.page,
        pageCount: table.pageCount,
        onPage: table.setPage,
      }}
    />
  );
}

// ---- Data browser with numbered pagination ----
