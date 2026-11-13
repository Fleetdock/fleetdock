"use client";

import { useState, type FormEvent } from "react";

import { useConfirm } from "@/components/confirm";
import { useToast } from "@/components/toast";
import { Badge, Button, Callout, Checkbox, ErrorText, Field, Input, Modal, Select, Spinner } from "@/components/ui";
import { SectionHeader } from "@/components/ui/detail";
import { ApiError } from "@/lib/api";
import { friendlyError } from "@/lib/errors";
import { parseSearchPath } from "@/lib/search-path";
import {
  useSchemaAccess,
  useSchemaPrivilegeMutations,
  useSchemas,
  useSearchPaths,
  useSetSearchPath,
} from "@/lib/hooks";
import type { DefaultPrivilege, DefaultPrivilegeObject } from "@/lib/types";

import { Plus } from "lucide-react";

// PostgreSQL schema privileges, default privileges and search_path, under the
// database's Access tab.

const OBJECT_PRIVILEGES: Record<DefaultPrivilegeObject, string[]> = {
  tables: ["SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"],
  sequences: ["USAGE", "SELECT", "UPDATE"],
  functions: ["EXECUTE"],
  types: ["USAGE"],
};

/** Fleetdock's own data roles: kept in step by Fleetdock, never edited here. */
const fleetdockRole = (role: string) => /^fleetdock_(ro|rw)_/.test(role);

