"use client";

import { usePathname } from "next/navigation";

import { DataBrowserSkeleton } from "@/components/data-browser/skeleton";
import { PageSkeleton } from "@/components/ui";

// Shown while a page's code loads on navigation; pages show their own
// skeletons while their data loads. The Data Browser gets one of its own layout.
export default function DashboardLoading() {
  const pathname = usePathname();
  return pathname === "/data" ? <DataBrowserSkeleton /> : <PageSkeleton />;
}
