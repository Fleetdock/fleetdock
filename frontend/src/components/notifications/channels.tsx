"use client";

import { Pencil, Plug, Plus, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";

import { FormError } from "@/components/access/form-error";
import { useConfirm } from "@/components/confirm";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { useErrorToast, useToast } from "@/components/toast";
import { Button, Field, IconButton, Input, Modal, SectionHeader, Select, StatusBadge, Switch } from "@/components/ui";
import { fieldError, friendlyError } from "@/lib/errors";
import { useChannels, useCreateChannel, useDeleteChannel, useTestChannel, useUpdateChannel } from "@/lib/hooks";
import type { ChannelType, NotificationChannel } from "@/lib/types";

import { CHANNEL_LABEL } from "./labels";

/** ChannelsSection lists where notifications go (email, Slack, webhooks). */
export function ChannelsSection({ canWrite }: { canWrite: boolean }) {
  const { data, isLoading, error, refetch } = useChannels();
  const del = useDeleteChannel();
  const toastError = useErrorToast();
  const confirm = useConfirm();
  const { push } = useToast();
  const test = useTestChannel();
  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<NotificationChannel | null>(null);

  const onTest = useCallback(
    async (id: string, name: string) => {
      try {
        await test.mutateAsync(id);
        push("success", `${name}: test notification sent`);
      } catch (err) {
        push("error", `${name}: ${friendlyError(err, "the test failed")}`);
      }
    },
    [push, test],
  );

  const columns = useMemo(() => {
    const cols: DataTableColumn<NotificationChannel>[] = [
      { id: "name", header: "Name", className: "font-medium", render: (c) => c.name },
      { id: "type", header: "Type", className: "muted", render: (c) => CHANNEL_LABEL[c.type] ?? c.type },
      {
        id: "target",
        header: "Target",
        className: "muted",
        hideOnMobile: true,
        render: (c) => (
          <span className="block truncate max-w-80 mono text-xs" title={channelTarget(c)}>
            {channelTarget(c)}
          </span>
        ),
      },
      { id: "enabled", header: "Status", render: (c) => <StatusBadge status={c.enabled ? "active" : "stopped"} /> },
    ];
    if (canWrite) {
      cols.push({
        id: "actions",
        header: "Actions",
        align: "right",
        render: (c) => (
          <div className="flex items-center justify-end gap-1">
            <Button size="sm" icon={<Plug size={15} aria-hidden />} onClick={() => onTest(c.id, c.name)} disabled={test.isPending}>
              Test
            </Button>
            <IconButton label={`Edit ${c.name}`} onClick={() => setEditTarget(c)}>
              <Pencil size={15} aria-hidden />
            </IconButton>
            <IconButton
              label={`Delete ${c.name}`}
              variant="danger"
              onClick={async () => {
                const ok = await confirm({
                  title: `Delete "${c.name}"?`,
                  message: "Alert rules that use this channel stop sending to it.",
                  confirmLabel: "Delete channel",
                  danger: true,
                });
                if (ok) del.mutate(c.id, { onError: toastError("Failed to delete the channel") });
              }}
              disabled={del.isPending}
            >
              <Trash2 size={15} aria-hidden />
            </IconButton>
          </div>
        ),
      });
    }
    return cols;
  }, [toastError, canWrite, confirm, del, onTest, test]);

  const addButton = canWrite ? (
    <Button variant="primary" size="sm" icon={<Plus size={15} aria-hidden />} onClick={() => setAddOpen(true)}>
      Add channel
    </Button>
  ) : null;

  return (
    <section className="section" aria-labelledby="channels-title">
      <SectionHeader
        title={<span id="channels-title">Channels</span>}
        description="Where alerts are delivered."
        actions={addButton}
      />
      <DataTable<NotificationChannel>
        columns={columns}
        rows={data?.items ?? []}
        rowKey={(c) => c.id}
        isLoading={isLoading}
        error={error ? friendlyError(error) : undefined}
        errorTitle="Could not load channels"
        onRetry={() => void refetch()}
        emptyTitle="No channels yet"
        emptyHint="Add a webhook, Slack or email channel to receive notifications."
        emptyAction={addButton}
      />
      <ChannelModal mode="create" open={addOpen} onClose={() => setAddOpen(false)} />
      <ChannelModal mode="edit" channel={editTarget} open={editTarget !== null} onClose={() => setEditTarget(null)} />
    </section>
  );
}

function channelTarget(c: NotificationChannel): string {
  if (c.type === "email") return c.config.to ?? "—";
  if (c.type === "slack") return c.config.webhook_url ?? "—";
  return c.config.url ?? "—";
}

type ChannelModalProps =
  | { mode: "create"; channel?: undefined; open: boolean; onClose: () => void }
  | { mode: "edit"; channel: NotificationChannel | null; open: boolean; onClose: () => void };

export function ChannelModal({ mode, channel, open, onClose }: ChannelModalProps) {
  const create = useCreateChannel();
  const update = useUpdateChannel();
  const isEdit = mode === "edit";

  const [name, setName] = useState("");
  const [type, setType] = useState<ChannelType>("webhook");
  const [url, setUrl] = useState("");
  const [slackUrl, setSlackUrl] = useState("");
  const [to, setTo] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!open) return;
    if (isEdit && channel) {
      setName(channel.name);
      setType(channel.type);
      setUrl(channel.config.url ?? "");
      setSlackUrl(channel.config.webhook_url ?? "");
      setTo(channel.config.to ?? "");
      setEnabled(channel.enabled);
    } else if (!isEdit) {
      setName("");
      setType("webhook");
      setUrl("");
      setSlackUrl("");
      setTo("");
      setEnabled(true);
    }
    setError(null);
  }, [open, isEdit, channel]);

  const pending = create.isPending || update.isPending;

  function buildConfig(): Record<string, string> {
    if (type === "webhook") return { url };
    if (type === "slack") return { webhook_url: slackUrl };
    return { to };
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const payload = { name, type, config: buildConfig(), enabled };
    try {
      if (isEdit && channel) {
        await update.mutateAsync({ id: channel.id, ...payload });
      } else {
        await create.mutateAsync(payload);
      }
      onClose();
    } catch (err) {
      setError(err);
    }
  }

  const keepHint = isEdit ? "Stored securely. Leave blank to keep the current one." : "Stored securely.";
  const configError = fieldError(error, "config");

  return (
    <Modal open={open} onClose={onClose} title={isEdit ? "Edit channel" : "Add channel"}>
      <form onSubmit={onSubmit}>
        <div className="form-grid">
          <Field label="Name" error={fieldError(error, "name")}>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="ops-alerts" required />
          </Field>
          <Field label="Type" error={fieldError(error, "type")}>
            <Select value={type} onChange={(e) => setType(e.target.value as ChannelType)}>
              <option value="webhook">Webhook</option>
              <option value="slack">Slack</option>
              <option value="email">Email</option>
            </Select>
          </Field>
        </div>
        {type === "webhook" ? (
          <Field label="Webhook URL" hint={keepHint} error={configError}>
            <Input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://example.com/hook"
              required={!isEdit}
            />
          </Field>
        ) : null}
        {type === "slack" ? (
          <Field label="Slack incoming webhook URL" hint={keepHint} error={configError}>
            <Input
              value={slackUrl}
              onChange={(e) => setSlackUrl(e.target.value)}
              placeholder="https://hooks.slack.com/services/…"
              required={!isEdit}
            />
          </Field>
        ) : null}
        {type === "email" ? (
          <Field
            label="Recipient address"
            hint="Email requires SMTP to be configured on the server (FLEETDOCK_SMTP_HOST)."
            error={configError}
          >
            <Input
              type="email"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              placeholder="oncall@example.com"
              required
            />
          </Field>
        ) : null}
        <Switch label="Enabled" checked={enabled} onCheckedChange={setEnabled} />
        <div className="mt-3">
          <FormError
            error={error}
            fields={["name", "type", "config"]}
            fallback={`Failed to ${isEdit ? "update" : "add"} channel`}
          />
        </div>
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={pending}>
            {pending ? "Saving…" : isEdit ? "Save changes" : "Add channel"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
