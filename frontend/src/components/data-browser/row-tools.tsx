"use client";

import { useState, type FormEvent } from "react";

import { Button, ErrorText, Field, IconButton, Input, Modal, Select } from "@/components/ui";
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
    <form className="card p-[.7rem] mb-[.6rem]" onSubmit={onApply}>
      {filters.map((f, i) => (
        <div key={i} className="flex flex-wrap items-center gap-2 mb-[.4rem]">
          <Select className="w-50" aria-label="Column" value={f.column} onChange={(e) => set(i, { column: e.target.value })}>
            {columns.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </Select>
          <Select className="w-35" aria-label="Condition" value={f.op} onChange={(e) => set(i, { op: e.target.value as FilterOp })}>
            {OPS.map((o) => (
              <option key={o.op} value={o.op}>
                {o.label}
              </option>
            ))}
          </Select>
          {needsValue(f.op) ? (
            <Input
              className="flex-[1_1_180px]"
              aria-label="Value"
              value={f.value ?? ""}
              onChange={(e) => set(i, { value: e.target.value })}
              placeholder="value"
            />
          ) : null}
          <IconButton label="Remove filter" onClick={() => onChange(filters.filter((_, j) => j !== i))}>
            <X size={14} aria-hidden />
          </IconButton>
        </div>
      ))}
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
