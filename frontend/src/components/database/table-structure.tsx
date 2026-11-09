"use client";

import { useState, type FormEvent, type ReactNode } from "react";

import { ColumnFields, emptyColumn } from "@/components/database/column-editor";
import { findingsFor } from "@/components/database/index-health";
import {
  Badge,
  Button,
  Checkbox,
  ConfirmModal,
  EmptyState,
  ErrorText,
  Field,
  IconButton,
  Input,
  Modal,
  QueryError,
  SectionHeader,
  Select,
  Skeleton,
  Table,
  TableSkeleton,
} from "@/components/ui";
import { ApiError } from "@/lib/api";
import type { Dialect } from "@/lib/data-browser/cells";
import { useForeignKeys, useIndexInsights, useStructureMutations, useTableSchema } from "@/lib/hooks";
import type { AlterOp, ColumnInfo, ColumnSpec, ForeignKey, IndexDef, TableSchema } from "@/lib/types";
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
  dialect = "mysql",
}: {
  databaseId: string;
  table: string;
  canWrite?: boolean;
  /** Engine of the database; picks the column types offered when editing. */
  dialect?: Dialect;
  /** Called after the table was dropped (null) or renamed (new name). */
  onTableGone?: (renamedTo: string | null) => void;
}) {
  const { data, isLoading, error, refetch } = useTableSchema(databaseId, table);
  const fks = useForeignKeys(databaseId, table);
  const insights = useIndexInsights(databaseId);
  const flagged = findingsFor(insights.data, table);
  const [modal, setModal] = useState<ReactNode>(null);
  const close = () => setModal(null);

  if (isLoading) {
    return (
      <div role="status" aria-label="Loading structure" className="stack">
        <Skeleton width="18rem" height="2rem" radius="var(--radius-md)" />
        <TableSkeleton rows={5} columns={6} />
      </div>
    );
  }
  if (error) return <QueryError error={error} what="table structure" onRetry={() => void refetch()} />;
  if (!data) return <EmptyState title="No structure" />;

  const props = { databaseId, table, schema: data, dialect, onClose: close };

  return (
    <div className="flex flex-col gap-4">
      {canWrite ? (
        <div className="row">
          <Button size="sm" icon={<TextCursorInput size={15} aria-hidden />} onClick={() => setModal(<RenameTable {...props} onDone={(n) => onTableGone?.(n)} />)}>
            Rename table
          </Button>
          <Button size="sm" icon={<Eraser size={15} aria-hidden />} onClick={() => setModal(<TruncateTable {...props} />)}>
            Truncate
          </Button>
          <Button size="sm" variant="danger" icon={<Trash2 size={15} aria-hidden />} onClick={() => setModal(<DropTable {...props} onDone={() => onTableGone?.(null)} />)}>
            Drop table
          </Button>
        </div>
      ) : null}

      <Section
        title="Columns"
        action={
          canWrite ? (
            <Button size="sm" icon={<Plus size={14} aria-hidden />} onClick={() => setModal(<ColumnModal {...props} column={null} />)}>
              Add column
            </Button>
          ) : null
        }
      >
        <Table className="text-[length:var(--fs-sm)] whitespace-nowrap">
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
                      <Badge className="text-[length:var(--fs-2xs)]">{c.key}</Badge>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className={c.default === null ? "muted" : undefined}>{c.default === null ? "NULL" : c.default}</td>
                  <td className="muted">{c.extra || "—"}</td>
                  <td className="muted">{c.comment || "—"}</td>
                  {canWrite ? (
                    <td className="text-right whitespace-nowrap">
                      <IconButton label={`Edit column ${c.name}`} onClick={() => setModal(<ColumnModal {...props} column={c} />)}>
                        <Pencil size={14} aria-hidden />
                      </IconButton>
                      <IconButton
                        label={`Drop column ${c.name}`}
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
                        <Trash2 size={14} aria-hidden />
                      </IconButton>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </Table>
      </Section>

      <Section
        title="Indexes"
        action={
          canWrite ? (
            <Button size="sm" icon={<Plus size={14} aria-hidden />} onClick={() => setModal(<IndexModal {...props} />)}>
              Add index
            </Button>
          ) : null
        }
      >
        {data.indexes.length === 0 ? (
          <p className="text-sm muted">No indexes.</p>
        ) : (
          <Table className="text-[length:var(--fs-sm)] whitespace-nowrap">
              <thead>
                <tr>
                  <th>Index</th>
                  <th>Columns</th>
                  <th>Unique</th>
                  <th>Type</th>
                  {insights.data?.usage_available ? <th className="text-right">Scans</th> : null}
                  <th />
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
                    {insights.data?.usage_available ? (
                      <td className="muted num text-right">
                        {usage(insights.data.indexes, table, idx.name)}
                      </td>
                    ) : null}
                    <td>
                      {flagged[idx.name] ? (
                        <Badge
                          tone={flagged[idx.name].kind === "redundant" ? "warning" : "neutral"}
                          title={`${flagged[idx.name].reason}. To drop: ${flagged[idx.name].drop_sql}`}
                        >
                          {flagged[idx.name].kind === "redundant" ? `redundant · ${flagged[idx.name].covered_by}` : "unused"}
                        </Badge>
                      ) : null}
                    </td>
                    {canWrite ? (
                      <td className="text-right whitespace-nowrap">
                        <IconButton
                          label={`Drop index ${idx.name}`}
                          onClick={() => setModal(<DropIndex {...props} index={idx.name} />)}
                        >
                          <Trash2 size={14} aria-hidden />
                        </IconButton>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </Table>
        )}
      </Section>

      <Section
        title="Foreign keys"
        action={
          canWrite ? (
            <Button size="sm" icon={<Plus size={14} aria-hidden />} onClick={() => setModal(<ForeignKeyModal {...props} />)}>
              Add foreign key
            </Button>
          ) : null
        }
      >
        {fks.isLoading ? (
          <TableSkeleton rows={2} columns={5} />
        ) : fks.error ? (
          <ErrorText message={errMsg(fks.error, "Could not load foreign keys")} />
        ) : !fks.data?.length ? (
          <p className="text-sm muted">No foreign keys.</p>
        ) : (
          <Table className="text-[length:var(--fs-sm)] whitespace-nowrap">
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
                      <td className="text-right whitespace-nowrap">
                        <IconButton
                          label={`Drop foreign key ${fk.name}`}
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
                          <Trash2 size={14} aria-hidden />
                        </IconButton>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </Table>
        )}
      </Section>

      {data.ddl ? (
        <Section title="DDL">
          <pre className="card p-3.5 overflow-x-auto text-xs m-0">
            <code>{data.ddl}</code>
          </pre>
        </Section>
      ) : null}
      {modal}
    </div>
  );
}

function usage(indexes: IndexDef[], table: string, name: string): string {
  const ix = indexes.find((i) => i.name === name && (i.table === table || `${i.schema}.${i.table}` === table));
  return ix?.scans === undefined ? "" : ix.scans.toLocaleString();
}

function Section({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section>
      <SectionHeader as="h3" title={title} actions={action} />
      {children}
    </section>
  );
}

type ModalProps = { databaseId: string; table: string; schema: TableSchema; dialect: Dialect; onClose: () => void };

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

function ColumnModal({ databaseId, table, dialect, onClose, column }: ModalProps & { column: ColumnInfo | null }) {
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
        <ColumnFields value={spec} onChange={setSpec} dialect={dialect} hideDefault={editing && spec.keep_default} />
        {editing ? (
          <Checkbox
            checked={Boolean(spec.keep_default)}
            onChange={(e) => setSpec({ ...spec, keep_default: e.target.checked, default: null, auto_increment: false })}
            label={
              <>
                Keep the current default{column.default !== null ? ` (${column.default})` : ""}
                {column.extra ? ` and ${column.extra}` : ""}
              </>
            }
          />
        ) : null}
        {editing ? (
          <p className="text-sm muted">
            Changing the type rewrites the column; values that cannot be converted make the change fail without
            modifying anything.
          </p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose} disabled={alter.isPending}>Cancel</Button>
          <Button type="submit" variant="primary" loading={alter.isPending}>
            {alter.isPending ? "Applying…" : editing ? "Save column" : "Add column"}
          </Button>
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
          <Input value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label="Columns (in order)">
          <ColumnPicker all={schema.columns.map((c) => c.name)} value={cols} onChange={setCols} />
        </Field>
        <Checkbox label="Unique" checked={unique} onChange={(e) => setUnique(e.target.checked)} />
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={createIndex.isPending} disabled={cols.length === 0}>
            {createIndex.isPending ? "Creating…" : "Create index"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/** ColumnPicker selects an ordered list of columns. */
function ColumnPicker({ all, value, onChange }: { all: string[]; value: string[]; onChange: (v: string[]) => void }) {
  return (
    <div className="row" role="group">
      {all.map((c) => {
        const i = value.indexOf(c);
        return (
          <Button
            key={c}
            size="sm"
            variant={i >= 0 ? "primary" : "secondary"}
            aria-pressed={i >= 0}
            onClick={() => onChange(i >= 0 ? value.filter((x) => x !== c) : [...value, c])}
          >
            {i >= 0 ? `${i + 1}. ` : ""}
            {c}
          </Button>
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
          <Input value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label="Columns">
          <ColumnPicker all={schema.columns.map((c) => c.name)} value={cols} onChange={setCols} />
        </Field>
        <div className="form-grid">
          <Field label="Referenced table">
            <Input value={refTable} onChange={(e) => setRefTable(e.target.value)} placeholder="customers" required />
          </Field>
          <Field label="Referenced columns (comma-separated)">
            <Input value={refCols} onChange={(e) => setRefCols(e.target.value)} placeholder="id" required />
          </Field>
        </div>
        <Field label="On delete">
          <Select value={onDelete} onChange={(e) => setOnDelete(e.target.value)}>
            <option value="">(default: NO ACTION)</option>
            <option value="CASCADE">CASCADE</option>
            <option value="SET NULL">SET NULL</option>
            <option value="RESTRICT">RESTRICT</option>
          </Select>
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={alter.isPending} disabled={cols.length === 0}>
            {alter.isPending ? "Adding…" : "Add foreign key"}
          </Button>
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
          <p className="mt-0">{message}</p>
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
          <p className="mt-0">
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
          <p className="mt-0">Every row is permanently deleted. The table and its structure stay.</p>
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
          <p className="mt-0">The table and all of its data are permanently deleted.</p>
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
          <Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        </Field>
        <p className="text-sm muted">Views, routines and application code that use the old name will break.</p>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={rename.isPending} disabled={name === bareName(table)}>
            {rename.isPending ? "Renaming…" : "Rename"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
