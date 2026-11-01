"use client";

import { Network } from "lucide-react";
import dynamic from "next/dynamic";

import { ButtonLink, EmptyState, QueryError, Skeleton } from "@/components/ui";
import { useDiagram } from "@/lib/hooks";
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
  const { data, isLoading, error, refetch } = useDiagram(database.id);
  if (isLoading) return <CanvasSkeleton />;
  if (error || !data) {
    return (
      <div className="erd-state">
        <QueryError error={error} what="diagram" onRetry={() => void refetch()} />
      </div>
    );
  }
  if (data.tables.length === 0) {
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
  return <ERCanvas databaseId={database.id} databaseName={database.name} data={data} />;
}
