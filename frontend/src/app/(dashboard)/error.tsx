"use client";

import { AlertTriangle, RefreshCw } from "lucide-react";
import { useEffect } from "react";

import { Button, ButtonLink, EmptyState } from "@/components/ui";

// Shown when a dashboard page throws while rendering. The shell (sidebar,
// topbar) stays usable, so the user can retry or go elsewhere.
export default function DashboardError({
  error,
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);
  return (
    <EmptyState
      icon={<AlertTriangle size={22} />}
      title="This page hit an unexpected error"
      hint={
        <>
          Try again; if it keeps happening, check the Fleetdock logs.
          {error.digest ? (
            <span className="block mt-1 text-xs mono">
              Reference: {error.digest}
            </span>
          ) : null}
        </>
      }
      action={
        <>
          <Button
            variant="primary"
            icon={<RefreshCw size={14} />}
            onClick={() => retry()}
          >
            Try again
          </Button>
          <ButtonLink href="/dashboard">Go to overview</ButtonLink>
        </>
      }
    />
  );
}
