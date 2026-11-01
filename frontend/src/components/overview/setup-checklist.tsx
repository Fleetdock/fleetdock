"use client";

import { CheckCircle2, Circle, X } from "lucide-react";
import { useSyncExternalStore } from "react";

import { ButtonLink, IconButton } from "@/components/ui";
import { useCan, useCanAny } from "@/lib/hooks";
import type { Overview } from "@/lib/types";

type Step = {
  id: keyof Overview["setup"];
  title: string;
  text: string;
  href: string;
  cta: string;
  /** Who can do it; the step is hidden from everyone else. */
  allowed: boolean;
  optional?: boolean;
};

const DISMISS_KEY = "fleetdock.setup-dismissed";
const listeners = new Set<() => void>();

function readDismissed(): boolean {
  try {
    return localStorage.getItem(DISMISS_KEY) === "1";
  } catch {
    return false;
  }
}

function dismiss() {
  try {
    localStorage.setItem(DISMISS_KEY, "1");
  } catch {
    // Storage blocked: the checklist just comes back next visit.
  }
  listeners.forEach((l) => l());
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

/**
 * SetupChecklist walks a new install through the steps that make Fleetdock
 * useful. It hides itself once every step the user can take is done, or when
 * dismissed.
 */
export function SetupChecklist({ setup }: { setup: Overview["setup"] }) {
  const can = useCan();
  const canAny = useCanAny();
  const dismissed = useSyncExternalStore(subscribe, readDismissed, () => true);

  const steps: Step[] = [
    {
      id: "instances",
      title: "Connect a database server",
      text: "Point Fleetdock at a MySQL, MariaDB or PostgreSQL server — or let it create one. Its databases are found automatically.",
      href: "/databases?connect=1",
      cta: "Connect",
      allowed: canAny("instance:write"),
    },
    {
      id: "destinations",
      title: "Add backup storage",
      text: "An S3-compatible bucket (AWS S3, Cloudflare R2, MinIO…) where backups are kept.",
      href: "/backups/storage",
      cta: "Add storage",
      allowed: can("destination:write"),
    },
    {
      id: "schedules",
      title: "Back up automatically",
      text: "Pick a database and how often to back it up. Old backups are cleaned up for you.",
      href: "/backups/schedules",
      cta: "Create schedule",
      allowed: can("schedule:write"),
    },
    {
      id: "channels",
      title: "Get alerts",
      text: "Be told by email, Slack or webhook when a backup fails or a server goes offline.",
      href: "/settings/notifications",
      cta: "Set up alerts",
      allowed: can("notification:write"),
    },
    {
      id: "servers",
      title: "Connect a machine",
      text: "Install the agent on a server so Fleetdock can create database servers there and back up databases that aren't reachable from outside.",
      href: "/servers",
      cta: "Connect machine",
      allowed: can("server:write"),
      optional: true,
    },
  ];
  const visible = steps.filter((s) => s.allowed);
  const required = visible.filter((s) => !s.optional);
  const doneCount = visible.filter((s) => setup[s.id] > 0).length;
  if (dismissed || visible.length === 0 || required.every((s) => setup[s.id] > 0)) return null;

  return (
    <section className="card setup-checklist" aria-labelledby="setup-title">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h2 id="setup-title" className="font-semibold m-0">
            Get started
          </h2>
          <p className="muted text-sm mt-1 mb-0">
            {doneCount} of {visible.length} done
          </p>
        </div>
        <IconButton label="Hide the setup checklist" onClick={dismiss}>
          <X size={16} aria-hidden />
        </IconButton>
      </div>
      <div className="progress" aria-hidden>
        {/* Width is the share of steps done. */}
        <span style={{ width: `${(doneCount / visible.length) * 100}%` }} />
      </div>
      <ol className="setup-steps">
        {visible.map((s) => {
          const done = setup[s.id] > 0;
          return (
            <li key={s.id} className={done ? "done" : undefined}>
              {done ? (
                <CheckCircle2 size={18} className="setup-icon-done" aria-label="Done" />
              ) : (
                <Circle size={18} className="muted" aria-label="To do" />
              )}
              <div className="flex-1 min-w-0">
                <div className="font-medium">
                  {s.title}
                  {s.optional ? <span className="muted text-sm"> · optional</span> : null}
                </div>
                {!done ? <p className="muted text-sm mt-0.5 mb-0">{s.text}</p> : null}
              </div>
              {!done ? (
                <ButtonLink href={s.href} size="sm">
                  {s.cta}
                </ButtonLink>
              ) : null}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
