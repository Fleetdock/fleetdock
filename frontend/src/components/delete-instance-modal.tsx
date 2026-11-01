"use client";

import { useState } from "react";

import { Checkbox, ConfirmModal, ErrorText } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useDeleteInstance } from "@/lib/hooks";
import type { Instance } from "@/lib/types";

/**
 * DeleteInstanceModal removes an instance. For a provisioned instance the
 * data volume is kept unless explicitly ticked — and deleting it requires
 * typing the instance name, because it destroys the data irreversibly.
 */
export function DeleteInstanceModal({
  instance,
  onClose,
  onDeleted,
}: {
  instance: Instance | null;
  onClose: () => void;
  onDeleted?: () => void;
}) {
  const del = useDeleteInstance();
  const [removeVolume, setRemoveVolume] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function close() {
    setRemoveVolume(false);
    setError(null);
    onClose();
  }

  return (
    <ConfirmModal
      open={instance !== null}
      danger
      title={`Remove ${instance?.name ?? "this database server"}?`}
      confirmLabel={removeVolume ? "Remove and delete its data" : "Remove"}
      confirmText={removeVolume ? instance?.name : undefined}
      busy={del.isPending}
      message={
        <div className="stack-sm">
          {instance?.provisioned ? (
            <>
              <p className="m-0">Its Docker container is stopped and removed.</p>
              <Checkbox
                checked={removeVolume}
                onChange={(e) => setRemoveVolume(e.target.checked)}
                label={
                  <>
                    Also delete the data volume — <strong>permanently destroys all of its databases</strong>
                  </>
                }
              />
              {!removeVolume ? <p className="m-0 text-muted">The volume is kept and can be reattached later.</p> : null}
            </>
          ) : (
            <p className="m-0">It is removed from Fleetdock only; the database server itself is not touched.</p>
          )}
          <ErrorText message={error ?? undefined} />
        </div>
      }
      onConfirm={async () => {
        if (!instance) return;
        setError(null);
        try {
          await del.mutateAsync({ id: instance.id, removeVolume: instance.provisioned && removeVolume });
          close();
          onDeleted?.();
        } catch (err) {
          setError(err instanceof ApiError ? err.message : "Failed to remove the database server");
        }
      }}
      onCancel={close}
    />
  );
}
