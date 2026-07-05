"use client";

import { useState } from "react";

import { ErrorText, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useDeleteDatabase } from "@/lib/hooks";
import type { Database, Instance } from "@/lib/types";

type DeleteMode = "metadata" | "physical";

export function DeleteDatabaseModal({
  database,
  instance,
  onClose,
  onDeleted,
}: {
  database: Database | null;
  instance: Instance | undefined;
  onClose: () => void;
  onDeleted?: () => void;
}) {
  const del = useDeleteDatabase();
  const [mode, setMode] = useState<DeleteMode>("metadata");
  const [error, setError] = useState<string | null>(null);

  const canDrop = instance?.has_credentials ?? false;

  function close() {
    setMode("metadata");
    setError(null);
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
      setError(err instanceof ApiError ? err.message : "Failed to remove database");
    }
  }

  if (!database) return null;

  return (
    <Modal open onClose={close} title={`Remove database "${database.name}"?`}>
      <div className="flex flex-col gap-3">
        <label className="flex items-start gap-2 text-sm" style={{ cursor: "pointer" }}>
          <input
            type="radio"
            name="delete-mode"
            checked={mode === "metadata"}
            onChange={() => setMode("metadata")}
            style={{ marginTop: ".2rem" }}
          />
          <span>
            <span className="font-medium">Remove from control plane only</span>
            <span className="muted block" style={{ marginTop: ".15rem" }}>
              The database on the server is not touched. The record enters a 7-day recovery window.
            </span>
          </span>
        </label>
        <label
          className="flex items-start gap-2 text-sm"
          style={{ cursor: canDrop ? "pointer" : "not-allowed", opacity: canDrop ? 1 : 0.55 }}
        >
          <input
            type="radio"
            name="delete-mode"
            checked={mode === "physical"}
            onChange={() => setMode("physical")}
            disabled={!canDrop}
            style={{ marginTop: ".2rem" }}
          />
          <span>
            <span className="font-medium">Also drop database on the instance</span>
            <span className="muted block" style={{ marginTop: ".15rem" }}>
              Permanently runs DROP DATABASE on the instance. This cannot be undone.
            </span>
          </span>
        </label>
        {!canDrop ? (
          <p className="muted text-sm">Add admin credentials to the instance to enable physical drop.</p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".25rem" }}>
          <button type="button" className="btn" onClick={close}>Cancel</button>
          <button
            type="button"
            className="btn btn-danger"
            onClick={onConfirm}
            disabled={del.isPending}
          >
            {del.isPending ? "Removing…" : mode === "physical" ? "Drop database" : "Remove"}
          </button>
        </div>
      </div>
    </Modal>
  );
}
