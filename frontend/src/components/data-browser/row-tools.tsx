"use client";

import { useState, type FormEvent } from "react";

import { ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useRowMutations } from "@/lib/hooks";
import type { BrowseColumn, BrowseResult, FilterOp, RowFilter } from "@/lib/types";
import { Plus, X } from "lucide-react";

// Row filtering, CSV import and the row-range label for Data Browser table tabs.

export const OPS: { op: FilterOp; label: string; needsValue: boolean }[] = [
  { op: "eq", label: "=", needsValue: true },
  { op: "ne", label: "≠", needsValue: true },
  { op: "lt", label: "<", needsValue: true },
  { op: "lte", label: "≤", needsValue: true },
  { op: "gt", label: ">", needsValue: true },
  { op: "gte", label: "≥", needsValue: true },
  { op: "contains", label: "contains", needsValue: true },
  { op: "starts_with", label: "starts with", needsValue: true },
  { op: "is_null", label: "is NULL", needsValue: false },
  { op: "not_null", label: "is not NULL", needsValue: false },
];

export const needsValue = (op: FilterOp) => OPS.find((o) => o.op === op)?.needsValue ?? true;

export function RowRange({ data, offset }: { data: BrowseResult; offset: number }) {
  if (data.rows.length === 0) return null;
  const total = data.total > 0 ? (data.total_exact ? data.total.toLocaleString() : `~${data.total.toLocaleString()}`) : "";
  return (
    <span className="text-sm muted">
      rows {offset + 1}–{offset + data.rows.length}
      {total ? ` of ${total}${data.total_capped ? "+" : ""}` : ""}
    </span>
  );
}

export function FilterEditor({
  columns,
  filters,
  onChange,
  onApply,
}: {
  columns: BrowseColumn[];
  filters: RowFilter[];
  onChange: (f: RowFilter[]) => void;
  onApply: (e?: FormEvent) => void;
}) {
  const set = (i: number, patch: Partial<RowFilter>) =>
    onChange(filters.map((f, j) => (j === i ? { ...f, ...patch } : f)));
  return (
    <form className="card" style={{ padding: ".7rem", marginBottom: ".6rem" }} onSubmit={onApply}>
      {filters.map((f, i) => (
        <div key={i} className="flex items-center gap-2" style={{ marginBottom: ".4rem", flexWrap: "wrap" }}>
          <select className="input" style={{ width: 200 }} value={f.column} onChange={(e) => set(i, { column: e.target.value })}>
            {columns.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
          <select className="input" style={{ width: 140 }} value={f.op} onChange={(e) => set(i, { op: e.target.value as FilterOp })}>
            {OPS.map((o) => (
              <option key={o.op} value={o.op}>
                {o.label}
              </option>
            ))}
          </select>
          {needsValue(f.op) ? (
            <input
              className="input"
              style={{ flex: "1 1 180px" }}
              value={f.value ?? ""}
              onChange={(e) => set(i, { value: e.target.value })}
              placeholder="value"
            />
          ) : null}
          <button type="button" className="btn btn-ghost btn-sm" aria-label="Remove filter" onClick={() => onChange(filters.filter((_, j) => j !== i))}>
            <X size={14} />
          </button>
        </div>
      ))}
      <div className="flex items-center gap-2">
        <button
          type="button"
          className="btn btn-sm"
          onClick={() => onChange([...filters, { column: columns[0]?.name ?? "", op: "eq", value: "" }])}
        >
          <Plus size={14} /> Add filter
        </button>
        <button type="submit" className="btn btn-sm btn-primary">
          Apply
        </button>
      </div>
    </form>
  );
}

export function ImportCSV({ databaseId, table, onClose }: { databaseId: string; table: string; onClose: () => void }) {
  const { importCSV } = useRowMutations(databaseId, table);
  const [file, setFile] = useState<File | null>(null);
  const [emptyAsNull, setEmptyAsNull] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<number | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!file) return;
    setError(null);
    try {
      const res = await importCSV.mutateAsync({ file, emptyAsNull });
      setDone(res.imported);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Import failed");
    }
  }

  return (
    <Modal open onClose={onClose} title={`Import CSV into ${table}`}>
      {done !== null ? (
        <>
          <p>
            Imported <strong>{done.toLocaleString()}</strong> row{done === 1 ? "" : "s"}.
          </p>
          <button className="btn btn-primary" onClick={onClose}>
            Done
          </button>
        </>
      ) : (
        <form onSubmit={onSubmit}>
          <p className="text-sm muted" style={{ marginTop: 0 }}>
            The first line must name the table&apos;s columns; omitted columns take their defaults. The whole file is
            loaded in one transaction — if any row fails, nothing is imported. A field of exactly <code>\N</code> is
            NULL. Max 100 MB.
          </p>
          <Field label="CSV file">
            <input className="input" type="file" accept=".csv,text/csv" onChange={(e) => setFile(e.target.files?.[0] ?? null)} required />
          </Field>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={emptyAsNull} onChange={(e) => setEmptyAsNull(e.target.checked)} />
            Treat empty fields as NULL
          </label>
          <ErrorText message={error ?? undefined} />
          <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
            <button className="btn btn-primary" type="submit" disabled={!file || importCSV.isPending}>
              {importCSV.isPending ? "Importing…" : "Import"}
            </button>
            <button className="btn" type="button" onClick={onClose} disabled={importCSV.isPending}>
              Cancel
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
