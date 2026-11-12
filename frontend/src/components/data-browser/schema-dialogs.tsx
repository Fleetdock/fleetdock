"use client";

import { useState, type FormEvent } from "react";

import { useToast } from "@/components/toast";
import { Button, Checkbox, ErrorText, Field, Input, Modal, Spinner, Textarea } from "@/components/ui";
import { ApiError } from "@/lib/api";
import type { SchemaSummary } from "@/lib/data-browser/schemas";
import { useSchemaDependents, useSchemaMutations } from "@/lib/hooks";

// Dialogs that create, change and drop PostgreSQL schemas, opened from the
// data browser's tree.

export type SchemaDialog =
  | { kind: "create" }
  | { kind: "rename" | "owner" | "comment"; schema: SchemaSummary }
  | { kind: "drop"; schema: SchemaSummary };

const errorText = (err: unknown, fallback: string) => (err instanceof ApiError ? err.message : fallback);

/** Matches the server's rule for new names. */
const NAME_RE = /^[A-Za-z_][A-Za-z0-9_$]{0,62}$/;
const nameProblem = (name: string) =>
  !name
    ? "Enter a name."
    : !NAME_RE.test(name)
      ? "Use letters, digits and _ (at most 63, not starting with a digit)."
      : /^pg_/i.test(name)
        ? "Names starting with pg_ are reserved."
        : null;

export function SchemaDialogs({
  databaseId,
  dialog,
  owners,
  onClose,
  onRenamed,
  onDropped,
  onCreated,
}: {
  databaseId: string;
  dialog: SchemaDialog | null;
  /** Roles that own schemas already, offered as owners. */
  owners: string[];
  onClose: () => void;
  onRenamed: (from: string, to: string) => void;
  onDropped: (name: string) => void;
  onCreated: (name: string) => void;
}) {
  if (!dialog) return null;
  if (dialog.kind === "create") {
    return <CreateSchema databaseId={databaseId} owners={owners} onClose={onClose} onCreated={onCreated} />;
  }
  if (dialog.kind === "drop") {
    return <DropSchema databaseId={databaseId} schema={dialog.schema} onClose={onClose} onDropped={onDropped} />;
  }
  return (
    <EditSchema
      databaseId={databaseId}
      kind={dialog.kind}
      schema={dialog.schema}
      owners={owners}
      onClose={onClose}
      onRenamed={onRenamed}
    />
  );
}

/** The roles offered for an owner, for an Input with list="fd-schema-owners". */
function OwnerOptions({ owners }: { owners: string[] }) {
  return (
    <datalist id="fd-schema-owners">
      {owners.map((o) => (
        <option key={o} value={o} />
      ))}
    </datalist>
  );
}

