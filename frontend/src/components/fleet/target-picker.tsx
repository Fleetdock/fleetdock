"use client";

import { useMemo, useState } from "react";

import { Spinner } from "@/components/ui";
import { engineLabel } from "@/lib/engines";
import { useAllDatabases } from "@/lib/hooks";
import type { Database } from "@/lib/types";

/** globMatch matches a name against a pattern where * is any run of characters. */
export function globMatch(pattern: string, name: string): boolean {
  const p = pattern.trim().toLowerCase();
  if (!p) return true;
  if (!p.includes("*")) return name.toLowerCase().includes(p);
  const re = new RegExp(`^${p.split("*").map((s) => s.replace(/[.+?^${}()|[\]\\]/g, "\\$&")).join(".*")}$`);
  return re.test(name.toLowerCase());
}

/**
 * TargetPicker chooses the databases a fleet query runs on: filtered by
 * server, engine and a name pattern (shop_*), then picked one by one or all
 * at once. System and missing databases are left out.
 */
export function TargetPicker({
  selected,
  onChange,
}: {
  selected: Set<string>;
  onChange: (next: Set<string>) => void;
}) {
  const { data, isLoading } = useAllDatabases();
  const [instance, setInstance] = useState("");
  const [engine, setEngine] = useState("");
  const [pattern, setPattern] = useState("");

  const all = useMemo(() => (data ?? []).filter((d) => !d.system && d.status !== "missing"), [data]);
  const instances = useMemo(() => {
    const m = new Map<string, string>();
    for (const d of all) m.set(d.instance_id, d.instance?.name ?? d.instance_id.slice(0, 8));
    return [...m.entries()].sort((a, b) => a[1].localeCompare(b[1]));
  }, [all]);
  const engines = useMemo(() => [...new Set(all.map((d) => d.instance?.engine ?? ""))].filter(Boolean).sort(), [all]);
  const shown = all.filter(
    (d) =>
      (!instance || d.instance_id === instance) &&
      (!engine || d.instance?.engine === engine) &&
      globMatch(pattern, d.name),
  );
  const allShown = shown.length > 0 && shown.every((d) => selected.has(d.id));

  const toggle = (d: Database) => {
    const next = new Set(selected);
    if (next.has(d.id)) next.delete(d.id);
    else next.add(d.id);
    onChange(next);
  };
  const toggleShown = () => {
    const next = new Set(selected);
    for (const d of shown) {
      if (allShown) next.delete(d.id);
      else next.add(d.id);
    }
    onChange(next);
  };

  if (isLoading) return <Spinner />;
  return (
    <div className="fleet-picker">
      <div className="flex items-center gap-2" style={{ flexWrap: "wrap" }}>
        <input
          className="input"
          style={{ maxWidth: "14rem" }}
          placeholder="Name, or a pattern like shop_*"
          value={pattern}
          onChange={(e) => setPattern(e.target.value)}
          aria-label="Filter by name"
        />
        <select className="input" style={{ maxWidth: "12rem" }} value={instance} onChange={(e) => setInstance(e.target.value)} aria-label="Server">
          <option value="">All servers</option>
          {instances.map(([id, name]) => (
            <option key={id} value={id}>
              {name}
            </option>
          ))}
        </select>
        {engines.length > 1 ? (
          <select className="input" style={{ maxWidth: "10rem" }} value={engine} onChange={(e) => setEngine(e.target.value)} aria-label="Engine">
            <option value="">All engines</option>
            {engines.map((e) => (
              <option key={e} value={e}>
                {engineLabel(e)}
              </option>
            ))}
          </select>
        ) : null}
        <button className="btn btn-sm" onClick={toggleShown} disabled={shown.length === 0}>
          {allShown ? "Clear shown" : `Select ${shown.length} shown`}
        </button>
      </div>
      <div className="card fleet-picker-list">
        {shown.length === 0 ? (
          <p className="text-sm muted" style={{ margin: ".75rem" }}>
            No databases match.
          </p>
        ) : (
          shown.map((d) => (
            <label key={d.id} className="fleet-picker-row">
              <input type="checkbox" checked={selected.has(d.id)} onChange={() => toggle(d)} />
              <span className="font-medium">{d.name}</span>
              <span className="text-sm muted">
                {d.instance?.name ?? ""} · {engineLabel(d.instance?.engine ?? "")}
              </span>
            </label>
          ))
        )}
      </div>
    </div>
  );
}

/** engineMix lists the engines of the selected databases. */
export function engineMix(all: Database[] | undefined, selected: Set<string>): string[] {
  return [...new Set((all ?? []).filter((d) => selected.has(d.id)).map((d) => d.instance?.engine ?? ""))].filter(Boolean);
}