export function SchemaPrivilegesSection({
  databaseId,
  canWrite,
  canManage,
}: {
  databaseId: string;
  /** database:write: the database's search_path. */
  canWrite: boolean;
  /** instance:write as well: grants, default privileges, roles' search_path. */
  canManage: boolean;
}) {
  const { data: schemas } = useSchemas(databaseId, true);
  const [picked, setPicked] = useState<string | null>(null);
  const names = (schemas ?? []).map((s) => s.name);
  const schema = picked && names.includes(picked) ? picked : names.includes("public") ? "public" : names[0] ?? null;

  return (
    <div className="mt-6">
      <SectionHeader
        as="h3"
        title="Schema privileges"
        description="Who may use each PostgreSQL schema, and what new objects in it are granted."
        actions={
          names.length > 0 ? (
            <Select aria-label="Schema" value={schema ?? ""} onChange={(e) => setPicked(e.target.value)}>
              {names.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </Select>
          ) : null
        }
      />
      {schema ? <SchemaGrants databaseId={databaseId} schema={schema} canManage={canManage} /> : null}
      <SearchPathEditor databaseId={databaseId} schemas={names} canWrite={canWrite} canManage={canManage} />
    </div>
  );
}

function SchemaGrants({ databaseId, schema, canManage }: { databaseId: string; schema: string; canManage: boolean }) {
  const { data, isLoading, error } = useSchemaAccess(databaseId, schema);
  const { setGrant, alterDefaults } = useSchemaPrivilegeMutations(databaseId);
  const { push } = useToast();
  const confirm = useConfirm();
  const [adding, setAdding] = useState<"grant" | "default" | null>(null);

  if (isLoading) return <Spinner label="Reading privileges…" />;
  if (error || !data) return <ErrorText message={(error as ApiError | null)?.message ?? "Could not read the privileges."} />;

  async function toggle(role: string, usage: boolean, create: boolean) {
    try {
      await setGrant.mutateAsync({ schema, role, usage, create });
      push("success", `Saved ${role}'s privileges on ${schema}`);
    } catch (err) {
      push("error", friendlyError(err, "Could not change the privileges"));
    }
  }

  async function revokeDefault(d: DefaultPrivilege) {
    const ok = await confirm({
      title: "Stop granting this?",
      message: (
        <>
          New {d.object_type} that <code>{d.creator}</code> creates in {schema} will no longer be granted{" "}
          {d.privileges.join(", ")} to <code>{d.grantee}</code>. Existing objects keep their privileges.
        </>
      ),
      confirmLabel: "Stop granting",
      danger: true,
    });
    if (!ok) return;
    try {
      await alterDefaults.mutateAsync({
        schema,
        creator: d.creator,
        object_type: d.object_type,
        grantee: d.grantee,
        privileges: d.privileges,
        revoke: true,
      });
      push("success", "Default privileges changed");
    } catch (err) {
      push("error", friendlyError(err, "Could not change the default privileges"));
    }
  }

  const busy = setGrant.isPending || alterDefaults.isPending;
  return (
    <>
      <p className="text-sm muted">
        Owned by <code>{data.owner}</code>, who can do everything in it.
        {!canManage ? " Changing privileges needs write access to the server." : null}
      </p>
      <table className="table">
        <thead>
          <tr>
            <th>Role</th>
            <th>Use objects (USAGE)</th>
            <th>Create objects (CREATE)</th>
            {canManage ? <th className="text-right">Actions</th> : null}
          </tr>
        </thead>
        <tbody>
          {data.grants.length === 0 ? (
            <tr>
              <td colSpan={canManage ? 4 : 3} className="muted">
                No other role has privileges on {schema}.
              </td>
            </tr>
          ) : (
            data.grants.map((g) => (
              <tr key={g.role}>
                <td className="font-medium">
                  {g.role === "PUBLIC" ? <Badge>PUBLIC: every role</Badge> : g.role}
                  {fleetdockRole(g.role) ? (
                    <Badge className="ml-2" title="Fleetdock's data role for this database; Fleetdock keeps its grants in step">
                      Fleetdock
                    </Badge>
                  ) : null}
                </td>
                <td>
                  <input
                    type="checkbox"
                    aria-label={`${g.role} may use objects in ${schema}`}
                    checked={g.usage}
                    disabled={!canManage || busy || fleetdockRole(g.role)}
                    onChange={(e) => toggle(g.role, e.target.checked, g.create)}
                  />
                </td>
                <td>
                  <input
                    type="checkbox"
                    aria-label={`${g.role} may create objects in ${schema}`}
                    checked={g.create}
                    disabled={!canManage || busy || fleetdockRole(g.role)}
                    onChange={(e) => toggle(g.role, g.usage, e.target.checked)}
                  />
                </td>
                {canManage ? (
                  <td className="text-right">
                    {fleetdockRole(g.role) ? null : (
                      <Button size="sm" variant="danger" disabled={busy} onClick={() => toggle(g.role, false, false)}>
                        Revoke
                      </Button>
                    )}
                  </td>
                ) : null}
              </tr>
            ))
          )}
        </tbody>
      </table>
      {canManage ? (
        <div className="toolbar mt-2">
          <Button size="sm" icon={<Plus size={15} aria-hidden />} onClick={() => setAdding("grant")}>
            Grant on {schema}
          </Button>
        </div>
      ) : null}

      <h4 className="mt-6 mb-1">Default privileges</h4>
      <p className="text-sm muted mt-0">
        What objects created in {schema} from now on are granted (ALTER DEFAULT PRIVILEGES). Existing objects are not
        changed.
      </p>
      <table className="table">
        <thead>
          <tr>
            <th>New objects of</th>
            <th>Kind</th>
            <th>Granted to</th>
            <th>Privileges</th>
            {canManage ? <th className="text-right">Actions</th> : null}
          </tr>
        </thead>
        <tbody>
          {data.default_privileges.length === 0 ? (
            <tr>
              <td colSpan={canManage ? 5 : 4} className="muted">
                None set for {schema}.
              </td>
            </tr>
          ) : (
            data.default_privileges.map((d) => (
              <tr key={`${d.creator}/${d.object_type}/${d.grantee}`}>
                <td>{d.creator}</td>
                <td>{d.object_type}</td>
                <td>{d.grantee}</td>
                <td>
                  <div className="flex flex-wrap gap-1">
                    {d.privileges.map((p) => (
                      <Badge key={p}>{p}</Badge>
                    ))}
                  </div>
                </td>
                {canManage ? (
                  <td className="text-right">
                    {fleetdockRole(d.grantee) ? null : (
                      <Button size="sm" variant="danger" disabled={busy} onClick={() => revokeDefault(d)}>
                        Remove
                      </Button>
                    )}
                  </td>
                ) : null}
              </tr>
            ))
          )}
        </tbody>
      </table>
      {canManage ? (
        <div className="toolbar mt-2">
          <Button size="sm" icon={<Plus size={15} aria-hidden />} onClick={() => setAdding("default")}>
            Add default privilege
          </Button>
        </div>
      ) : null}

      <datalist id="fd-schema-roles">
        <option value="PUBLIC" />
        {data.roles.map((r) => (
          <option key={r} value={r} />
        ))}
      </datalist>
      {adding === "grant" ? <GrantModal databaseId={databaseId} schema={schema} onClose={() => setAdding(null)} /> : null}
      {adding === "default" ? (
        <DefaultModal databaseId={databaseId} schema={schema} owner={data.owner} onClose={() => setAdding(null)} />
      ) : null}
    </>
  );
}

function GrantModal({ databaseId, schema, onClose }: { databaseId: string; schema: string; onClose: () => void }) {
  const { setGrant } = useSchemaPrivilegeMutations(databaseId);
  const { push } = useToast();
  const [role, setRole] = useState("");
  const [usage, setUsage] = useState(true);
  const [create, setCreate] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await setGrant.mutateAsync({ schema, role: role.trim(), usage, create });
      push("success", `Saved ${role.trim()}'s privileges on ${schema}`);
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not grant the privileges.");
    }
  }

  return (
    <Modal open onClose={onClose} title={`Grant on ${schema}`}>
      <form onSubmit={submit}>
        <Field label="Role" hint="A role on the server, or PUBLIC for every role.">
          <Input value={role} onChange={(e) => setRole(e.target.value)} list="fd-schema-roles" autoFocus autoComplete="off" />
        </Field>
        <Checkbox label="Use objects in the schema (USAGE)" checked={usage} onChange={(e) => setUsage(e.target.checked)} />
        <Checkbox label="Create objects in the schema (CREATE)" checked={create} onChange={(e) => setCreate(e.target.checked)} />
        <p className="text-sm muted">USAGE lets the role reach the schema&apos;s objects; it still needs privileges on each table.</p>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={setGrant.isPending} disabled={!role.trim() || (!usage && !create)}>
            Grant
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function DefaultModal({
  databaseId,
  schema,
  owner,
  onClose,
}: {
  databaseId: string;
  schema: string;
  owner: string;
  onClose: () => void;
}) {
  const { alterDefaults } = useSchemaPrivilegeMutations(databaseId);
  const { push } = useToast();
  const [creator, setCreator] = useState(owner);
  const [kind, setKind] = useState<DefaultPrivilegeObject>("tables");
  const [grantee, setGrantee] = useState("");
  const [privs, setPrivs] = useState<string[]>(["SELECT"]);
  const [error, setError] = useState<string | null>(null);
  const offered = OBJECT_PRIVILEGES[kind];
  const chosen = privs.filter((p) => offered.includes(p));

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await alterDefaults.mutateAsync({
        schema,
        creator: creator.trim() || undefined,
        object_type: kind,
        grantee: grantee.trim(),
        privileges: chosen,
        revoke: false,
      });
      push("success", "Default privileges changed");
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not change the default privileges.");
    }
  }

  return (
    <Modal open onClose={onClose} title={`Default privileges in ${schema}`}>
      <form onSubmit={submit}>
        <Field label="For new objects created by" hint="Objects get the grant only when this role creates them.">
          <Input value={creator} onChange={(e) => setCreator(e.target.value)} list="fd-schema-roles" autoComplete="off" />
        </Field>
        <Field label="Kind of object">
          <Select value={kind} onChange={(e) => setKind(e.target.value as DefaultPrivilegeObject)}>
            {Object.keys(OBJECT_PRIVILEGES).map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Grant to">
          <Input value={grantee} onChange={(e) => setGrantee(e.target.value)} list="fd-schema-roles" autoFocus autoComplete="off" />
        </Field>
        <fieldset className="mb-3">
          <legend className="text-sm font-medium mb-1">Privileges</legend>
          <div className="flex flex-wrap gap-3">
            {offered.map((p) => (
              <Checkbox
                key={p}
                label={p}
                checked={chosen.includes(p)}
                onChange={(e) => setPrivs(e.target.checked ? [...chosen, p] : chosen.filter((x) => x !== p))}
              />
            ))}
          </div>
        </fieldset>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="primary"
            loading={alterDefaults.isPending}
            disabled={!grantee.trim() || chosen.length === 0}
          >
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function SearchPathEditor({
  databaseId,
  schemas,
  canWrite,
  canManage,
}: {
  databaseId: string;
  schemas: string[];
  canWrite: boolean;
  canManage: boolean;
}) {
  const { data, error } = useSearchPaths(databaseId);
  const [editing, setEditing] = useState<{ role: string; path: string } | null>(null);

  return (
    <>
      <h4 className="mt-6 mb-1">search_path</h4>
      <p className="text-sm muted mt-0">
        The schemas PostgreSQL looks in for names without a schema. New sessions pick a change up; open ones keep theirs.
      </p>
      {error ? <ErrorText message={(error as ApiError).message} /> : null}
      {data ? (
        <table className="table">
          <thead>
            <tr>
              <th>For</th>
              <th>search_path</th>
              <th className="text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td className="font-medium">This database</td>
              <td>
                {data.database ? <code>{data.database}</code> : <span className="muted">Not set: {data.server}</span>}
              </td>
              <td className="text-right">
                {canWrite ? (
                  <Button size="sm" onClick={() => setEditing({ role: "", path: data.database || data.server })}>
                    Change
                  </Button>
                ) : null}
              </td>
            </tr>
            {data.roles.map((r) => (
              <tr key={r.role}>
                <td>Role {r.role}</td>
                <td>
                  <code>{r.path}</code>
                </td>
                <td className="text-right">
                  {canManage ? (
                    <Button size="sm" onClick={() => setEditing({ role: r.role, path: r.path })}>
                      Change
                    </Button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
      {canManage ? (
        <div className="toolbar mt-2">
          <Button size="sm" icon={<Plus size={15} aria-hidden />} onClick={() => setEditing({ role: "?", path: "" })}>
            Set for a role
          </Button>
        </div>
      ) : null}
      {editing ? (
        <SearchPathModal
          databaseId={databaseId}
          schemas={schemas}
          role={editing.role}
          initial={editing.path}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </>
  );
}

function SearchPathModal({
  databaseId,
  schemas,
  role: initialRole,
  initial,
  onClose,
}: {
  databaseId: string;
  schemas: string[];
  /** "" is the database, "?" a role still to be chosen. */
  role: string;
  initial: string;
  onClose: () => void;
}) {
  const set = useSetSearchPath(databaseId);
  const { push } = useToast();
  const [role, setRole] = useState(initialRole === "?" ? "" : initialRole);
  const [path, setPath] = useState(parseSearchPath(initial).join(", "));
  const [error, setError] = useState<string | null>(null);
  const forRole = initialRole !== "";
  const names = path
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  const unknown = names.filter((n) => n !== "$user" && !schemas.includes(n));

  async function save(schemasToSet: string[]) {
    setError(null);
    try {
      await set.mutateAsync({ role: forRole ? role.trim() : undefined, schemas: schemasToSet });
      push("success", schemasToSet.length ? "search_path saved" : "search_path reset");
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the search_path.");
    }
  }

  return (
    <Modal open onClose={onClose} title={forRole ? "search_path of a role" : "search_path of this database"}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void save(names);
        }}
      >
        {forRole ? (
          <Field label="Role">
            <Input value={role} onChange={(e) => setRole(e.target.value)} disabled={initialRole !== "?"} autoComplete="off" />
          </Field>
        ) : null}
        <Field label="Schemas, in order" hint={'Separated by commas. $user is the schema named like the session\'s role.'}>
          <Input value={path} onChange={(e) => setPath(e.target.value)} autoFocus autoComplete="off" placeholder="$user, public" />
        </Field>
        {unknown.length ? (
          <Callout tone="warning">
            {unknown.join(", ")} {unknown.length === 1 ? "is not a schema" : "are not schemas"} of this database. PostgreSQL
            skips missing schemas.
          </Callout>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button type="button" onClick={() => save([])} disabled={set.isPending || (forRole && !role.trim())}>
            Reset
          </Button>
          <div className="toolbar-spacer" />
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={set.isPending} disabled={names.length === 0 || (forRole && !role.trim())}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
