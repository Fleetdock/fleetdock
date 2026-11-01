"use client";

import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { SectionLayout } from "@/components/section-tabs";

// Tabs (Tasks, Audit log) on the two lists; a task's own page has none.
export default function ActivityLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (pathname !== "/activity" && pathname !== "/activity/audit") return children;
  return <SectionLayout section="Activity">{children}</SectionLayout>;
}
