"use client";

import { useState } from "react";

import { Button, Callout, ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useUpdateAllowedCIDRs } from "@/lib/hooks";

/** parseCIDRs splits comma/newline separated input into trimmed entries. */
export function parseCIDRs(input: string): string[] {
  return input
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export const ALLOW_ANYWHERE = "0.0.0.0/0";

/**
 * AllowlistEditor changes the allowed networks of a live endpoint.
 *
 * This is the recovery path for an allowlist that turns away real clients.
 * Disabling and re-enabling public access would allocate a different port and
 * break every client that already has the old connection URL.
 */
export function AllowlistEditor({
  databaseId,
  current,
  onClose,
}: {
  databaseId: string;
  current: string[];
  onClose: () => void;
}) {
  const update = useUpdateAllowedCIDRs(databaseId);
  const [input, setInput] = useState(current.join(", "));
  const [error, setError] = useState<string | null>(null);
  const [confirmAnywhere, setConfirmAnywhere] = useState(false);

  const parsed = parseCIDRs(input);
  const opensToInternet = parsed.includes(ALLOW_ANYWHERE);

  async function onSave() {
    setError(null);

    if (parsed.length === 0) {
      setError("Enter at least one network. To allow any address, enter 0.0.0.0/0 explicitly.");
      return;
    }
    if (opensToInternet && !confirmAnywhere) {
      setConfirmAnywhere(true);
      return;
    }

    try {
      await update.mutateAsync({ allowed_cidrs: parsed });
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Failed to update allowed networks");
    }
  }

  return (
    <Modal open onClose={onClose} title="Edit allowed networks">
      <Field
        label="Allowed networks (CIDR or IP, comma-separated)"
        hint="A bare address is treated as a single host. The port stays the same, so existing clients keep working."
      >
        <input
          className="input"
          value={input}
          onChange={(e) => {
            setInput(e.target.value);
            setConfirmAnywhere(false);
          }}
          placeholder="203.0.113.7, 10.0.0.0/8"
        />
      </Field>

      {confirmAnywhere ? (
        <Callout tone="warning" role="alert" title="0.0.0.0/0 allows connections from any address on the internet.">
          Press Save again to confirm.
        </Callout>
      ) : null}

      <ErrorText message={error ?? undefined} />

      <div className="modal-actions">
        <Button disabled={update.isPending} onClick={onClose}>
          Cancel
        </Button>
        <Button variant="primary" loading={update.isPending} onClick={onSave}>
          {update.isPending ? "Saving…" : "Save"}
        </Button>
      </div>
    </Modal>
  );
}
