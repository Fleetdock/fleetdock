"use client";

import { useEffect, useMemo, useState, type FormEvent } from "react";

import { FormError } from "@/components/access/form-error";
import { Button, Checkbox, Field, Input, Modal } from "@/components/ui";
import { fieldError } from "@/lib/errors";
import { useCreateRole, useUpdateRole } from "@/lib/hooks";
import type { Role } from "@/lib/types";

function groupPermissions(perms: string[]) {
  const groups = new Map<string, string[]>();
  for (const p of perms) {
    const [resource] = p.split(":");
    groups.set(resource, [...(groups.get(resource) ?? []), p]);
  }
  return [...groups.entries()];
}

type RoleModalProps = {
  mode: "create" | "edit";
  role?: Role | null;
  open: boolean;
  onClose: () => void;
  allPermissions: string[];
};

export function RoleModal({ mode, role, open, onClose, allPermissions }: RoleModalProps) {
  const create = useCreateRole();
  const update = useUpdateRole();
  const isEdit = mode === "edit";

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!open) return;
    if (isEdit && role) {
      setName(role.name);
      setDescription(role.description);
      setSelected(new Set(role.permissions));
    } else {
      setName("");
      setDescription("");
      setSelected(new Set());
    }
    setError(null);
  }, [open, isEdit, role]);

  const groups = useMemo(() => groupPermissions(allPermissions), [allPermissions]);
  const pending = create.isPending || update.isPending;

  function toggle(perm: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(perm)) {
        next.delete(perm);
      } else {
        next.add(perm);
      }
      return next;
    });
  }

  function toggleGroup(perms: string[]) {
    setSelected((prev) => {
      const next = new Set(prev);
      const allOn = perms.every((p) => next.has(p));
      perms.forEach((p) => (allOn ? next.delete(p) : next.add(p)));
      return next;
    });
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const payload = { name, description, permissions: [...selected] };
    try {
      if (isEdit && role) {
        await update.mutateAsync({ id: role.id, ...payload });
      } else {
        await create.mutateAsync(payload);
      }
      onClose();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={isEdit ? `Edit role "${role?.name}"` : "Create role"} wide>
      <form onSubmit={onSubmit}>
        <div className="form-grid">
          <Field label="Name" hint="Lowercase letters, digits, - and _." error={fieldError(error, "name")}>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="backup-operator" required />
          </Field>
          <Field label="Description (optional)" error={fieldError(error, "description")}>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Can run and restore backups"
            />
          </Field>
        </div>
        <fieldset className="field m-0 border-0 p-0">
          <legend className="label">Permissions</legend>
          <div className="card p-3 max-h-72 overflow-y-auto stack-sm">
            {groups.map(([resource, perms]) => (
              <div key={resource}>
                <Checkbox
                  className="font-medium"
                  label={resource}
                  checked={perms.every((p) => selected.has(p))}
                  onChange={() => toggleGroup(perms)}
                />
                <div className="flex flex-wrap gap-x-4 gap-y-1 pl-6 mt-1">
                  {perms.map((p) => (
                    <Checkbox
                      key={p}
                      className="text-sm muted"
                      label={p.split(":")[1]}
                      aria-label={p}
                      checked={selected.has(p)}
                      onChange={() => toggle(p)}
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>
          <p className="field-hint" aria-live="polite">
            {selected.size} permission{selected.size === 1 ? "" : "s"} selected
          </p>
          {fieldError(error, "permissions") ? <p className="field-error">{fieldError(error, "permissions")}</p> : null}
        </fieldset>
        <FormError
          error={error}
          fields={["name", "description", "permissions"]}
          fallback={`Failed to ${isEdit ? "update" : "create"} role`}
        />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={pending}>
            {pending ? "Saving…" : isEdit ? "Save changes" : "Create role"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
