"use client";

import { useSyncExternalStore } from "react";

import { formatRelative } from "@/lib/format";

// One shared 30-second clock for every <Time>, instead of a timer each.
let now = Date.now();
const listeners = new Set<() => void>();
let timer: ReturnType<typeof setInterval> | null = null;

function subscribe(cb: () => void) {
  listeners.add(cb);
  if (!timer) {
    timer = setInterval(() => {
      now = Date.now();
      listeners.forEach((l) => l());
    }, 30_000);
  }
  return () => {
    listeners.delete(cb);
    if (listeners.size === 0 && timer) {
      clearInterval(timer);
      timer = null;
    }
  };
}

/**
 * Time shows a timestamp relative to now ("5 min ago"), with the exact local
 * date and time on hover. Relative times are easier to read in tables; the
 * exact one is always one hover away.
 */
export function Time({ value, absolute }: { value: string | number | Date | null | undefined; absolute?: boolean }) {
  const current = useSyncExternalStore(
    subscribe,
    () => now,
    () => now,
  );
  if (value === null || value === undefined || value === "") return <span className="muted">—</span>;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return <span className="muted">—</span>;
  const exact = d.toLocaleString();
  return (
    <time dateTime={d.toISOString()} title={exact}>
      {absolute ? exact : formatRelative(d, current)}
    </time>
  );
}
