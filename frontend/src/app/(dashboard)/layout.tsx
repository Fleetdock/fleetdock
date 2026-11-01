"use client";

import { usePathname } from "next/navigation";
import { useCallback, useEffect, useState, type ReactNode } from "react";

import { CommandPalette } from "@/components/command-palette";
import { Sidebar } from "@/components/sidebar";
import { Topbar } from "@/components/topbar";
import { RefreshCw, WifiOff } from "lucide-react";

import { Button, PageSkeleton, Skeleton } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { clearLegacyToken } from "@/lib/auth";
import { friendlyError } from "@/lib/errors";
import { useChromeHidden } from "@/lib/focus-mode";
import { useMe, useOperationToasts } from "@/lib/hooks";

// OperationToasts subscribes to operation-completion toasts. It renders nothing
// and lives inside the authenticated shell so it only polls when signed in.
function OperationToasts() {
  useOperationToasts();
  return null;
}

export default function DashboardLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const [navOpen, setNavOpen] = useState(false);
  const closeNav = useCallback(() => setNavOpen(false), []);
  // The session is an httpOnly cookie, so ask the API who we are. A 401 sends
  // the browser to /login (see lib/api.ts).
  const me = useMe();
  // Full-screen workspaces (the ER diagram, the data browser in full screen)
  // render without the sidebar and the top bar.
  const chromeHidden = useChromeHidden(pathname);

  useEffect(() => {
    clearLegacyToken();
  }, []);

  // A 401 is already redirecting to /login; anything else is an outage.
  if (
    me.isError &&
    !(me.error instanceof ApiError && me.error.status === 401)
  ) {
    return (
      <div className="flex justify-center items-center p-4 min-h-dvh">
        <div className="w-full max-w-md card empty-state">
          <div className="empty-state-icon" aria-hidden>
            <WifiOff size={22} />
          </div>
          <p className="empty-state-title">
            Can&apos;t reach the Fleetdock API
          </p>
          <p className="empty-state-hint">{friendlyError(me.error)}</p>
          <div className="empty-state-actions">
            <Button variant="primary" icon={<RefreshCw size={14} aria-hidden />} onClick={() => void me.refetch()}>
              Try again
            </Button>
          </div>
        </div>
      </div>
    );
  }
  // While we learn who is signed in, show the shell's outline rather than a
  // blank screen.
  if (!me.data) return <ShellSkeleton />;

  return (
    <div className="app-shell">
      <a href="#main" className="skip-link">
        Skip to content
      </a>
      <OperationToasts />
      <CommandPalette />
      {chromeHidden ? null : <Sidebar open={navOpen} onClose={closeNav} />}
      <div className="app-column">
        {chromeHidden ? null : <Topbar onOpenNav={() => setNavOpen(true)} />}
        <main
          id="main"
          // The Data Browser and the diagram are full-bleed workspaces with
          // their own scrolling panes.
          className={
            chromeHidden
              ? "app-main app-main-wide app-main-focus"
              : pathname === "/data"
                ? "app-main app-main-wide"
                : "app-main"
          }
          tabIndex={-1}
        >
          {children}
        </main>
      </div>
    </div>
  );
}

function ShellSkeleton() {
  return (
    <div className="app-shell" aria-busy="true">
      <aside className="sidebar" aria-hidden>
        <div className="sidebar-brand">
          <Skeleton width={26} height={26} radius={7} />
          <Skeleton
            width="6rem"
            height=".9rem"
            className="sidebar-brand-text"
          />
        </div>
        <div className="sidebar-nav">
          {[3, 2, 2, 2].map((n, g) => (
            <div key={g} className="sidebar-group">
              {Array.from({ length: n }, (_, i) => (
                <div key={i} className="sidebar-link">
                  <Skeleton width={17} height={17} radius={4} />
                  <Skeleton
                    width={`${50 + (((g + i) * 17) % 40)}%`}
                    height=".8rem"
                    className="sidebar-label"
                  />
                </div>
              ))}
            </div>
          ))}
        </div>
      </aside>
      <div className="app-column">
        <div className="topbar" aria-hidden>
          <Skeleton width="8rem" height=".9rem" />
        </div>
        <main className="app-main">
          <PageSkeleton label="Signing you in" />
        </main>
      </div>
    </div>
  );
}
