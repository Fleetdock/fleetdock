"use client";

import { changeLabel } from "@/lib/schema-changes";
import type { SchemaChange } from "@/lib/types";

const OP_BADGE: Record<SchemaChange["op"], string> = {
  added: "badge-green",
  removed: "badge-red",
  changed: "badge-amber",
};

/**
 * SchemaChangeList lists structural differences with their definitions:
 * what was there before and what is there now (or, comparing two
 * databases, what the target has and what the source has).
 */
export function SchemaChangeList({
  changes,
  beforeLabel = "Before",
  afterLabel = "After",
  opLabels,
}: {
  changes: SchemaChange[];
  beforeLabel?: string;
  afterLabel?: string;
  opLabels?: Partial<Record<SchemaChange["op"], string>>;
}) {
  if (changes.length === 0) return <p className="text-sm muted">No structural differences.</p>;
  return (
    <ul className="schema-changes">
      {changes.map((c, i) => (
        <li key={`${c.kind}:${c.schema ?? ""}:${c.table ?? ""}:${c.name ?? ""}:${i}`}>
          <details>
            <summary>
              <span className={`badge ${OP_BADGE[c.op]}`}>{opLabels?.[c.op] ?? c.op}</span>
              <code>{changeLabel(c)}</code>
            </summary>
            <div className="schema-change-defs">
              {c.before ? (
                <div>
                  <div className="text-sm muted">{beforeLabel}</div>
                  <pre className="schema-def is-before">{c.before}</pre>
                </div>
              ) : null}
              {c.after ? (
                <div>
                  <div className="text-sm muted">{afterLabel}</div>
                  <pre className="schema-def is-after">{c.after}</pre>
                </div>
              ) : null}
            </div>
          </details>
        </li>
      ))}
    </ul>
  );
}
