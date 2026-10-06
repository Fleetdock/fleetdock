"use client";

import { useState, type FormEvent } from "react";

import { ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useCreateDatabase } from "@/lib/hooks";

export function CreateDatabaseModal({
  open,
  onClose,
  instances,
  instanceId: presetInstance,
}: {
  open: boolean;
  onClose: () => void;
  instances: { id: string; name: string }[];
  /** Create on this database server (hides the picker). */
  instanceId?: string;
}) {
  const create = useCreateDatabase();
  const [picked, setPicked] = useState("");
  const instanceId = presetInstance ?? picked;
  const setInstanceId = setPicked;
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({ instance_id: instanceId, name });
      setName("");
      onClose();
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Failed to create database",
      );
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Create database">
      <form onSubmit={onSubmit}>
        {presetInstance ? null : (
          <Field label="Database server">
            <select
              className="input"
              value={instanceId}
              onChange={(e) => setInstanceId(e.target.value)}
              required
            >
              <option value="" disabled>
                Choose a database server…
              </option>
              {instances.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name}
                </option>
              ))}
            </select>
          </Field>
        )}
        <Field label="Name" hint="Letters, digits and underscores.">
          <input
            className="input"
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="app_production"
            required
          />
        </Field>
        {!presetInstance && instances.length === 0 ? (
          <p className="text-sm muted">Connect a database server first.</p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div
          className="flex justify-end items-center gap-2"
          style={{ marginTop: ".5rem" }}
        >
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={
              create.isPending || (!presetInstance && instances.length === 0)
            }
          >
            {create.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
