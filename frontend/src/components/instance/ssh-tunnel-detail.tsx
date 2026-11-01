"use client";

import { useState } from "react";

import { useErrorToast } from "@/components/toast";
import { Button, ConfirmModal } from "@/components/ui";
import { useUpdateInstance } from "@/lib/hooks";
import type { Instance } from "@/lib/types";

/**
 * SSHTunnelDetail shows the bastion and its pinned host key. Resetting the pin
 * is how an expected host key change (a rebuilt bastion) is accepted.
 */
export function SSHTunnelDetail({ instance, canWrite }: { instance: Instance; canWrite: boolean }) {
  const t = instance.ssh_tunnel!;
  const update = useUpdateInstance();
  const toastError = useErrorToast();
  const [confirm, setConfirm] = useState(false);

  async function reset() {
    try {
      await update.mutateAsync({ id: instance.id, reset_ssh_host_key: true });
      setConfirm(false);
    } catch (err) {
      toastError("Couldn't reset the host key")(err);
    }
  }

  return (
    <span className="block min-w-0">
      <span className="mono">
        {t.username}@{t.host}
        {t.port !== 22 ? `:${t.port}` : ""}
      </span>
      <span className="block break-all text-sm text-muted">
        {t.host_key_fingerprint ? (
          <>
            Host key <code>{t.host_key_fingerprint}</code>
            {canWrite ? (
              <Button variant="ghost" size="sm" className="ml-1" onClick={() => setConfirm(true)}>
                Reset
              </Button>
            ) : null}
          </>
        ) : (
          "Host key is pinned on the first connection."
        )}
      </span>
      <ConfirmModal
        open={confirm}
        title="Reset the SSH host key?"
        confirmLabel="Reset host key"
        busy={update.isPending}
        message="Fleetdock forgets the pinned key and trusts whatever key the SSH server presents next time. Only do this if you know the server's key changed (for example, it was rebuilt)."
        onConfirm={reset}
        onCancel={() => setConfirm(false)}
      />
    </span>
  );
}
