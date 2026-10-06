"use client";

import { useRouter } from "next/navigation";

import { Loader2 } from "lucide-react";

import { Menu } from "@/components/ui";
import { useCanAny, useOperations } from "@/lib/hooks";
import { operationTitle } from "@/lib/operations";

/**
 * RunningOps shows how many tasks are in progress, with a dropdown linking to
 * each. It shares the operations query the toasts already poll.
 */
export function RunningOps() {
  const router = useRouter();
  const canAny = useCanAny();
  const { data } = useOperations({ page: 1, enabled: canAny("operation:read") });
  const running = (data?.items ?? []).filter((op) => op.status === "running" || op.status === "pending");
  if (running.length === 0) return null;
  return (
    <Menu
      label={`${running.length} task${running.length === 1 ? "" : "s"} in progress`}
      trigger={
        <span className="running-ops">
          <Loader2 size={15} className="spin-icon" aria-hidden />
          <span>{running.length} running</span>
        </span>
      }
      items={[
        ...running.slice(0, 8).map((op) => ({
          label: (
            <span className="flex items-center justify-between gap-3" style={{ width: "100%" }}>
              <span className="truncate">{operationTitle(op)}</span>
              {op.status === "running" && op.progress > 0 ? <span className="muted text-sm">{op.progress}%</span> : null}
            </span>
          ),
          onSelect: () => router.push(`/activity/${op.id}`),
        })),
        { label: <span className="link">View all activity</span>, onSelect: () => router.push("/activity") },
      ]}
    />
  );
}
