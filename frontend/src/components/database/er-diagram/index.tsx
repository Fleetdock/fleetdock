"use client";

import { Network } from "lucide-react";
import dynamic from "next/dynamic";

import { ButtonLink, EmptyState, QueryError, Skeleton } from "@/components/ui";
import { useDiagram } from "@/lib/hooks";
import type { Database } from "@/lib/types";

function CanvasSkeleton() {
  return (
    <div role="status" aria-label="Loading diagram" className="stack-sm">
      <Skeleton height="2.25rem" width="60%" radius="var(--radius-md)" />
      <Skeleton height="min(70vh, 42rem)" radius="var(--radius)" />
    </div>
  );
}

// React Flow, ELK and the image exporter only load when the tab opens.
const ERCanvas = dynamic(() => import("./canvas"), { ssr: false, loading: CanvasSkeleton });

/**
 * ERDiagram is the database page's Diagram tab: every table with its
 * columns and keys, and the foreign keys between them.
 */
export function ERDiagram({ database }: { database: Database }) {
  const { data, isLoading, error, refetch } = useDiagram(database.id);
  if (isLoading) return <CanvasSkeleton />;
  if (error || !data) {
    return <QueryError error={error} what="diagram" onRetry={() => void refetch()} />;
  }
  if (data.tables.length === 0) {
    return (
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
    );
  }
  return <ERCanvas databaseId={database.id} databaseName={database.name} data={data} />;
}
