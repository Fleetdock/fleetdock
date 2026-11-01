"use client";

import { Columns3, Search } from "lucide-react";
import { useState } from "react";

import { Button, Input } from "@/components/ui";
import type { BrowseColumn } from "@/lib/types";

import { Popover } from "./popover";

/** ColumnsMenu picks which columns the grid shows. */
export function ColumnsMenu({
  columns,
  hidden,
  onChange,
}: {
  columns: BrowseColumn[];
  hidden: string[];
  onChange: (hidden: string[]) => void;
}) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const h = new Set(hidden.filter((n) => columns.some((c) => c.name === n)));
  const shown = columns.length - h.size;
  const term = q.trim().toLowerCase();
  const list = term ? columns.filter((c) => c.name.toLowerCase().includes(term)) : columns;
  const toggle = (name: string) => {
    const next = new Set(h);
    if (next.has(name)) next.delete(name);
    else if (shown > 1) next.add(name);
    onChange([...next]);
  };
  return (
    <Popover
      open={open}
      onClose={() => setOpen(false)}
      label="Columns to show"
      align="right"
      className="dbx-columns-pop"
      trigger={
        <Button
          size="sm"
          icon={<Columns3 size={14} aria-hidden />}
          active={h.size > 0}
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          disabled={columns.length === 0}
        >
          <span className="hide-sm">Columns</span>
          {h.size ? ` ${shown}/${columns.length}` : ""}
        </Button>
      }
    >
      {columns.length > 8 ? (
        <div className="dbx-search mb-1.5">
          <Search size={14} aria-hidden />
          <Input
            size="sm"
            placeholder="Find a column…"
            value={q}
            autoFocus
            onChange={(e) => setQ(e.target.value)}
            aria-label="Find a column"
          />
        </div>
      ) : null}
      <ul className="dbx-columns-list">
        {list.map((c) => (
          <li key={c.name}>
            <label className="dbx-columns-item">
              <input
                type="checkbox"
                checked={!h.has(c.name)}
                disabled={!h.has(c.name) && shown <= 1}
                onChange={() => toggle(c.name)}
              />
              <span className="truncate">{c.name}</span>
              <span className="muted text-xs truncate ml-auto">{c.type}</span>
            </label>
          </li>
        ))}
        {list.length === 0 ? <li className="muted text-sm p-2">No column matches.</li> : null}
      </ul>
      <div className="dbx-columns-foot">
        <Button size="sm" variant="ghost" disabled={h.size === 0} onClick={() => onChange([])}>
          Show all
        </Button>
        <span className="muted text-xs">
          {shown} of {columns.length} shown
        </span>
      </div>
    </Popover>
  );
}
