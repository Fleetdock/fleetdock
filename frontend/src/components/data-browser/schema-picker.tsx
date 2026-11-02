"use client";

import { ChevronDown, Layers, Search } from "lucide-react";
import { useState } from "react";

import { Input, cx } from "@/components/ui";
import { compactCount } from "@/lib/data-browser/cells";
import {
  isSelected,
  selectedSchemas,
  selectionLabel,
  toggleSchema,
  type SchemaSelection,
  type SchemaSummary,
} from "@/lib/data-browser/schemas";

import { Popover } from "./popover";

/**
 * SchemaPicker chooses which PostgreSQL schemas to show. Ticking adds or
 * removes a schema; "Only" shows that schema alone; "All schemas" shows
 * everything, including schemas created later.
 */
export function SchemaPicker({
  summary,
  selection,
  onChange,
  className,
  align = "left",
}: {
  summary: SchemaSummary[];
  selection: SchemaSelection;
  onChange: (sel: SchemaSelection) => void;
  className?: string;
  align?: "left" | "right";
}) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const shown = selectedSchemas(selection, summary).length;
  const term = q.trim().toLowerCase();
  const list = term ? summary.filter((s) => s.name.toLowerCase().includes(term)) : summary;
  const all = selection.all || shown === summary.length;

  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      label="Schemas to show"
      align={align}
      className="dbx-schema-pop"
      trigger={
        <button
          type="button"
          className={cx("dbx-schema-button", className)}
          aria-expanded={open}
          aria-haspopup="dialog"
          onClick={() => setOpen((v) => !v)}
          title="Choose the schemas to show"
        >
          <Layers size={14} aria-hidden className="muted" />
          <span className="truncate flex-1 text-left">{selectionLabel(selection, summary)}</span>
          {all ? null : (
            <span className="dbx-schema-count">
              {shown} of {summary.length}
            </span>
          )}
          <ChevronDown size={14} aria-hidden className="muted" />
        </button>
      }
    >
      {summary.length > 8 ? (
        <div className="dbx-search mb-1.5">
          <Search size={14} aria-hidden />
          <Input size="sm" placeholder="Find a schema…" value={q} autoFocus onChange={(e) => setQ(e.target.value)} aria-label="Find a schema" />
        </div>
      ) : null}
      <label className="dbx-columns-item font-medium">
        <input type="checkbox" checked={all} disabled={all} onChange={() => onChange({ all: true })} />
        <span className="flex-1">All schemas</span>
      </label>
      <ul className="dbx-columns-list dbx-schema-list">
        {list.map((s) => {
          const on = isSelected(selection, s.name);
          return (
            <li key={s.name} className="dbx-schema-row">
              <label className="dbx-columns-item">
                <input
                  type="checkbox"
                  checked={on}
                  disabled={on && shown === 1}
                  onChange={() => onChange(toggleSchema(selection, s.name, summary))}
                />
                <span className="truncate">{s.name}</span>
                <span className="muted text-xs ml-auto num">
                  {compactCount(s.tables)}
                  {s.views ? ` + ${compactCount(s.views)}` : ""}
                </span>
              </label>
              <button
                type="button"
                className="dbx-schema-only"
                onClick={() => {
                  onChange({ all: false, schemas: [s.name] });
                  setOpen(false);
                }}
                aria-label={`Show only ${s.name}`}
              >
                Only
              </button>
            </li>
          );
        })}
        {list.length === 0 ? <li className="muted text-sm p-2">No schema matches.</li> : null}
      </ul>
      <div className="dbx-columns-foot">
        <span className="muted text-xs">Tables + views per schema</span>
        <span className="muted text-xs">
          {shown} of {summary.length} shown
        </span>
      </div>
    </Popover>
  );
}
