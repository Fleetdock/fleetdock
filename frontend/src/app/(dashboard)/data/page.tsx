"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

import { DataBrowserSkeleton } from "@/components/data-browser/skeleton";
import { DataBrowser } from "@/components/data-browser/workspace";

export default function DataBrowserPage() {
  return (
    <Suspense fallback={<DataBrowserSkeleton />}>
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
