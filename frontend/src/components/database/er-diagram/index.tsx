"use client";

import { Network } from "lucide-react";
import dynamic from "next/dynamic";

import { ButtonLink, EmptyState, QueryError, Skeleton } from "@/components/ui";
import { useMemo, useState } from "react";

import {
  loadSelection,
  resolveSelection,
  saveSelection,
  selectedSchemas,
  summarizeSchemas,
  type SchemaSelection,
  type SchemaSummary,
} from "@/lib/data-browser/schemas";
import { useDiagram, useTables } from "@/lib/hooks";
import type { Database } from "@/lib/types";

function CanvasSkeleton() {
  return (
    <div role="status" aria-label="Loading diagram" className="erd erd-loading">
      <Skeleton height="2.25rem" width="60%" radius="var(--radius-md)" />
      <Skeleton height="100%" radius="var(--radius)" />
    </div>
  );
}

// React Flow, ELK and the image exporter only load with the diagram page.
const ERCanvas = dynamic(() => import("./canvas"), { ssr: false, loading: CanvasSkeleton });

/**
 * ERDiagram fills its container (the full-screen diagram page) with every
 * table, its columns and keys, and the foreign keys between them.
 */
export function ERDiagram({ database }: { database: Database }) {
  // The schema choice is shared with the data browser's table tree. A first
  // visit needs the schema list to pick a default, which the table list has.
  const tables = useTables(database.id);
  const [stored, setStored] = useState(() => loadSelection(database.id));
  const fromTables = useMemo(
    () => summarizeSchemas((tables.data?.items ?? []).map((t) => ({ schema: t.schema, kind: "table" }))),
    [tables.data],
  );
  const [summaryFromDiagram, setSummaryFromDiagram] = useState<SchemaSummary[] | null>(null);
  const summary = summaryFromDiagram ?? fromTables;
  const selection = resolveSelection(stored, summary);
  const filtered = summary.length > 1 && !selection.all;
  const schemas = filtered ? selectedSchemas(selection, summary) : undefined;
  const { data, isLoading, isPlaceholderData, isFetching, error, refetch } = useDiagram(
    database.id,
    schemas,
    tables.isFetched,
  );
  // The diagram's own schema list also counts views and empty schemas.
  const names = (l: SchemaSummary[] | null | undefined) => (l ?? []).map((x) => x.name).join(",");
  if (data?.schemas && names(data.schemas) !== names(summaryFromDiagram)) setSummaryFromDiagram(data.schemas);
  const schemaKey = schemas?.join(",") ?? "";
  // Which choice the data on screen belongs to: while another one loads, the
  // previous diagram stays as it is.
  const [dataKey, setDataKey] = useState(schemaKey);
  if (data && !isPlaceholderData && dataKey !== schemaKey) setDataKey(schemaKey);
  const shown = useMemo(() => (dataKey ? new Set(dataKey.split(",")) : undefined), [dataKey]);
  const onSelection = (sel: SchemaSelection) => {
    setStored(sel);
    saveSelection(database.id, sel);
  };

  if (isLoading || !tables.isFetched) return <CanvasSkeleton />;
  if (error || !data) {
    return (
      <div className="erd-state">
        <QueryError error={error} what="diagram" onRetry={() => void refetch()} />
      </div>
    );
  }
  if (data.tables.length === 0 && !filtered) {
    return (
      <div className="erd-state">
        <EmptyState
          icon={<Network size={20} aria-hidden />}
          title="No tables to draw yet"
          hint="Create a table in the data browser and it appears here with its relations."
          action={
            <ButtonLink href={`/data?db=${encodeURIComponent(database.id)}`} variant="secondary" size="sm">
              Open data browser
            </ButtonLink>
          }
        />
      </div>
    );
  }
  return (
    <ERCanvas
      databaseId={database.id}
      databaseName={database.name}
      data={data}
      summary={summary}
      selection={selection}
      onSelection={onSelection}
      shownSchemas={shown}
      loading={isFetching && isPlaceholderData}
    />
  );
}
