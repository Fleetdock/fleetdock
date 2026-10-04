"use client";

import { useMemo, useState } from "react";

import { EmptyState, ErrorText, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useDBObjects } from "@/lib/hooks";
import type { DBObject } from "@/lib/types";
import { ChevronDown, ChevronRight } from "lucide-react";

const LABELS: Record<DBObject["kind"], string> = {
  view: "Views",
  materialized_view: "Materialized views",
  function: "Functions",
  procedure: "Procedures",
  trigger: "Triggers",
  sequence: "Sequences",
  event: "Events",
};

/** DBObjects lists the database's views, routines, triggers, sequences and events. */
export function DBObjects({ databaseId }: { databaseId: string }) {
  const [open, setOpen] = useState(false);
  const { data, isLoading, error } = useDBObjects(open ? databaseId : "");
  const [expanded, setExpanded] = useState<string | null>(null);

  const groups = useMemo(() => {
    const by = new Map<DBObject["kind"], DBObject[]>();
    for (const o of data ?? []) by.set(o.kind, [...(by.get(o.kind) ?? []), o]);
    return [...by.entries()];
  }, [data]);

  return (
    <div style={{ marginTop: "1.25rem" }}>
      <button className="btn btn-ghost btn-sm" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        {open ? <ChevronDown size={15} /> : <ChevronRight size={15} />} Views, routines &amp; triggers
      </button>
      {!open ? null : isLoading ? (
        <div className="flex items-center gap-2 text-sm muted">
          <Spinner /> Loading…
        </div>
      ) : error ? (
        <ErrorText message={error instanceof ApiError ? error.message : "Could not load objects"} />
      ) : groups.length === 0 ? (
        <EmptyState title="No views, routines, triggers or sequences" />
      ) : (
        groups.map(([kind, items]) => (
          <div key={kind} style={{ marginTop: ".6rem" }}>
            <p className="text-sm muted" style={{ marginBottom: ".3rem" }}>
              {LABELS[kind]} ({items.length})
            </p>
            <div className="card">
              <table className="table" style={{ fontSize: 13 }}>
                <tbody>
                  {items.map((o) => {
                    const key = `${o.kind}:${o.schema}.${o.name}`;
                    const isOpen = expanded === key;
                    return (
                      <tr key={key}>
                        <td>
                          <button
                            style={{ all: "unset", cursor: o.definition ? "pointer" : "default" }}
                            onClick={() => o.definition && setExpanded(isOpen ? null : key)}
                          >
                            <span className="font-medium">{o.name}</span>
                            {o.table ? <span className="muted"> on {o.table}</span> : null}
                            <span className="muted"> · {o.schema}</span>
                          </button>
                          {isOpen && o.definition ? (
                            <pre style={{ marginTop: ".5rem", fontSize: 12, whiteSpace: "pre-wrap" }}>
                              <code>{o.definition}</code>
                            </pre>
                          ) : null}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        ))
      )}
    </div>
  );
}
