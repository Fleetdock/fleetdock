"use client";

import { useState, type FormEvent } from "react";

import { Plus } from "lucide-react";

import { useConfirm } from "@/components/confirm";
import { useToast } from "@/components/toast";
import { Badge, Button, Checkbox, ErrorText, Field, Input, Modal, Select, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { friendlyError } from "@/lib/errors";
import { useExtensionMutations, useExtensions, useSchemas } from "@/lib/hooks";
import type { ExtensionInfo } from "@/lib/types";

/**
 * ExtensionsSection lists a PostgreSQL database's extensions: installed
 * ones with update and remove, and the rest the server offers to add.
 */
export function ExtensionsSection({ databaseId, canManage }: { databaseId: string; canManage: boolean }) {
  const { data, isLoading, error } = useExtensions(databaseId);
  const { update, drop } = useExtensionMutations(databaseId);
  const confirm = useConfirm();
  const { push } = useToast();
  const [adding, setAdding] = useState(false);
  const [dropping, setDropping] = useState<ExtensionInfo | null>(null);

  if (isLoading) return <Spinner label="Reading extensions…" />;
  if (error || !data) return <ErrorText message={friendlyError(error, "Could not read the extensions.")} />;
  const installed = data.items.filter((x) => x.installed_version);
  const available = data.items.filter((x) => !x.installed_version);

  async function onUpdate(x: ExtensionInfo) {
    const to = x.updates?.[x.updates.length - 1];
    const ok = await confirm({
      title: `Update ${x.name}?`,
      message: `${x.name} goes from ${x.installed_version} to ${to ?? x.default_version}. Check the extension's release notes first: an update can change its functions.`,
      confirmLabel: "Update",
    });
    if (!ok) return;
    try {
      await update.mutateAsync({ name: x.name, version: to });
      push("success", `${x.name} updated`);
    } catch (err) {
      push("error", friendlyError(err, "Could not update the extension"));
    }
  }

  return (
    <div>
      <div className="toolbar">
        <p className="text-sm muted m-0">
          Extensions installed in this database, read live from the server.
          {!canManage ? " Changing them needs write access to the server." : null}
        </p>
        <div className="toolbar-spacer" />
        {canManage ? (
          <Button size="sm" variant="primary" icon={<Plus size={15} aria-hidden />} onClick={() => setAdding(true)}>
            Add extension
          </Button>
        ) : null}
      </div>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Extension</th>
              <th>Version</th>
              <th>Schema</th>
              <th>About</th>
              {canManage ? <th className="text-right">Actions</th> : null}
            </tr>
          </thead>
          <tbody>
            {installed.map((x) => {
              const newer = (x.updates?.length ?? 0) > 0;
              return (
                <tr key={x.name}>
                  <td className="font-medium">{x.name}</td>
                  <td className="whitespace-nowrap">
                    {x.installed_version}{" "}
                    {newer ? (
                      <Badge tone="info" title={`Can be updated to ${x.updates!.join(", ")}`}>
                        {x.updates![x.updates!.length - 1]} available
                      </Badge>
                    ) : null}
                  </td>
                  <td>{x.schema}</td>
                  <td className="muted text-sm">{x.comment}</td>
                  {canManage ? (
                    <td className="text-right whitespace-nowrap">
                      {newer ? (
                        <Button size="sm" onClick={() => void onUpdate(x)} disabled={update.isPending}>
                          Update
                        </Button>
                      ) : null}{" "}
                      {x.name === "plpgsql" ? null : (
                        <Button size="sm" variant="danger" onClick={() => setDropping(x)}>
                          Remove
                        </Button>
                      )}
                    </td>
                  ) : null}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <p className="text-sm muted">
        {available.length} more {available.length === 1 ? "is" : "are"} available on this server.
      </p>
      {adding ? <AddExtension databaseId={databaseId} available={available} onClose={() => setAdding(false)} /> : null}
      {dropping ? <DropExtension databaseId={databaseId} ext={dropping} onClose={() => setDropping(null)} /> : null}
      {drop.isPending ? <Spinner label="Removing…" /> : null}
    </div>
  );
}

function AddExtension({
  databaseId,
  available,
  onClose,
}: {
  databaseId: string;
  available: ExtensionInfo[];
  onClose: () => void;
}) {
  const { install } = useExtensionMutations(databaseId);
  const { data: schemas } = useSchemas(databaseId, true);
  const { push } = useToast();
  const [query, setQuery] = useState("");
  const [name, setName] = useState("");
  const [schema, setSchema] = useState("");
  const [error, setError] = useState<string | null>(null);
  const q = query.trim().toLowerCase();
  const matches = available.filter((x) => !q || x.name.includes(q) || x.comment.toLowerCase().includes(q)).slice(0, 60);
  const picked = available.find((x) => x.name === name);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!picked) return;
    setError(null);
    try {
      await install.mutateAsync({ name: picked.name, schema: schema || undefined });
      push("success", `Installed ${picked.name}`);
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not install the extension.");
    }
  }

  return (
    <Modal open onClose={onClose} title="Add an extension" wide>
      <form onSubmit={submit}>
        <Field label="Find">
          <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="pg_trgm, uuid, crypto…" autoFocus autoComplete="off" />
        </Field>
        <div className="ext-pick" role="radiogroup" aria-label="Extension">
          {matches.length === 0 ? (
            <p className="muted text-sm">Nothing matches.</p>
          ) : (
            matches.map((x) => (
              <label key={x.name} className={`ext-pick-row${x.name === name ? " active" : ""}`}>
                <input type="radio" name="extension" checked={x.name === name} onChange={() => setName(x.name)} />
                <span className="flex-1 min-w-0">
                  <span className="font-medium">{x.name}</span> <span className="muted text-xs">{x.default_version}</span>
                  {x.trusted ? (
                    <Badge tone="success" className="ml-2" title="A database owner may install it without a superuser">
                      trusted
                    </Badge>
                  ) : x.superuser ? (
                    <Badge className="ml-2" title="Installing it needs a superuser">
                      superuser
                    </Badge>
                  ) : null}
                  <span className="muted text-sm block truncate">{x.comment}</span>
                </span>
              </label>
            ))
          )}
        </div>
        <Field label="Into schema" hint="Its types and functions are created there. Leave empty for the first schema of the search_path.">
          <Select value={schema} onChange={(e) => setSchema(e.target.value)}>
            <option value="">Default (search_path)</option>
            {(schemas ?? []).map((s) => (
              <option key={s.name} value={s.name}>
                {s.name}
              </option>
            ))}
          </Select>
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={install.isPending} disabled={!picked}>
            {picked ? `Install ${picked.name}` : "Install"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function DropExtension({ databaseId, ext, onClose }: { databaseId: string; ext: ExtensionInfo; onClose: () => void }) {
  const { drop } = useExtensionMutations(databaseId);
  const { push } = useToast();
  const [cascade, setCascade] = useState(false);
  const [typed, setTyped] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await drop.mutateAsync({ name: ext.name, cascade });
      push("success", `Removed ${ext.name}`);
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not remove the extension.");
    }
  }

  return (
    <Modal open onClose={onClose} title={`Remove ${ext.name}?`}>
      <form onSubmit={submit}>
        <p className="text-sm mt-0">
          Its types, functions and operators are dropped. Without the option below this is refused while anything
          uses them.
        </p>
        <Checkbox
          label="Also drop everything that uses it"
          hint="Columns of its types, indexes and functions that use it (CASCADE). Their data is lost."
          checked={cascade}
          onChange={(e) => setCascade(e.target.checked)}
        />
        {cascade ? (
          <Field label={`Type ${ext.name} to confirm`}>
            <Input value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" />
          </Field>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="danger" loading={drop.isPending} disabled={cascade && typed !== ext.name}>
            Remove {ext.name}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
