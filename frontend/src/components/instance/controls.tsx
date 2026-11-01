"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { KeyRound, Play, RefreshCw, RotateCw, Square, Trash2 } from "lucide-react";
import { DeleteInstanceModal } from "@/components/delete-instance-modal";
import { useErrorToast } from "@/components/toast";
import { Button, ConfirmModal, Menu } from "@/components/ui";
import { useInstanceLifecycle, useProbeInstance } from "@/lib/hooks";
import type { Instance } from "@/lib/types";

/** CheckNowButton checks the connection — and picks up new databases — now. */
export function CheckNowButton({ instance }: { instance: Instance }) {
  const probe = useProbeInstance();
  const toastError = useErrorToast();
  return (
    <Button
      icon={<RefreshCw size={15} aria-hidden />}
      loading={probe.isPending}
      onClick={() => probe.mutate(instance.id, { onError: toastError("Could not check the server") })}
      title="Check the connection and look for new databases now"
    >
      Check now
    </Button>
  );
}

/**
 * InstanceControls holds the write actions in the page header: start, edit
 * (or add a login), and restart / stop / remove behind a menu.
 */
export function InstanceControls({ instance, onEdit }: { instance: Instance; onEdit: () => void }) {
  const router = useRouter();
  const lifecycle = useInstanceLifecycle();
  const toastError = useErrorToast();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [confirm, setConfirm] = useState<"stop" | "restart" | null>(null);
  const busy = lifecycle.isPending;

  const run = (action: "start" | "stop" | "restart") =>
    lifecycle.mutate(
      { id: instance.id, action },
      { onError: toastError(`Failed to ${action} ${instance.name}`), onSettled: () => setConfirm(null) },
    );

  return (
    <>
      {instance.provisioned && instance.status === "stopped" ? (
        <Button icon={<Play size={15} aria-hidden />} disabled={busy} onClick={() => run("start")}>
          Start
        </Button>
      ) : null}
      <Button icon={<KeyRound size={15} aria-hidden />} disabled={busy} onClick={onEdit}>
        {instance.has_credentials ? "Edit" : "Add login"}
      </Button>
      <Menu
        label={`More actions for ${instance.name}`}
        items={[
          {
            label: "Restart",
            icon: <RotateCw size={15} />,
            onSelect: () => setConfirm("restart"),
            hidden: !instance.provisioned || instance.status === "stopped",
          },
          {
            label: "Stop",
            icon: <Square size={15} />,
            onSelect: () => setConfirm("stop"),
            hidden: !instance.provisioned || instance.status === "stopped",
          },
          { label: "Remove…", icon: <Trash2 size={15} />, danger: true, onSelect: () => setDeleteOpen(true) },
        ]}
      />
      <ConfirmModal
        open={confirm !== null}
        danger={confirm === "stop"}
        title={confirm === "stop" ? `Stop ${instance.name}?` : `Restart ${instance.name}?`}
        confirmLabel={confirm === "stop" ? "Stop server" : "Restart server"}
        busy={busy}
        message={
          confirm === "stop"
            ? "Every application connected to its databases loses its connection until you start it again. No data is deleted."
            : "Connected applications are disconnected for a few seconds while it restarts."
        }
        onConfirm={() => confirm && run(confirm)}
        onCancel={() => setConfirm(null)}
      />
      <DeleteInstanceModal
        instance={deleteOpen ? instance : null}
        onClose={() => setDeleteOpen(false)}
        onDeleted={() => router.push("/databases")}
      />
    </>
  );
}
