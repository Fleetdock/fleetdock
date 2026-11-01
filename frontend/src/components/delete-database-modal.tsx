"use client";

import { useState } from "react";

import { Button, ErrorText, Field, Input, Modal, cx } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useDeleteDatabase } from "@/lib/hooks";
import type { Database } from "@/lib/types";

type DeleteMode = "metadata" | "physical";

export function DeleteDatabaseModal({
  database,
  onClose,
  onDeleted,
}: {
  database: Database | null;
  onClose: () => void;
  onDeleted?: () => void;
}) {
  const del = useDeleteDatabase();
  const [mode, setMode] = useState<DeleteMode>("metadata");
  const [error, setError] = useState<string | null>(null);
  const [typed, setTyped] = useState("");

  // The owning instance travels with the database, so a physical drop can no
  // longer be mis-enabled because the instance was missing from a client-side
  // lookup.
  const canDrop = database?.instance?.has_credentials ?? false;

  function close() {
    setMode("metadata");
    setError(null);
    setTyped("");
    onClose();
  }

  async function onConfirm() {
    if (!database) return;
    setError(null);
    try {
      await del.mutateAsync({ id: database.id, drop: mode === "physical" });
      close();
      onDeleted?.();
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Failed to remove database",
      );
    }
  }

  if (!database) return null;

  // System databases are imported so they can be browsed and backed up, but the
  // control plane depends on them — the delete path refuses them server-side too.
  if (database.system) {
    return (
      <Modal
        open
        onClose={close}
        title={`"${database.name}" is a system database`}
      >
        <div className="flex flex-col gap-3">
          <p className="text-sm">
            This database belongs to the engine itself. Fleetdock connects to it
            for administration, so it cannot be dropped or removed from the
            control plane. You can still browse it and back it up.
          </p>
          <div className="mt-1 modal-actions">
            <Button onClick={close}>Close</Button>
          </div>
        </div>
      </Modal>
    );
  }

  return (
    <Modal open onClose={close} title={`Remove database "${database.name}"?`}>
      <div className="flex flex-col gap-3">
        <label className="flex items-start gap-2 text-sm cursor-pointer">
          <input
            type="radio"
            name="delete-mode"
            checked={mode === "metadata"}
            onChange={() => setMode("metadata")}
            className="mt-1"
          />
          <span>
            <span className="font-medium">Remove from Fleetdock only</span>
            <span className="block mt-0.5 muted">
              Fleetdock stops showing it; the data on the server is not touched.
              If the database still exists there, it is picked up again after 7
              days.
            </span>
          </span>
        </label>
        <label
          className={cx(
            "flex items-start gap-2 text-sm",
            canDrop ? "cursor-pointer" : "cursor-not-allowed opacity-55",
          )}
        >
          <input
            type="radio"
            name="delete-mode"
            checked={mode === "physical"}
            onChange={() => setMode("physical")}
            disabled={!canDrop}
            className="mt-1"
          />
          <span>
            <span className="font-medium">
              Also delete it from the database server
            </span>
            <span className="block mt-0.5 muted">
              Permanently deletes the database and all its data (DROP DATABASE).
              This cannot be undone.
            </span>
          </span>
        </label>
        {!canDrop ? (
          <p className="text-sm muted">
            Add an admin login to the database server to delete the data too.
          </p>
        ) : null}
        {mode === "physical" ? (
          <Field
            label={
              <>
                Type <code>{database.name}</code> to confirm
              </>
            }
          >
            <Input
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoComplete="off"
              autoFocus
            />
          </Field>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="mt-1 modal-actions">
          <Button onClick={close}>Cancel</Button>
          <Button
            variant={mode === "physical" ? "danger-solid" : "danger"}
            onClick={onConfirm}
            loading={del.isPending}
            disabled={mode === "physical" && typed !== database.name}
          >
            {del.isPending
              ? "Removing…"
              : mode === "physical"
                ? "Drop database"
                : "Remove"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