function CreateSchema({
  databaseId,
  owners,
  onClose,
  onCreated,
}: {
  databaseId: string;
  owners: string[];
  onClose: () => void;
  onCreated: (name: string) => void;
}) {
  const { create } = useSchemaMutations(databaseId);
  const { push } = useToast();
  const [name, setName] = useState("");
  const [owner, setOwner] = useState("");
  const [error, setError] = useState<string | null>(null);
  const problem = nameProblem(name.trim());

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (problem) return setError(problem);
    setError(null);
    try {
      await create.mutateAsync({ name: name.trim(), owner: owner.trim() || undefined });
      push("success", `Created schema ${name.trim()}`);
      onCreated(name.trim());
      onClose();
    } catch (err) {
      setError(errorText(err, "Could not create the schema."));
    }
  }

  return (
    <Modal open onClose={onClose} title="New schema">
      <form onSubmit={submit}>
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus autoComplete="off" />
        </Field>
        <Field label="Owner" hint="Leave empty for the database's owner, who can then use the schema like the rest of the database.">
          <Input
            value={owner}
            onChange={(e) => setOwner(e.target.value)}
            list="fd-schema-owners"
            placeholder="The database's owner"
            autoComplete="off"
          />
        </Field>
        <OwnerOptions owners={owners} />
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={create.isPending} disabled={Boolean(problem)}>
            Create schema
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function EditSchema({
  databaseId,
  kind,
  schema,
  owners,
  onClose,
  onRenamed,
}: {
  databaseId: string;
  kind: "rename" | "owner" | "comment";
  schema: SchemaSummary;
  owners: string[];
  onClose: () => void;
  onRenamed: (from: string, to: string) => void;
}) {
  const { alter } = useSchemaMutations(databaseId);
  const { push } = useToast();
  const initial = kind === "rename" ? schema.name : kind === "owner" ? schema.owner ?? "" : schema.comment ?? "";
  const [value, setValue] = useState(initial);
  const [error, setError] = useState<string | null>(null);
  const trimmed = kind === "comment" ? value : value.trim();
  const problem = kind === "rename" ? nameProblem(trimmed) : kind === "owner" && !trimmed ? "Enter a role." : null;
  const unchanged = trimmed === initial;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (problem) return setError(problem);
    if (unchanged) return onClose();
    setError(null);
    const change = kind === "rename" ? { name: trimmed } : kind === "owner" ? { owner: trimmed } : { comment: trimmed };
    try {
      await alter.mutateAsync({ schema: schema.name, change });
      if (kind === "rename") {
        push("success", `Renamed ${schema.name} to ${trimmed}`);
        onRenamed(schema.name, trimmed);
      } else {
        push("success", kind === "owner" ? `${schema.name} now belongs to ${trimmed}` : `Saved the comment of ${schema.name}`);
      }
      onClose();
    } catch (err) {
      setError(errorText(err, "Could not change the schema."));
    }
  }

  const title = kind === "rename" ? `Rename ${schema.name}` : kind === "owner" ? `Owner of ${schema.name}` : `Comment on ${schema.name}`;
  return (
    <Modal open onClose={onClose} title={title}>
      <form onSubmit={submit}>
        {kind === "rename" ? (
          <Field label="New name" hint="Views and functions that spell the old name out in their text are not rewritten.">
            <Input value={value} onChange={(e) => setValue(e.target.value)} autoFocus autoComplete="off" />
          </Field>
        ) : kind === "owner" ? (
          <>
            <Field label="Owner" hint="An existing role. The owner can change and drop everything in the schema.">
              <Input value={value} onChange={(e) => setValue(e.target.value)} list="fd-schema-owners" autoComplete="off" autoFocus />
            </Field>
            <OwnerOptions owners={owners} />
          </>
        ) : (
          <Field label="Comment" hint="Leave empty to remove it.">
            <Textarea value={value} onChange={(e) => setValue(e.target.value)} rows={3} maxLength={1024} autoFocus />
          </Field>
        )}
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={alter.isPending} disabled={Boolean(problem)}>
            {kind === "rename" ? "Rename" : "Save"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function plural(n: number, one: string, many: string) {
  return `${n.toLocaleString()} ${n === 1 ? one : many}`;
}

function DropSchema({
  databaseId,
  schema,
  onClose,
  onDropped,
}: {
  databaseId: string;
  schema: SchemaSummary;
  onClose: () => void;
  onDropped: (name: string) => void;
}) {
  const deps = useSchemaDependents(databaseId, schema.name);
  const { drop } = useSchemaMutations(databaseId);
  const { push } = useToast();
  const [cascade, setCascade] = useState(false);
  const [typed, setTyped] = useState("");
  const [error, setError] = useState<string | null>(null);

  const d = deps.data;
  const holds = d
    ? [
        d.tables ? plural(d.tables, "table", "tables") : null,
        d.views ? plural(d.views, "view", "views") : null,
        d.sequences ? plural(d.sequences, "sequence", "sequences") : null,
        d.functions ? plural(d.functions, "function", "functions") : null,
        d.types ? plural(d.types, "type", "types") : null,
      ].filter((x): x is string => Boolean(x))
    : [];
  const empty = d ? holds.length === 0 && d.external.length === 0 : false;
  const ready = Boolean(d) && (empty || (cascade && typed === schema.name));

  async function confirm() {
    setError(null);
    try {
      await drop.mutateAsync({ schema: schema.name, cascade: !empty });
      push("success", `Dropped schema ${schema.name}`);
      onDropped(schema.name);
      onClose();
    } catch (err) {
      setError(errorText(err, "Could not drop the schema."));
    }
  }

  return (
    <Modal open onClose={onClose} title={`Drop schema ${schema.name}?`}>
      {deps.isLoading ? (
        <div className="flex items-center gap-2 text-sm muted">
          <Spinner /> Checking what is in it…
        </div>
      ) : deps.error ? (
        <ErrorText message={errorText(deps.error, "Could not check what the schema holds.")} />
      ) : empty ? (
        <p className="text-sm">The schema is empty and nothing depends on it.</p>
      ) : (
        <div className="stack-sm text-sm">
          {holds.length ? <p>It holds {holds.join(", ")}. All of it is dropped with the schema, with its data.</p> : null}
          {d && d.external.length ? (
            <div>
              <p>These objects in other schemas depend on it and are dropped too:</p>
              <ul className="list-disc pl-5 mt-1">
                {d.external.map((x) => (
                  <li key={x}>
                    <code>{x}</code>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          <Checkbox checked={cascade} onChange={(e) => setCascade(e.target.checked)} label="Drop everything in it, and what depends on it (CASCADE)" />
          {cascade ? (
            <Field label={`Type ${schema.name} to confirm`}>
              <Input value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus autoComplete="off" />
            </Field>
          ) : null}
        </div>
      )}
      <ErrorText message={error ?? undefined} />
      <div className="modal-actions">
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="danger" loading={drop.isPending} disabled={!ready} onClick={() => void confirm()}>
          Drop schema
        </Button>
      </div>
    </Modal>
  );
}
