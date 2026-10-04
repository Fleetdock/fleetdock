"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState, type ReactNode } from "react";

import { CommandPalette } from "@/components/command-palette";
import { Sidebar } from "@/components/sidebar";
import { Topbar } from "@/components/topbar";
import { getToken } from "@/lib/auth";
import { useOperationToasts } from "@/lib/hooks";

// OperationToasts subscribes to operation-completion toasts. It renders nothing
// and lives inside the authenticated shell so it only polls when signed in.
function OperationToasts() {
  useOperationToasts();
  return null;
}

export default function DashboardLayout({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [ready, setReady] = useState(false);
  const [navOpen, setNavOpen] = useState(false);
  const closeNav = useCallback(() => setNavOpen(false), []);

  useEffect(() => {
    if (!getToken()) {
      router.replace("/login");
    } else {
      setReady(true);
    }
  }, [router]);

  if (!ready) return null;

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
        <main id="main" className="app-main" tabIndex={-1}>
          {children}
        </main>
      </div>
    </div>
  );
}
