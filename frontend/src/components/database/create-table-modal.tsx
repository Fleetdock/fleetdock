"use client";

import { useState, type FormEvent } from "react";

import { ColumnFields, emptyColumn } from "@/components/database/column-editor";
import { ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useStructureMutations } from "@/lib/hooks";
import type { ColumnSpec } from "@/lib/types";
import { Plus, X } from "lucide-react";

/** CreateTableModal builds a table from typed column specs (no SQL). */
export function CreateTableModal({
  databaseId,
  schema,
  onClose,
  onCreated,
}: {
  databaseId: string;
  /** PostgreSQL schema to create the table in; null on MySQL/MariaDB. */
  schema: string | null;
  onClose: () => void;
  onCreated: (table: string) => void;
}) {
  const { createTable } = useStructureMutations(databaseId);
  const [name, setName] = useState("");
  const [columns, setColumns] = useState<ColumnSpec[]>([
    { name: "id", type: "bigint", nullable: false, default: null, auto_increment: true },
    emptyColumn(),
  ]);
  const [pk, setPk] = useState<string[]>(["id"]);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const qualified = schema ? `${schema}.${name}` : name;
    try {
      await createTable.mutateAsync({
        name: qualified,
        columns,
        primary_key: pk.filter((c) => columns.some((x) => x.name === c)),
      });
      onClose();
      onCreated(qualified);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create the table");
    }
  }

  return (
    <Modal open onClose={onClose} title={schema ? `Create table in ${schema}` : "Create table"}>
      <form onSubmit={onSubmit}>
        <Field label="Table name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        </Field>
        <p className="text-sm muted" style={{ margin: ".2rem 0 .4rem" }}>
          Columns
        </p>
        <div style={{ maxHeight: "45vh", overflowY: "auto" }}>
          {columns.map((c, i) => (
            <div key={i} className="flex items-start gap-2">
              <div style={{ flex: 1 }}>
                <ColumnFields
                  compact
                  value={c}
                  onChange={(next) => {
                    setColumns((prev) => prev.map((x, j) => (j === i ? next : x)));
                    if (next.name !== c.name) setPk((prev) => prev.map((p) => (p === c.name ? next.name : p)));
                  }}
                />
              </div>
              <label className="flex items-center gap-1 text-sm" style={{ marginTop: ".45rem" }} title="Part of the primary key">
                <input
                  type="checkbox"
                  checked={pk.includes(c.name) && c.name !== ""}
                  onChange={(e) => setPk((prev) => (e.target.checked ? [...prev, c.name] : prev.filter((p) => p !== c.name)))}
                />
                PK
              </label>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                aria-label="Remove column"
                disabled={columns.length === 1}
                onClick={() => {
                  setColumns((prev) => prev.filter((_, j) => j !== i));
                  setPk((prev) => prev.filter((p) => p !== c.name));
                }}
              >
                <X size={14} />
              </button>
            </div>
          ))}
        </div>
        <button type="button" className="btn btn-sm" onClick={() => setColumns((prev) => [...prev, emptyColumn()])}>
          <Plus size={14} /> Add column
        </button>
        {pk.length === 0 ? (
          <p className="text-sm muted">
            Without a primary key, rows of this table can&apos;t be edited one by one in the data browser.
          </p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
          <button className="btn btn-primary" type="submit" disabled={createTable.isPending}>
            {createTable.isPending ? "Creating…" : "Create table"}
          </button>
          <button className="btn" type="button" onClick={onClose} disabled={createTable.isPending}>
            Cancel
          </button>
        </div>
      </form>
    </Modal>
  );
}
