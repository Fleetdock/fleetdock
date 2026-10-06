"use client";

import { useEffect, useMemo, useRef, useState } from "react";

import { Spinner } from "@/components/ui";
import { recentDatabases } from "@/lib/data-browser/storage";
import { engineLabel } from "@/lib/engines";
import { useDatabases } from "@/lib/hooks";
import type { Database } from "@/lib/types";
import { Check, ChevronsUpDown, Clock, Database as DatabaseIcon, Search } from "lucide-react";

type Group = { key: string; label: string; engine?: string; items: Database[] };

/** EngineMark is a small engine-coloured tag ("PG", "My", "Ma"). */
export function EngineMark({ engine }: { engine?: string }) {
  const short = engine === "postgres" ? "PG" : engine === "mariadb" ? "Ma" : engine === "mysql" ? "My" : "DB";
  return (
    <span className={`dbx-engine dbx-engine-${engine ?? "other"}`} title={engine ? engineLabel(engine) : undefined}>
      {short}
    </span>
  );
}

/** useDatabaseGroups lists databases grouped by their server, recent ones first. */
function useDatabaseGroups(search: string) {
  const [debounced, setDebounced] = useState(search);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(search.trim()), 200);
    return () => clearTimeout(t);
  }, [search]);
  const { data, isLoading, error } = useDatabases({ search: debounced || undefined });

  const groups = useMemo(() => {
    const items = data?.items ?? [];
    const out: Group[] = [];
    if (!debounced) {
      const recent = recentDatabases()
        .map((id) => items.find((d) => d.id === id))
        .filter((d): d is Database => Boolean(d));
      if (recent.length > 1) out.push({ key: "recent", label: "Recent", items: recent });
    }
    const by = new Map<string, Group>();
    for (const d of items) {
      const key = d.instance_id;
      let g = by.get(key);
      if (!g) {
        g = { key, label: d.instance?.name ?? d.instance_id.slice(0, 8), engine: d.instance?.engine, items: [] };
        by.set(key, g);
      }
      g.items.push(d);
    }
    for (const g of [...by.values()].sort((a, b) => a.label.localeCompare(b.label))) {
      // User databases first, the engine's own (postgres, mysql, sys) last.
      g.items.sort((a, b) => Number(a.system) - Number(b.system) || a.name.localeCompare(b.name));
      out.push(g);
    }
    return out;
  }, [data, debounced]);

  return { groups, isLoading, error: error as Error | null, total: data?.items.length ?? 0 };
}

function DatabaseOptions({
  search,
  currentId,
  onPick,
  cursor,
  setCursor,
  onCount,
}: {
  search: string;
  currentId?: string;
  onPick: (id: string) => void;
  cursor: number;
  setCursor: (i: number) => void;
  onCount: (ids: string[]) => void;
}) {
  const { groups, isLoading, error } = useDatabaseGroups(search);
  const flat = useMemo(() => groups.flatMap((g) => g.items.map((d) => `${g.key}/${d.id}`)), [groups]);
  useEffect(() => onCount(flat), [flat, onCount]);
  const listRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    listRef.current?.querySelector(`[data-index="${cursor}"]`)?.scrollIntoView({ block: "nearest" });
  }, [cursor]);

  if (isLoading && groups.length === 0) {
    return (
      <div className="dbx-picker-empty">
        <Spinner /> Loading databases…
      </div>
    );
  }
  if (error) return <div className="dbx-picker-empty">Could not load databases: {error.message}</div>;
  if (groups.length === 0) {
    return <div className="dbx-picker-empty">{search ? "No databases match." : "No databases yet."}</div>;
  }
  // Each option's position in the keyboard order across all groups.
  const starts = groups.map((_, gi) => groups.slice(0, gi).reduce((n, g) => n + g.items.length, 0));
  return (
    <div className="dbx-picker-list" ref={listRef} role="listbox" aria-label="Databases">
      {groups.map((g, gi) => (
        <div key={g.key}>
          <div className="dbx-picker-group">
            {g.key === "recent" ? <Clock size={12} /> : <EngineMark engine={g.engine} />}
            <span className="truncate">{g.label}</span>
          </div>
          {g.items.map((d, di) => {
            const idx = starts[gi] + di;
            return (
              <button
                key={`${g.key}/${d.id}`}
                type="button"
                role="option"
                aria-selected={idx === cursor}
                data-index={idx}
                className={`dbx-picker-item${idx === cursor ? " active" : ""}`}
                onMouseMove={() => setCursor(idx)}
                onClick={() => onPick(d.id)}
              >
                <DatabaseIcon size={14} className="muted" />
                <span className="truncate" style={{ flex: 1 }}>
                  {d.name}
                </span>
                {g.key === "recent" ? <span className="muted text-xs truncate">{d.instance?.name}</span> : null}
                {d.system ? <span className="badge badge-gray">system</span> : null}
                {d.id === currentId ? <Check size={14} style={{ color: "var(--accent)" }} /> : null}
              </button>
            );
          })}
        </div>
      ))}
    </div>
  );
}

