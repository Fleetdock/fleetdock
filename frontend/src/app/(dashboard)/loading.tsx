"use client";

import { usePathname } from "next/navigation";

import { DataBrowserSkeleton } from "@/components/data-browser/skeleton";
import { RouteLoading } from "@/components/page-transition";
import { PageSkeleton } from "@/components/ui";

// Shown while a page's code loads on navigation; pages show their own
// skeletons while their data loads. The Data Browser gets one of its own
// layout, and RouteLoading fades the page in over either.
export default function DashboardLoading() {
  const pathname = usePathname();
  return (
    <RouteLoading>{pathname === "/data" ? <DataBrowserSkeleton /> : <PageSkeleton />}</RouteLoading>
  );
}