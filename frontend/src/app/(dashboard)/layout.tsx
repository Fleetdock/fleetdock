"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";

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

  useEffect(() => {
    if (!getToken()) {
      router.replace("/login");
    } else {
      setReady(true);
    }
  }, [router]);

  if (!ready) return null;

  return (
    <div className="flex" style={{ minHeight: "100vh" }}>
      <OperationToasts />
      <Sidebar />
      <div className="flex flex-col" style={{ flex: 1, minWidth: 0 }}>
        <Topbar />
        <main style={{ padding: "1.5rem", width: "100%", maxWidth: "72rem", margin: "0 auto" }}>
          {children}
        </main>
      </div>
    </div>
  );
}
