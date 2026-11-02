"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

import { DataBrowser } from "@/components/data-browser/workspace";
import { Skeleton } from "@/components/ui";

export default function DataBrowserPage() {
  return (
    <Suspense
      fallback={
        <div role="status" aria-label="Loading Data Browser" className="flex gap-3 p-4">
          <Skeleton width="15rem" height="70vh" radius="var(--radius)" className="hide-sm shrink-0" />
          <Skeleton height="70vh" radius="var(--radius)" className="flex-1" />
        </div>
      }
    >
      <DataBrowserRoute />
    </Suspense>
  );
}

function DataBrowserRoute() {
  const sp = useSearchParams();
  return (
    <DataBrowser
      databaseId={sp.get("db")}
      tableParam={sp.get("table")}
      viewParam={sp.get("view")}
      queryParam={sp.get("query")}
      savedParam={sp.get("saved")}
    />
  );
}
