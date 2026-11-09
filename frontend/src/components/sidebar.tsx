"use client";

import { BookOpen, ChevronsLeft } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { LogoMark, Wordmark } from "@/components/logo";
import { Tooltip } from "@/components/ui";
import { useCan, useCanAny, useVersion } from "@/lib/hooks";
import { allowed, NAV, NAV_GROUPS, type NavSection } from "@/lib/nav";

/** useVisibleSections returns the sections (and pages) the user may open. */
export function useVisibleSections(): NavSection[] {
  const can = useCan();
  const canAny = useCanAny();
  return NAV.map((s) => ({
    ...s,
    pages: s.pages.filter((p) => allowed(p.requires, can, canAny)),
  })).filter((s) => s.pages.length > 0);
}

/** useActiveSection returns the section the current path belongs to. */
export function useActiveSection(): NavSection | undefined {
  const pathname = usePathname();
  return NAV.find((s) =>
    s.match.some((m) => pathname === m || pathname.startsWith(`${m}/`)),
  );
}

const COLLAPSE_KEY = "fd.sidebar.collapsed";

// useCollapsed remembers the user's choice of a full sidebar or an icon rail.
function useCollapsed(): [boolean, () => void] {
  const [collapsed, setCollapsed] = useState(false);
  useEffect(() => {
    try {
      setCollapsed(localStorage.getItem(COLLAPSE_KEY) === "1");
    } catch {
      /* storage unavailable */
    }
  }, []);
  const toggle = useCallback(() => {
    setCollapsed((c) => {
      try {
        localStorage.setItem(COLLAPSE_KEY, c ? "0" : "1");
      } catch {
        /* storage unavailable */
      }
      return !c;
    });
  }, []);
  return [collapsed, toggle];
}

/**
 * Sidebar lists the sections in four groups. On wide screens it is a full
 * sidebar the user can collapse to an icon rail; on mid-size screens it is
 * always the rail; on phones it is a drawer opened from the topbar and closed
 * by navigating, tapping outside or Esc. In the rail, labels show as tooltips.
 */
export function Sidebar({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const pathname = usePathname();
  const sections = useVisibleSections();
  const [collapsed, toggleCollapsed] = useCollapsed();
  const [rail, setRail] = useState(false);

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

  // Whether labels are hidden right now (user choice, or the medium-width rail),
  // so tooltips only appear when they add something.
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 1200px) and (min-width: 901px)");
    const desk = window.matchMedia("(min-width: 901px)");
    const update = () => setRail(mq.matches || (collapsed && desk.matches));
    update();
    mq.addEventListener("change", update);
    desk.addEventListener("change", update);
    return () => {
      mq.removeEventListener("change", update);
      desk.removeEventListener("change", update);
    };
  }, [collapsed]);

  return (
    <>
      <div
        className={`sidebar-scrim${open ? " open" : ""}`}
        onClick={onClose}
        aria-hidden
      />
      <aside
        className={`sidebar${open ? " open" : ""}${collapsed ? " collapsed" : ""}`}
        aria-label="Sidebar"
      >
        <Link
          href="/dashboard"
          className="sidebar-brand"
          aria-label="Fleetdock overview"
        >
          <span className="sidebar-brand-text">
            <Wordmark height={27} />
          </span>
          <span className="sidebar-brand-mark">
            <LogoMark size={28} />
          </span>
        </Link>
        <nav className="sidebar-nav" aria-label="Main">
          {NAV_GROUPS.map((group) => {
            const items = sections.filter((s) => s.group === group);
            if (items.length === 0) return null;
            return (
              <div
                key={group}
                className="sidebar-group"
                role="group"
                aria-label={group}
              >
                <div className="sidebar-group-label" aria-hidden>
                  {group}
                </div>
                {items.map((s) => {
                  const active = s.match.some(
                    (m) => pathname === m || pathname.startsWith(`${m}/`),
                  );
                  // A section whose landing page the user can't open goes to its
                  // first permitted page instead.
                  const href = s.pages.some((p) => p.href === s.href)
                    ? s.href
                    : s.pages[0].href;
                  return (
                    <Tooltip
                      key={s.label}
                      content={s.label}
                      side="right"
                      describe={false}
                      disabled={!rail}
                    >
                      <Link
                        href={href}
                        className={`sidebar-link w-full${active ? " active" : ""}`}
                        aria-current={active ? "page" : undefined}
                        aria-label={rail ? s.label : undefined}
                      >
                        <s.Icon size={17} aria-hidden />
                        <span className="sidebar-label">{s.label}</span>
                      </Link>
                    </Tooltip>
                  );
                })}
              </div>
            );
          })}
        </nav>
        <div className="sidebar-foot">
          <Tooltip
            content="Documentation"
            side="right"
            describe={false}
            disabled={!rail}
          >
            <a
              href="https://fleetdock.dev/docs"
              target="_blank"
              rel="noreferrer"
              className="w-full sidebar-link"
              aria-label={rail ? "Documentation" : undefined}
            >
              <BookOpen size={16} aria-hidden />
              <span className="sidebar-label">Documentation</span>
            </a>
          </Tooltip>
          <Tooltip
            content="GitHub"
            side="right"
            describe={false}
            disabled={!rail}
          >
            <a
              href="https://github.com/Fleetdock/fleetdock"
              target="_blank"
              rel="noreferrer"
              className="w-full sidebar-link"
              aria-label={rail ? "GitHub" : undefined}
            >
              <GithubMark />
              <span className="sidebar-label">GitHub</span>
            </a>
          </Tooltip>
          <Tooltip
            content={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            side="right"
            describe={false}
            disabled={!rail}
          >
            <button
              type="button"
              className="sidebar-collapse bg-transparent border-0 w-full sidebar-link"
              onClick={toggleCollapsed}
              aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              aria-expanded={!collapsed}
            >
              <ChevronsLeft
                size={16}
                aria-hidden
                className="transition-transform"
              />
              <span className="sidebar-label">Collapse</span>
            </button>
          </Tooltip>
          <VersionLine />
        </div>
      </aside>
    </>
  );
}

// VersionLine names the running build, for support requests and to tell at a
// glance whether an upgrade took.
function VersionLine() {
  const { data } = useVersion();
  if (!data) return null;
  const label = data.version === "dev" ? "development build" : `v${data.version}`;
  return (
    <div className="sidebar-version" title={data.commit ? `Commit ${data.commit}` : undefined}>
      Fleetdock {label}
    </div>
  );
}

// GithubMark is the GitHub logo (lucide no longer ships brand icons).
function GithubMark() {
  return (
    <svg
      width={16}
      height={16}
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden
    >
      <path d="M12 .5a11.5 11.5 0 0 0-3.64 22.41c.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.37-3.88-1.37-.52-1.33-1.28-1.69-1.28-1.69-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.17 1.18a11 11 0 0 1 5.77 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.7 5.4-5.26 5.68.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0 0 12 .5Z" />
    </svg>
  );
}
