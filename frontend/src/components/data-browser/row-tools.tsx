"use client";

import { useState, type FormEvent } from "react";

import { Button, ErrorText, Field, IconButton, Input, Modal, Select } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useRowMutations } from "@/lib/hooks";
import type { BrowseColumn, BrowseResult, FilterOp, RowFilter } from "@/lib/types";
import { Plus, X } from "lucide-react";

import { arityOf, OPS } from "@/lib/data-browser/filters";

// Row filtering, CSV import and the row-range label for Data Browser table tabs.

export { OPS, needsValue } from "@/lib/data-browser/filters";

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
  focus,
}: {
  columns: BrowseColumn[];
  filters: RowFilter[];
  onChange: (f: RowFilter[]) => void;
  onApply: (e?: FormEvent) => void;
  /** Index of the filter to put the cursor in (a chip that was clicked, a column picked from its header). */
  focus?: number;
}) {
  const set = (i: number, patch: Partial<RowFilter>) =>
    onChange(filters.map((f, j) => (j === i ? { ...f, ...patch } : f)));
  const setOp = (i: number, op: FilterOp) => {
    const f = filters[i];
    const a = arityOf(op);
    // Carry the typed operand across operators where it still makes sense.
    const text = f.value ?? f.values?.[0] ?? "";
    set(i, a === "range" ? { op, value: undefined, values: [text, f.values?.[1] ?? ""] } : { op, value: text, values: undefined });
  };
  return (
    <form className="card p-[.7rem] mb-[.6rem]" onSubmit={onApply}>
      {filters.map((f, i) => {
        const a = arityOf(f.op);
        return (
          <div key={i} className="flex flex-wrap items-center gap-2 mb-[.4rem]">
            <Select className="w-50" aria-label="Column" value={f.column} onChange={(e) => set(i, { column: e.target.value })}>
              {columns.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name}
                </option>
              ))}
            </Select>
            <Select className="w-40" aria-label="Condition" value={f.op} onChange={(e) => setOp(i, e.target.value as FilterOp)}>
              {OPS.map((o) => (
                <option key={o.op} value={o.op}>
                  {o.label}
                </option>
              ))}
            </Select>
            {a === "one" ? (
              <Input
                className="flex-[1_1_180px]"
                aria-label="Value"
                value={f.value ?? ""}
                autoFocus={focus === i}
                onChange={(e) => set(i, { value: e.target.value })}
                placeholder="value"
              />
            ) : a === "list" ? (
              <Input
                className="flex-[1_1_220px]"
                aria-label="Values, separated by commas"
                value={f.value ?? ""}
                autoFocus={focus === i}
                onChange={(e) => set(i, { value: e.target.value })}
                placeholder="a, b, c"
              />
            ) : a === "range" ? (
              <span className="flex items-center gap-1.5 flex-[1_1_220px]">
                <Input
                  aria-label="From"
                  value={f.values?.[0] ?? ""}
                  autoFocus={focus === i}
                  onChange={(e) => set(i, { values: [e.target.value, f.values?.[1] ?? ""] })}
                  placeholder="from"
                />
                <span className="muted text-sm">and</span>
                <Input
                  aria-label="To"
                  value={f.values?.[1] ?? ""}
                  onChange={(e) => set(i, { values: [f.values?.[0] ?? "", e.target.value] })}
                  placeholder="to"
                />
              </span>
            ) : null}
            <IconButton label="Remove filter" onClick={() => onChange(filters.filter((_, j) => j !== i))}>
              <X size={14} aria-hidden />
            </IconButton>
          </div>
        );
      })}
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          icon={<Plus size={14} aria-hidden />}
          onClick={() => onChange([...filters, { column: columns[0]?.name ?? "", op: "eq", value: "" }])}
        >
          Add filter
        </Button>
        <Button type="submit" size="sm" variant="primary">
          Apply
        </Button>
        <span className="muted text-xs hide-sm">All filters must match. Lists take commas or one value per line.</span>
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
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        </>
      ) : (
        <form onSubmit={onSubmit}>
          <p className="text-sm muted mt-0">
            The first line must name the table&apos;s columns; omitted columns take their defaults. The whole file is
            loaded in one transaction — if any row fails, nothing is imported. A field of exactly <code>\N</code> is
            NULL. Max 100 MB.
          </p>
          <Field label="CSV file">
            <Input type="file" accept=".csv,text/csv" onChange={(e) => setFile(e.target.files?.[0] ?? null)} required />
          </Field>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={emptyAsNull} onChange={(e) => setEmptyAsNull(e.target.checked)} />
            Treat empty fields as NULL
          </label>
          <ErrorText message={error ?? undefined} />
          <div className="flex gap-2 mt-3">
            <Button variant="primary" type="submit" disabled={!file} loading={importCSV.isPending}>
              {importCSV.isPending ? "Importing…" : "Import"}
            </Button>
            <Button onClick={onClose} disabled={importCSV.isPending}>
              Cancel
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