/** useOptionKeys wires ↑/↓/Enter in a search box to a list of option ids. */
function useOptionKeys(onPick: (id: string) => void) {
  const [cursor, setCursor] = useState(0);
  const ids = useRef<string[]>([]);
  const onCount = useMemo(
    () => (list: string[]) => {
      ids.current = list;
      setCursor((c) => (c >= list.length ? 0 : c));
    },
    [],
  );
  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setCursor((c) => Math.min(c + 1, ids.current.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setCursor((c) => Math.max(c - 1, 0));
    } else if (e.key === "Enter") {
      const id = ids.current[cursor];
      if (id) {
        e.preventDefault();
        onPick(id.split("/")[1]);
      }
    }
  }
  return { cursor, setCursor, onCount, onKeyDown };
}

/**
 * DatabasePicker is the switcher at the top of the Data Browser sidebar: the
 * current database, and a searchable list of all databases by server.
 */
export function DatabasePicker({
  current,
  onPick,
  defaultOpen = false,
}: {
  current?: Database;
  onPick: (id: string) => void;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const [search, setSearch] = useState("");
  const root = useRef<HTMLDivElement>(null);
  const pick = (id: string) => {
    setOpen(false);
    setSearch("");
    onPick(id);
  };
  const keys = useOptionKeys(pick);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  return (
    <div className="dbx-picker" ref={root}>
      <button
        type="button"
        className="dbx-picker-button"
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        title="Switch database"
        aria-label={current ? `Switch database (now ${current.name})` : "Choose a database"}
      >
        <EngineMark engine={current?.instance?.engine} />
        <span className="dbx-picker-label">
          <span className="truncate font-medium">{current?.name ?? "Choose a database"}</span>
          {current?.instance ? <span className="truncate muted text-xs">{current.instance.name}</span> : null}
        </span>
        <ChevronsUpDown size={14} className="muted" />
      </button>
      {open ? (
        <div className="dbx-picker-pop">
          <div className="dbx-search">
            <Search size={14} />
            <input
              className="input"
              autoFocus
              placeholder="Find a database…"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                keys.setCursor(0);
              }}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  e.stopPropagation();
                  setOpen(false);
                  return;
                }
                keys.onKeyDown(e);
              }}
              aria-label="Find a database"
            />
          </div>
          <DatabaseOptions
            search={search}
            currentId={current?.id}
            onPick={pick}
            cursor={keys.cursor}
            setCursor={keys.setCursor}
            onCount={keys.onCount}
          />
        </div>
      ) : null}
    </div>
  );
}

/** DatabaseChooser fills the main area when no database is selected yet. */
export function DatabaseChooser({ onPick }: { onPick: (id: string) => void }) {
  const [search, setSearch] = useState("");
  const keys = useOptionKeys(onPick);
  return (
    <div className="dbx-chooser">
      <div className="dbx-welcome-icon">
        <DatabaseIcon size={22} />
      </div>
      <h2 className="font-semibold" style={{ fontSize: "1.15rem", margin: ".2rem 0" }}>
        Which database do you want to browse?
      </h2>
      <p className="muted text-sm" style={{ margin: "0 0 1rem" }}>
        Its tables and views appear in the sidebar. You can switch at any time from the top of the sidebar.
      </p>
      <div className="card dbx-chooser-card">
        <div className="dbx-search">
          <Search size={14} />
          <input
            className="input"
            autoFocus
            placeholder="Find a database…"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              keys.setCursor(0);
            }}
            onKeyDown={keys.onKeyDown}
            aria-label="Find a database"
          />
        </div>
        <DatabaseOptions
          search={search}
          onPick={onPick}
          cursor={keys.cursor}
          setCursor={keys.setCursor}
          onCount={keys.onCount}
        />
      </div>
    </div>
  );
}
