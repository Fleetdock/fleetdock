"use client";

import { useState, type FormEvent } from "react";

import {
  Button,
  ButtonLink,
  Callout,
  Checkbox,
  ErrorText,
  Field,
  Input,
  Modal,
  Select,
} from "@/components/ui";

import { ApiError } from "@/lib/api";
import { useDestinations, useInstances, useStartMove } from "@/lib/hooks";
import type { Database } from "@/lib/types";

export function MoveDatabaseModal({
  database,
  onClose,
}: {
  database: Database | null;
  onClose: () => void;
}) {
  const start = useStartMove();
  const { data: instances } = useInstances();
  const { data: destinations } = useDestinations();
  const [targetInstance, setTargetInstance] = useState("");
  const [targetName, setTargetName] = useState("");
  const [destination, setDestination] = useState("");
  const [dropSource, setDropSource] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [started, setStarted] = useState<string | null>(null);
  const [operationId, setOperationId] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!database) return;
    setError(null);
    try {
      const res = await start.mutateAsync({
        source_database_id: database.id,
        target_instance_id: targetInstance,
        target_database: targetName || undefined,
        destination_id: destination,
        drop_source: dropSource,
      });
      setOperationId(res.operation_id);
      setStarted(
        "Move started — it will back up, then restore and verify in the background. Follow it in Activity.",
      );
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to start move");
    }
  }

  if (!database) return null;
  return (
    <Modal open onClose={onClose} title={`Copy, move or rename "${database.name}"`}>
      {started ? (
        <div>
          <Callout tone="success" role="status">
            {started}
          </Callout>
          <div className="modal-actions">
            <ButtonLink href={operationId ? `/activity/${operationId}` : "/activity"}>
              View operation
            </ButtonLink>
            <Button variant="primary" onClick={onClose}>
              Done
            </Button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="Copy to database server">
            <Select
              value={targetInstance}
              onChange={(e) => setTargetInstance(e.target.value)}
              required
            >
              <option value="">Select a database server…</option>
              {(instances?.items ?? [])
                .filter(
                  (i) => i.id !== database.instance_id && i.has_credentials,
                )
                .map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.name} ({i.engine})
                  </option>
                ))}
            </Select>
          </Field>
          <Field label="Target database name (optional)">
            <Input
              value={targetName}
              onChange={(e) => setTargetName(e.target.value)}
              placeholder={database.name}
            />
          </Field>
          <Field label="Backup storage to use">
            <Select
              value={destination}
              onChange={(e) => setDestination(e.target.value)}
              required
            >
              <option value="">Select backup storage…</option>
              {(destinations?.items ?? []).map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </Select>
          </Field>
          <Checkbox
            className="mt-1 mb-2.5"
            checked={dropSource}
            onChange={(e) => setDropSource(e.target.checked)}
            label="Drop the source database after a successful, verified move (cutover)"
          />
          <p className="text-sm muted">
            The source is backed up and restored to the target (verifying the
            checksum and table count), in the background. Leave &quot;drop the
            source&quot; unticked to <strong>copy</strong>; pick the same instance
            with a new name and tick it to <strong>rename</strong>; pick another
            instance to <strong>move</strong>.
          </p>
          <ErrorText message={error ?? undefined} />
          <div className="modal-actions">
            <Button onClick={onClose}>Cancel</Button>
            <Button type="submit" variant="primary" loading={start.isPending}>
              {start.isPending ? "Starting…" : "Start move"}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
