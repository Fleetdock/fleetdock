"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect } from "react";

import { Logo } from "@/components/logo";
import { useCan, useCanAny } from "@/lib/hooks";
import { allowed, NAV, type NavSection } from "@/lib/nav";

/** useVisibleSections returns the sections (and pages) the user may open. */
export function useVisibleSections(): NavSection[] {
  const can = useCan();
  const canAny = useCanAny();
  return NAV.map((s) => ({ ...s, pages: s.pages.filter((p) => allowed(p.requires, can, canAny)) })).filter(
    (s) => s.pages.length > 0,
  );
}

/**
 * Sidebar lists the seven sections. On wide screens it is always visible; on
 * mid-size screens it shrinks to an icon rail; on phones it is a drawer opened
 * from the topbar and closed by navigating, tapping outside or Esc.
 */
export function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const pathname = usePathname();
  const sections = useVisibleSections();

  // Close the drawer whenever the route changes.
  useEffect(() => {
    onClose();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  return (
    <>
      <div className={`sidebar-scrim${open ? " open" : ""}`} onClick={onClose} aria-hidden />
      <aside className={`sidebar${open ? " open" : ""}`}>
        <Link
          href="/dashboard"
          className="flex items-center gap-2"
          style={{ padding: "1.05rem 1rem", borderBottom: "1px solid var(--border)" }}
        >
          <Logo size={26} />
          <div className="sidebar-brand-text">
            <div className="font-semibold text-sm">Fleetdock</div>
            <div className="muted" style={{ fontSize: 11 }}>
              Database manager
            </div>
          </div>
        </Link>
        <nav className="flex flex-col gap-1" style={{ padding: ".6rem" }} aria-label="Main">
          {sections.map((s) => {
            const active = s.match.some((m) => pathname === m || pathname.startsWith(`${m}/`));
            // A section whose landing page the user can't open goes to its
            // first permitted page instead.
            const href = s.pages.some((p) => p.href === s.href) ? s.href : s.pages[0].href;
            return (
              <Link
                key={s.label}
                href={href}
                className={`sidebar-link${active ? " active" : ""}`}
                aria-current={active ? "page" : undefined}
                title={s.label}
              >
                <s.Icon size={17} aria-hidden />
                <span className="sidebar-label">{s.label}</span>
              </Link>
            );
          })}
        </nav>
      </aside>
    </>
  );
}
