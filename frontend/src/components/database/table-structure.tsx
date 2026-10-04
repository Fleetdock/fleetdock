"use client";

import { useState, type FormEvent, type ReactNode } from "react";

import { ColumnFields, emptyColumn } from "@/components/database/column-editor";
import { ConfirmModal, EmptyState, ErrorText, Field, Modal, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useForeignKeys, useStructureMutations, useTableSchema } from "@/lib/hooks";
import type { AlterOp, ColumnInfo, ColumnSpec, ForeignKey, TableSchema } from "@/lib/types";
import { Eraser, Pencil, Plus, TextCursorInput, Trash2 } from "lucide-react";

const errMsg = (err: unknown, fallback: string) => (err instanceof ApiError ? err.message : fallback);

/**
 * SchemaView shows a table's structure — columns, indexes, foreign keys and
 * its DDL — and, for users who may write, edits it through the typed
 * structure API (no SQL is sent from the browser).
 */
export function SchemaView({
  databaseId,
  table,
  canWrite = false,
  onTableGone,
}: {
  databaseId: string;
  table: string;
  canWrite?: boolean;
  /** Called after the table was dropped (null) or renamed (new name). */
  onTableGone?: (renamedTo: string | null) => void;
}) {
  const { data, isLoading, error } = useTableSchema(databaseId, table);
  const fks = useForeignKeys(databaseId, table);
  const [modal, setModal] = useState<ReactNode>(null);
  const close = () => setModal(null);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm muted">
        <Spinner /> Loading structure…
      </div>
    );
  }
  if (error) return <EmptyState title="Could not load structure" hint={(error as ApiError).message} />;
  if (!data) return <EmptyState title="No structure" />;

  const props = { databaseId, table, schema: data, onClose: close };

  return (
    <div className="flex" style={{ flexDirection: "column", gap: "1.1rem" }}>
      {canWrite ? (
        <div className="flex items-center gap-2" style={{ flexWrap: "wrap" }}>
          <button className="btn btn-sm" onClick={() => setModal(<RenameTable {...props} onDone={(n) => onTableGone?.(n)} />)}>
            <TextCursorInput size={15} /> Rename table
          </button>
          <button className="btn btn-sm" onClick={() => setModal(<TruncateTable {...props} />)}>
            <Eraser size={15} /> Truncate
          </button>
          <button className="btn btn-sm btn-danger" onClick={() => setModal(<DropTable {...props} onDone={() => onTableGone?.(null)} />)}>
            <Trash2 size={15} /> Drop table
          </button>
        </div>
      ) : null}

      <Section
        title="Columns"
        action={
          canWrite ? (
            <button className="btn btn-sm" onClick={() => setModal(<ColumnModal {...props} column={null} />)}>
              <Plus size={14} /> Add column
            </button>
          ) : null
        }
      >
        <div className="card" style={{ overflowX: "auto" }}>
          <table className="table" style={{ fontSize: 13, whiteSpace: "nowrap" }}>
            <thead>
              <tr>
                <th>Column</th>
                <th>Type</th>
                <th>Null</th>
                <th>Key</th>
                <th>Default</th>
                <th>Extra</th>
                <th>Comment</th>
                {canWrite ? <th /> : null}
              </tr>
            </thead>
            <tbody>
              {data.columns.map((c) => (
                <tr key={c.name}>
                  <td className="font-medium">{c.name}</td>
                  <td className="muted">
                    <code>{c.type}</code>
                  </td>
                  <td className="muted">{c.nullable ? "YES" : "NO"}</td>
                  <td className="muted">
                    {c.key ? (
                      <span className="badge badge-gray" style={{ fontSize: 11 }}>
                        {c.key}
                      </span>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className={c.default === null ? "muted" : undefined}>{c.default === null ? "NULL" : c.default}</td>
                  <td className="muted">{c.extra || "—"}</td>
                  <td className="muted">{c.comment || "—"}</td>
                  {canWrite ? (
                    <td style={{ textAlign: "right" }}>
                      <button className="btn btn-ghost btn-sm" aria-label={`Edit column ${c.name}`} onClick={() => setModal(<ColumnModal {...props} column={c} />)}>
                        <Pencil size={14} />
                      </button>
                      <button
                        className="btn btn-ghost btn-sm"
                        aria-label={`Drop column ${c.name}`}
                        disabled={data.columns.length === 1}
                        onClick={() =>
                          setModal(
                            <AlterConfirm
                              {...props}
                              title={`Drop column ${c.name}?`}
                              confirmLabel="Drop column"
                              confirmText={c.name}
                              ops={[{ op: "drop_column", name: c.name }]}
                              message={`Every value in "${c.name}" is permanently deleted.`}
                            />,
                          )
                        }
                      >
                        <Trash2 size={14} />
                      </button>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <Section
        title="Indexes"
        action={
          canWrite ? (
            <button className="btn btn-sm" onClick={() => setModal(<IndexModal {...props} />)}>
              <Plus size={14} /> Add index
            </button>
          ) : null
        }
      >
        {data.indexes.length === 0 ? (
          <p className="text-sm muted">No indexes.</p>
        ) : (
          <div className="card" style={{ overflowX: "auto" }}>
            <table className="table" style={{ fontSize: 13, whiteSpace: "nowrap" }}>
              <thead>
                <tr>
                  <th>Index</th>
                  <th>Columns</th>
                  <th>Unique</th>
                  <th>Type</th>
                  {canWrite ? <th /> : null}
                </tr>
              </thead>
              <tbody>
                {data.indexes.map((idx) => (
                  <tr key={idx.name}>
                    <td className="font-medium">{idx.name}</td>
                    <td className="muted">{idx.columns.join(", ")}</td>
                    <td className="muted">{idx.unique ? "YES" : "NO"}</td>
                    <td className="muted">{idx.type}</td>
                    {canWrite ? (
                      <td style={{ textAlign: "right" }}>
                        <button
                          className="btn btn-ghost btn-sm"
                          aria-label={`Drop index ${idx.name}`}
                          onClick={() => setModal(<DropIndex {...props} index={idx.name} />)}
                        >
                          <Trash2 size={14} />
                        </button>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <Section
        title="Foreign keys"
        action={
          canWrite ? (
            <button className="btn btn-sm" onClick={() => setModal(<ForeignKeyModal {...props} />)}>
              <Plus size={14} /> Add foreign key
            </button>
          ) : null
        }
      >
        {fks.isLoading ? (
          <Spinner />
        ) : fks.error ? (
          <ErrorText message={errMsg(fks.error, "Could not load foreign keys")} />
        ) : !fks.data?.length ? (
          <p className="text-sm muted">No foreign keys.</p>
        ) : (
          <div className="card" style={{ overflowX: "auto" }}>
            <table className="table" style={{ fontSize: 13, whiteSpace: "nowrap" }}>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Columns</th>
                  <th>References</th>
                  <th>On delete</th>
                  <th>On update</th>
                  {canWrite ? <th /> : null}
                </tr>
              </thead>
              <tbody>
                {fks.data.map((fk: ForeignKey) => (
                  <tr key={fk.name}>
                    <td className="font-medium">{fk.name}</td>
                    <td className="muted">{fk.columns.join(", ")}</td>
                    <td className="muted">
                      {fk.ref_table} ({fk.ref_columns.join(", ")})
                    </td>
                    <td className="muted">{fk.on_delete}</td>
                    <td className="muted">{fk.on_update}</td>
                    {canWrite ? (
                      <td style={{ textAlign: "right" }}>
                        <button
                          className="btn btn-ghost btn-sm"
                          aria-label={`Drop foreign key ${fk.name}`}
                          onClick={() =>
                            setModal(
                              <AlterConfirm
                                {...props}
                                title={`Drop foreign key ${fk.name}?`}
                                confirmLabel="Drop foreign key"
                                ops={[{ op: "drop_foreign_key", name: fk.name }]}
                                message="The constraint is removed; the data stays."
                              />,
                            )
                          }
                        >
                          <Trash2 size={14} />
                        </button>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      {data.ddl ? (
        <Section title="DDL">
          <pre className="card" style={{ padding: ".9rem", overflowX: "auto", fontSize: 12, margin: 0 }}>
            <code>{data.ddl}</code>
          </pre>
        </Section>
      ) : null}
      {modal}
    </div>
  );
}

function Section({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: ".4rem" }}>
        <p className="text-sm muted" style={{ margin: 0 }}>
          {title}
        </p>
        {action}
      </div>
      {children}
    </div>
  );
}

type ModalProps = { databaseId: string; table: string; schema: TableSchema; onClose: () => void };

/** The table name without its PostgreSQL schema prefix. */
const bareName = (t: string) => (t.includes(".") ? t.slice(t.indexOf(".") + 1) : t);

function toSpec(c: ColumnInfo): ColumnSpec {
  return {
    name: c.name,
    type: c.type,
    nullable: c.nullable,
    default: null,
    auto_increment: false,
    comment: c.comment || undefined,
    keep_default: true,
  };
}

function ColumnModal({ databaseId, table, onClose, column }: ModalProps & { column: ColumnInfo | null }) {
  const { alter } = useStructureMutations(databaseId, table);
  const [spec, setSpec] = useState<ColumnSpec>(() => (column ? toSpec(column) : emptyColumn()));
  const [error, setError] = useState<string | null>(null);
  const editing = column !== null;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const op: AlterOp = editing
      ? { op: "modify_column", name: column.name, column: spec }
      : { op: "add_column", column: { ...spec, keep_default: undefined } };
    try {
      await alter.mutateAsync([op]);
      onClose();
    } catch (err) {
      setError(errMsg(err, "Change failed"));
    }
  }

  return (
    <Modal open onClose={onClose} title={editing ? `Edit column ${column.name}` : `Add column to ${table}`}>
      <form onSubmit={onSubmit}>
        <ColumnFields value={spec} onChange={setSpec} hideDefault={editing && spec.keep_default} />
        {editing ? (
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={Boolean(spec.keep_default)}
              onChange={(e) => setSpec({ ...spec, keep_default: e.target.checked, default: null, auto_increment: false })}
            />
            Keep the current default{column.default !== null ? ` (${column.default})` : ""}
            {column.extra ? ` and ${column.extra}` : ""}
          </label>
        ) : null}
        {editing ? (
          <p className="text-sm muted">
            Changing the type rewrites the column; values that cannot be converted make the change fail without
            modifying anything.
          </p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
          <button className="btn btn-primary" type="submit" disabled={alter.isPending}>
            {alter.isPending ? "Applying…" : editing ? "Save column" : "Add column"}
          </button>
          <button className="btn" type="button" onClick={onClose} disabled={alter.isPending}>
            Cancel
          </button>
        </div>
      </form>
    </Modal>
  );
}

function IndexModal({ databaseId, table, schema, onClose }: ModalProps) {
  const { createIndex } = useStructureMutations(databaseId, table);
  const [name, setName] = useState(`ix_${bareName(table)}_`);
  const [cols, setCols] = useState<string[]>([]);
  const [unique, setUnique] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await createIndex.mutateAsync({ name, columns: cols, unique });
      onClose();
    } catch (err) {
      setError(errMsg(err, "Could not create the index"));
    }
  }

  return (
    <Modal open onClose={onClose} title={`Add index to ${table}`}>
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label="Columns (in order)">
          <ColumnPicker all={schema.columns.map((c) => c.name)} value={cols} onChange={setCols} />
        </Field>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={unique} onChange={(e) => setUnique(e.target.checked)} /> Unique
        </label>
        <ErrorText message={error ?? undefined} />
        <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
          <button className="btn btn-primary" type="submit" disabled={createIndex.isPending || cols.length === 0}>
            {createIndex.isPending ? "Creating…" : "Create index"}
          </button>
          <button className="btn" type="button" onClick={onClose}>
            Cancel
          </button>
        </div>
      </form>
    </Modal>
  );
}

/** ColumnPicker selects an ordered list of columns. */
function ColumnPicker({ all, value, onChange }: { all: string[]; value: string[]; onChange: (v: string[]) => void }) {
  return (
    <div className="flex items-center gap-2" style={{ flexWrap: "wrap" }}>
      {all.map((c) => {
        const i = value.indexOf(c);
        return (
          <button
            key={c}
            type="button"
            className={`btn btn-sm${i >= 0 ? " btn-primary" : ""}`}
            onClick={() => onChange(i >= 0 ? value.filter((x) => x !== c) : [...value, c])}
          >
            {i >= 0 ? `${i + 1}. ` : ""}
            {c}
          </button>
        );
      })}
    </div>
  );
}

function ForeignKeyModal({ databaseId, table, schema, onClose }: ModalProps) {
  const { alter } = useStructureMutations(databaseId, table);
  const [name, setName] = useState(`fk_${bareName(table)}_`);
  const [cols, setCols] = useState<string[]>([]);
  const [refTable, setRefTable] = useState("");
  const [refCols, setRefCols] = useState("");
  const [onDelete, setOnDelete] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await alter.mutateAsync([
        {
          op: "add_foreign_key",
          foreign_key: {
            name,
            columns: cols,
            ref_table: refTable.trim(),
            ref_columns: refCols.split(",").map((s) => s.trim()).filter(Boolean),
            on_delete: onDelete,
          },
        },
      ]);
      onClose();
    } catch (err) {
      setError(errMsg(err, "Could not add the foreign key"));
    }
  }

  return (
    <Modal open onClose={onClose} title={`Add foreign key to ${table}`}>
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label="Columns">
          <ColumnPicker all={schema.columns.map((c) => c.name)} value={cols} onChange={setCols} />
        </Field>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Referenced table">
            <input className="input" value={refTable} onChange={(e) => setRefTable(e.target.value)} placeholder="customers" required />
          </Field>
          <Field label="Referenced columns (comma-separated)">
            <input className="input" value={refCols} onChange={(e) => setRefCols(e.target.value)} placeholder="id" required />
          </Field>
        </div>
        <Field label="On delete">
          <select className="input" value={onDelete} onChange={(e) => setOnDelete(e.target.value)}>
            <option value="">(default: NO ACTION)</option>
            <option value="CASCADE">CASCADE</option>
            <option value="SET NULL">SET NULL</option>
            <option value="RESTRICT">RESTRICT</option>
          </select>
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
          <button className="btn btn-primary" type="submit" disabled={alter.isPending || cols.length === 0}>
            {alter.isPending ? "Adding…" : "Add foreign key"}
          </button>
          <button className="btn" type="button" onClick={onClose}>
            Cancel
          </button>
        </div>
      </form>
    </Modal>
  );
}

function AlterConfirm({
  databaseId,
  table,
  onClose,
  title,
  confirmLabel,
  confirmText,
  ops,
  message,
}: ModalProps & { title: string; confirmLabel: string; confirmText?: string; ops: AlterOp[]; message: string }) {
  const { alter } = useStructureMutations(databaseId, table);
  const [error, setError] = useState<string | null>(null);
  return (
    <ConfirmModal
      open
      danger
      title={title}
      confirmLabel={confirmLabel}
      confirmText={confirmText}
      busy={alter.isPending}
      message={
        <>
          <p style={{ marginTop: 0 }}>{message}</p>
          <ErrorText message={error ?? undefined} />
        </>
      }
      onConfirm={async () => {
        setError(null);
        try {
          await alter.mutateAsync(ops);
          onClose();
        } catch (err) {
          setError(errMsg(err, "Change failed"));
        }
      }}
      onCancel={onClose}
    />
  );
}

function DropIndex({ databaseId, table, onClose, index }: ModalProps & { index: string }) {
  const { dropIndex } = useStructureMutations(databaseId, table);
  const [error, setError] = useState<string | null>(null);
  return (
    <ConfirmModal
      open
      danger
      title={`Drop index ${index}?`}
      confirmLabel="Drop index"
      busy={dropIndex.isPending}
      message={
        <>
          <p style={{ marginTop: 0 }}>
            Queries that relied on it may get slower. Dropping a primary-key or unique index also drops that
            constraint.
          </p>
          <ErrorText message={error ?? undefined} />
        </>
      }
      onConfirm={async () => {
        setError(null);
        try {
          await dropIndex.mutateAsync(index);
          onClose();
        } catch (err) {
          setError(errMsg(err, "Could not drop the index"));
        }
      }}
      onCancel={onClose}
    />
  );
}

function TruncateTable({ databaseId, table, onClose }: ModalProps) {
  const { truncate } = useStructureMutations(databaseId, table);
  const [error, setError] = useState<string | null>(null);
  return (
    <ConfirmModal
      open
      danger
      title={`Truncate ${table}?`}
      confirmLabel="Delete all rows"
      confirmText={bareName(table)}
      busy={truncate.isPending}
      message={
        <>
          <p style={{ marginTop: 0 }}>Every row is permanently deleted. The table and its structure stay.</p>
          <ErrorText message={error ?? undefined} />
        </>
      }
      onConfirm={async () => {
        setError(null);
        try {
          await truncate.mutateAsync();
          onClose();
        } catch (err) {
          setError(errMsg(err, "Truncate failed"));
        }
      }}
      onCancel={onClose}
    />
  );
}

function DropTable({ databaseId, table, onClose, onDone }: ModalProps & { onDone: () => void }) {
  const { drop } = useStructureMutations(databaseId, table);
  const [error, setError] = useState<string | null>(null);
  return (
    <ConfirmModal
      open
      danger
      title={`Drop table ${table}?`}
      confirmLabel="Drop table"
      confirmText={bareName(table)}
      busy={drop.isPending}
      message={
        <>
          <p style={{ marginTop: 0 }}>The table and all of its data are permanently deleted.</p>
          <ErrorText message={error ?? undefined} />
        </>
      }
      onConfirm={async () => {
        setError(null);
        try {
          await drop.mutateAsync();
          onClose();
          onDone();
        } catch (err) {
          setError(errMsg(err, "Drop failed"));
        }
      }}
      onCancel={onClose}
    />
  );
}

function RenameTable({ databaseId, table, onClose, onDone }: ModalProps & { onDone: (newName: string) => void }) {
  const { rename } = useStructureMutations(databaseId, table);
  const [name, setName] = useState(bareName(table));
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await rename.mutateAsync(name);
      onClose();
      const prefix = table.includes(".") ? table.slice(0, table.indexOf(".") + 1) : "";
      onDone(prefix + name);
    } catch (err) {
      setError(errMsg(err, "Rename failed"));
    }
  }

  return (
    <Modal open onClose={onClose} title={`Rename ${table}`}>
      <form onSubmit={onSubmit}>
        <Field label="New name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        </Field>
        <p className="text-sm muted">Views, routines and application code that use the old name will break.</p>
        <ErrorText message={error ?? undefined} />
        <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
          <button className="btn btn-primary" type="submit" disabled={rename.isPending || name === bareName(table)}>
            {rename.isPending ? "Renaming…" : "Rename"}
          </button>
          <button className="btn" type="button" onClick={onClose}>
            Cancel
          </button>
        </div>
      </form>
    </Modal>
  );
}
