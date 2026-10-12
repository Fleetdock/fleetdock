"use client";

import { usePathname } from "next/navigation";
import { useCallback, useEffect, useState, type ReactNode } from "react";

import { CommandPalette } from "@/components/command-palette";
import { Sidebar } from "@/components/sidebar";
import { Topbar } from "@/components/topbar";
import { ApiError } from "@/lib/api";
import { clearLegacyToken } from "@/lib/auth";
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

  useEffect(() => {
    clearLegacyToken();
  }, []);

  // A 401 is already redirecting to /login; anything else is an outage.
  if (me.isError && !(me.error instanceof ApiError && me.error.status === 401)) {
    return (
      <div className="flex items-center justify-center" style={{ minHeight: "100vh", padding: "1rem" }}>
        <div className="callout callout-danger" role="alert">
          Could not reach the Fleetdock API.{" "}
          <button className="btn btn-sm" onClick={() => void me.refetch()}>
            Retry
          </button>
        </div>
      </div>
    );
  }
  if (!me.data) return null;

  return (
    <div className="app-shell">
      <a href="#main" className="skip-link">
        Skip to content
      </a>
      <OperationToasts />
      <CommandPalette />
      <Sidebar open={navOpen} onClose={closeNav} />
      <div className="flex flex-col" style={{ flex: 1, minWidth: 0 }}>
        <Topbar onOpenNav={() => setNavOpen(true)} />
        <main
          id="main"
          // The Data Browser is a full-bleed workspace with its own scrolling panes.
          className={pathname === "/data" ? "app-main app-main-wide" : "app-main"}
          tabIndex={-1}
        >
          {children}
        </main>
      </div>
    </div>
  );
}
