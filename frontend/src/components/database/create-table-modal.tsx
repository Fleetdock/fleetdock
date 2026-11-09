"use client";

import { useState, type FormEvent } from "react";

import { ColumnFields, emptyColumn } from "@/components/database/column-editor";
import { Button, ErrorText, Field, IconButton, Input, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import type { Dialect } from "@/lib/data-browser/cells";
import { useStructureMutations } from "@/lib/hooks";
import type { ColumnSpec } from "@/lib/types";
import { Plus, X } from "lucide-react";

/** CreateTableModal builds a table from typed column specs (no SQL). */
export function CreateTableModal({
  databaseId,
  schema,
  onClose,
  onCreated,
  dialect = schema ? "postgres" : "mysql",
}: {
  databaseId: string;
  /** Engine of the database; picks the column types offered. */
  dialect?: Dialect;
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
          <Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        </Field>
        <p className="text-sm muted mt-1 mb-1.5">
          Columns
        </p>
        <div className="max-h-[45vh] overflow-y-auto">
          {columns.map((c, i) => (
            <div key={i} className="flex items-start gap-2">
              <div className="flex-1">
                <ColumnFields
                  compact
                  dialect={dialect}
                  value={c}
                  onChange={(next) => {
                    setColumns((prev) => prev.map((x, j) => (j === i ? next : x)));
                    if (next.name !== c.name) setPk((prev) => prev.map((p) => (p === c.name ? next.name : p)));
                  }}
                />
              </div>
              <label className="flex items-center gap-1 text-sm mt-2" title="Part of the primary key">
                <input
                  type="checkbox"
                  checked={pk.includes(c.name) && c.name !== ""}
                  onChange={(e) => setPk((prev) => (e.target.checked ? [...prev, c.name] : prev.filter((p) => p !== c.name)))}
                />
                PK
              </label>
              <IconButton
                label="Remove column"
                disabled={columns.length === 1}
                onClick={() => {
                  setColumns((prev) => prev.filter((_, j) => j !== i));
                  setPk((prev) => prev.filter((p) => p !== c.name));
                }}
              >
                <X size={14} aria-hidden />
              </IconButton>
            </div>
          ))}
        </div>
        <Button size="sm" icon={<Plus size={14} aria-hidden />} onClick={() => setColumns((prev) => [...prev, emptyColumn()])}>
          Add column
        </Button>
        {pk.length === 0 ? (
          <p className="text-sm muted">
            Without a primary key, rows of this table can&apos;t be edited one by one in the data browser.
          </p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose} disabled={createTable.isPending}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={createTable.isPending}>
            {createTable.isPending ? "Creating…" : "Create table"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
