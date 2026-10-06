"use client";

import { useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";

import { useVisibleSections } from "@/components/sidebar";
import { EmptyState, Spinner, TabNav } from "@/components/ui";
import { useMe } from "@/lib/hooks";
import { NAV } from "@/lib/nav";

/** SectionLayout puts the section's page tabs (only the permitted ones) above its pages. */
export function SectionLayout({ section, children }: { section: string; children: ReactNode }) {
  const visible = useVisibleSections().find((s) => s.label === section);
  return (
    <>
      <TabNav label={`${section} pages`} items={(visible?.pages ?? []).map((p) => ({ href: p.href, label: p.label, exact: true }))} />
      {children}
    </>
  );
}

/**
 * SectionIndex is a section's bare URL (/access, /settings): it forwards to
 * the first page the user may open, or explains that there is none.
 */
export function SectionIndex({ section }: { section: string }) {
  const router = useRouter();
  const { isLoading } = useMe();
  const visible = useVisibleSections().find((s) => s.label === section);
  const target = visible?.pages[0]?.href;

  useEffect(() => {
    if (target) router.replace(target);
  }, [router, target]);

  if (isLoading || target) {
    return (
      <div className="flex items-center gap-2 muted text-sm">
        <Spinner /> Loading…
      </div>
    );
  }
  const all = NAV.find((s) => s.label === section);
  return (
    <EmptyState
      title={`You don't have access to ${section}`}
      hint={`Ask an administrator for access to ${all?.pages.map((p) => p.label.toLowerCase()).join(" or ") ?? "this section"}.`}
    />
  );
}
