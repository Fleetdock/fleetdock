"use client";

import { useMemo, useState, type FormEvent } from "react";

import { FormError } from "@/components/access/form-error";
import { Button, Checkbox, Field, HelpText, Input, Modal } from "@/components/ui";
import { fieldError } from "@/lib/errors";
import { useCreateToken, useMe } from "@/lib/hooks";

export function CreateTokenModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (secret: string) => void;
}) {
  const create = useCreateToken();
  const { data: me } = useMe();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>([]);
  const [ttlHours, setTtlHours] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [ttlError, setTtlError] = useState<string | null>(null);

  // A token can only carry scopes the user actually holds (at any scope).
  const available = useMemo(() => {
    const held = new Set((me?.grants ?? []).map((g) => g.permission));
    return [...held].sort();
  }, [me]);

  function toggle(perm: string) {
    setScopes((s) => (s.includes(perm) ? s.filter((p) => p !== perm) : [...s, perm]));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setTtlError(null);
    const ttl = ttlHours.trim() === "" ? undefined : Number(ttlHours);
    if (ttl !== undefined && (!Number.isFinite(ttl) || ttl <= 0)) {
      setTtlError("Expiry must be a positive number of hours.");
      return;
    }
    try {
      const res = await create.mutateAsync({ name, scopes, ttl_hours: ttl });
      setName("");
      setScopes([]);
      setTtlHours("");
      onClose();
      onCreated(res.token);
    } catch (err) {
      setError(err);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="New API token">
      <form onSubmit={onSubmit}>
        <Field label="Name" error={fieldError(error, "name")}>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="ci-pipeline" required />
        </Field>
        <fieldset className="field m-0 border-0 p-0">
          <legend className="label flex items-center gap-1">
            Permissions
            <HelpText>Give a token only what it needs — for example backup:write for a script that triggers backups.</HelpText>
          </legend>
          <div className="card p-2.5 max-h-44 overflow-y-auto grid gap-1.5">
            {available.length === 0 ? (
              <span className="muted text-sm">No permissions available.</span>
            ) : (
              available.map((perm) => (
                <Checkbox
                  key={perm}
                  className="text-sm"
                  label={<code>{perm}</code>}
                  checked={scopes.includes(perm)}
                  onChange={() => toggle(perm)}
                />
              ))
            )}
          </div>
          <p className="field-hint">Leave all unchecked to give the token every permission you have.</p>
          {fieldError(error, "scopes") ? <p className="field-error">{fieldError(error, "scopes")}</p> : null}
        </fieldset>
        <Field
          label="Expires after (hours)"
          hint="Optional. Leave empty for a token that never expires."
          error={ttlError ?? fieldError(error, "ttl_hours")}
        >
          <Input
            type="number"
            min={1}
            value={ttlHours}
            onChange={(e) => setTtlHours(e.target.value)}
            placeholder="never"
          />
        </Field>
        <FormError error={error} fields={["name", "scopes", "ttl_hours"]} fallback="Failed to create token" />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={create.isPending}>
            {create.isPending ? "Creating…" : "Create token"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
