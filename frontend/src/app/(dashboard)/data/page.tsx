"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

import { DataBrowser } from "@/components/data-browser/workspace";
import { Spinner } from "@/components/ui";

export default function DataBrowserPage() {
  return (
    <Suspense
      fallback={
        <div className="flex items-center gap-2 text-sm muted" style={{ padding: "1.5rem" }}>
          <Spinner /> Loading…
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
    />
  );
}
