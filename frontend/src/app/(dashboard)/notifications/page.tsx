"use client";

import { useEffect, useMemo, useState, type FormEvent } from "react";

import { Bell, Pencil, Plug, Plus, Trash2 } from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/data-table";
import { ErrorText, Field, Modal, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useAlertRules,
  useCan,
  useChannels,
  useCreateAlertRule,
  useCreateChannel,
  useDeleteAlertRule,
  useDeleteChannel,
  useServers,
  useTestChannel,
  useUpdateAlertRule,
  useUpdateChannel,
} from "@/lib/hooks";
import type { AlertRule, ChannelType, NotificationChannel } from "@/lib/types";

const CHANNEL_LABEL: Record<string, string> = { email: "Email", slack: "Slack", webhook: "Webhook" };
const METRIC_LABEL: Record<string, string> = {
  cpu_pct: "CPU %",
  mem_used_pct: "Memory used %",
  disk_used_pct: "Disk used %",
  connections: "Connections",
};
const COMPARATOR_LABEL: Record<string, string> = { gt: ">", gte: "≥", lt: "<", lte: "≤" };

export default function NotificationsPage() {
  const can = useCan();
  const canWrite = can("notification:write");
  const [notice, setNotice] = useState<string | null>(null);

  return (
    <div>
      <div style={{ marginBottom: "1.1rem" }}>
        <h1 className="text-xl font-semibold">Notifications</h1>
        <p className="muted text-sm">Delivery channels and alert rules for backup failures, offline servers and metric thresholds.</p>
      </div>

      {notice ? (
        <div className="card" style={{ padding: ".7rem .9rem", marginBottom: ".9rem" }}>
          <span className="text-sm">{notice}</span>
        </div>
      ) : null}

      <ChannelsSection canWrite={canWrite} onNotice={setNotice} />
      <RulesSection canWrite={canWrite} />
    </div>
  );
}

// ---------------- Channels ----------------

function ChannelsSection({ canWrite, onNotice }: { canWrite: boolean; onNotice: (s: string) => void }) {
  const { data, isLoading, error } = useChannels();
  const del = useDeleteChannel();
  const test = useTestChannel();
  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<NotificationChannel | null>(null);

  async function onTest(id: string, name: string) {
    try {
      await test.mutateAsync(id);
      onNotice(`"${name}": test notification delivered.`);
    } catch (err) {
      onNotice(err instanceof ApiError ? `"${name}": ${err.message}` : "Test failed");
    }
  }

  const columns = useMemo(() => {
    const cols: DataTableColumn<NotificationChannel>[] = [
      { id: "name", header: "Name", className: "font-medium", render: (c) => c.name },
      { id: "type", header: "Type", className: "muted", render: (c) => CHANNEL_LABEL[c.type] ?? c.type },
      { id: "target", header: "Target", className: "muted", render: (c) => channelTarget(c) },
      { id: "enabled", header: "Status", render: (c) => <StatusBadge status={c.enabled ? "active" : "stopped"} /> },
    ];
    if (canWrite) {
      cols.push({
        id: "actions",
        header: "Actions",
        align: "right",
        render: (c) => (
          <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
            <button className="btn btn-sm" onClick={() => onTest(c.id, c.name)} disabled={test.isPending}><Plug size={15} /> Test</button>
            <button className="btn btn-sm" onClick={() => setEditTarget(c)} aria-label="Edit"><Pencil size={15} /></button>
            <button
              className="btn btn-sm btn-danger"
              onClick={() => confirm(`Delete channel "${c.name}"?`) && del.mutate(c.id)}
              disabled={del.isPending}
              aria-label="Delete"
            >
              <Trash2 size={15} />
            </button>
          </div>
        ),
      });
    }
    return cols;
  }, [canWrite, del, test]);

  return (
    <section style={{ marginBottom: "1.8rem" }}>
      <div className="flex items-center justify-between" style={{ marginBottom: ".7rem" }}>
        <h2 className="font-semibold">Channels</h2>
        {canWrite ? (
          <button className="btn btn-primary btn-sm" onClick={() => setAddOpen(true)}><Plus size={15} /> Add channel</button>
        ) : null}
      </div>
      <DataTable<NotificationChannel>
        columns={columns}
        rows={data?.items ?? []}
        rowKey={(c) => c.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load channels"
        emptyTitle="No channels yet"
        emptyHint="Add a webhook, Slack or email channel to receive notifications."
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

function ChannelModal({ mode, channel, open, onClose }: ChannelModalProps) {
  const create = useCreateChannel();
  const update = useUpdateChannel();
  const isEdit = mode === "edit";

  const [name, setName] = useState("");
  const [type, setType] = useState<ChannelType>("webhook");
  const [url, setUrl] = useState("");
  const [slackUrl, setSlackUrl] = useState("");
  const [to, setTo] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState<string | null>(null);

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
      setError(err instanceof ApiError ? err.message : `Failed to ${isEdit ? "update" : "add"} channel`);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={isEdit ? "Edit channel" : "Add channel"}>
      <form onSubmit={onSubmit}>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Name">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="ops-alerts" required />
          </Field>
          <Field label="Type">
            <select className="input" value={type} onChange={(e) => setType(e.target.value as ChannelType)}>
              <option value="webhook">Webhook</option>
              <option value="slack">Slack</option>
              <option value="email">Email</option>
            </select>
          </Field>
        </div>
        {type === "webhook" ? (
          <Field label="Webhook URL">
            <input className="input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.com/hook" required={!isEdit} />
          </Field>
        ) : null}
        {type === "slack" ? (
          <Field label="Slack incoming webhook URL">
            <input className="input" value={slackUrl} onChange={(e) => setSlackUrl(e.target.value)} placeholder="https://hooks.slack.com/services/…" required={!isEdit} />
          </Field>
        ) : null}
        {type === "email" ? (
          <Field label="Recipient address">
            <input className="input" type="email" value={to} onChange={(e) => setTo(e.target.value)} placeholder="oncall@example.com" required />
          </Field>
        ) : null}
        <Field label="Status">
          <select className="input" value={enabled ? "yes" : "no"} onChange={(e) => setEnabled(e.target.value === "yes")}>
            <option value="yes">Enabled</option>
            <option value="no">Disabled</option>
          </select>
        </Field>
        <p className="muted text-sm">
          {type === "email"
            ? "Email requires SMTP to be configured on the server (MDCP_SMTP_HOST)."
            : "The URL is stored securely; leave it blank when editing to keep the current one."}
        </p>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending}>
            {pending ? "Saving…" : isEdit ? "Save changes" : "Add channel"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

// ---------------- Alert rules ----------------

function RulesSection({ canWrite }: { canWrite: boolean }) {
  const { data, isLoading, error } = useAlertRules();
  const { data: channels } = useChannels();
  const del = useDeleteAlertRule();
  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<AlertRule | null>(null);

  const channelName = useMemo(() => {
    const m = new Map<string, string>();
    channels?.items.forEach((c) => m.set(c.id, c.name));
    return m;
  }, [channels]);

  const columns = useMemo(() => {
    const cols: DataTableColumn<AlertRule>[] = [
      { id: "name", header: "Name", className: "font-medium", render: (r) => r.name },
      {
        id: "condition",
        header: "Condition",
        className: "muted",
        render: (r) => `${METRIC_LABEL[r.metric] ?? r.metric} ${COMPARATOR_LABEL[r.comparator] ?? r.comparator} ${r.threshold} for ${r.for_seconds}s`,
      },
      { id: "severity", header: "Severity", render: (r) => <StatusBadge status={r.severity === "critical" ? "failed" : r.severity === "warning" ? "pending" : "canceled"} /> },
      {
        id: "channels",
        header: "Channels",
        className: "muted",
        render: (r) => (r.channel_ids.length ? r.channel_ids.map((id) => channelName.get(id) ?? "?").join(", ") : "all"),
      },
      { id: "enabled", header: "Status", render: (r) => <StatusBadge status={r.enabled ? "active" : "stopped"} /> },
    ];
    if (canWrite) {
      cols.push({
        id: "actions",
        header: "Actions",
        align: "right",
        render: (r) => (
          <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
            <button className="btn btn-sm" onClick={() => setEditTarget(r)} aria-label="Edit"><Pencil size={15} /></button>
            <button
              className="btn btn-sm btn-danger"
              onClick={() => confirm(`Delete rule "${r.name}"?`) && del.mutate(r.id)}
              disabled={del.isPending}
              aria-label="Delete"
            >
              <Trash2 size={15} />
            </button>
          </div>
        ),
      });
    }
    return cols;
  }, [canWrite, channelName, del]);

  return (
    <section>
      <div className="flex items-center justify-between" style={{ marginBottom: ".7rem" }}>
        <h2 className="font-semibold flex items-center gap-2"><Bell size={16} /> Alert rules</h2>
        {canWrite ? (
          <button className="btn btn-primary btn-sm" onClick={() => setAddOpen(true)}><Plus size={15} /> Add rule</button>
        ) : null}
      </div>
      <DataTable<AlertRule>
        columns={columns}
        rows={data?.items ?? []}
        rowKey={(r) => r.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load alert rules"
        emptyTitle="No alert rules yet"
        emptyHint="Create a rule to be notified when a server metric crosses a threshold."
      />
      <RuleModal mode="create" open={addOpen} onClose={() => setAddOpen(false)} />
      <RuleModal mode="edit" rule={editTarget} open={editTarget !== null} onClose={() => setEditTarget(null)} />
    </section>
  );
}

type RuleModalProps =
  | { mode: "create"; rule?: undefined; open: boolean; onClose: () => void }
  | { mode: "edit"; rule: AlertRule | null; open: boolean; onClose: () => void };

function RuleModal({ mode, rule, open, onClose }: RuleModalProps) {
  const create = useCreateAlertRule();
  const update = useUpdateAlertRule();
  const { data: servers } = useServers();
  const { data: channels } = useChannels();
  const isEdit = mode === "edit";

  const [name, setName] = useState("");
  const [targetType, setTargetType] = useState("global");
  const [targetId, setTargetId] = useState("");
  const [metric, setMetric] = useState("cpu_pct");
  const [comparator, setComparator] = useState("gt");
  const [threshold, setThreshold] = useState("80");
  const [forSeconds, setForSeconds] = useState("60");
  const [severity, setSeverity] = useState("warning");
  const [channelIds, setChannelIds] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    if (isEdit && rule) {
      setName(rule.name);
      setTargetType(rule.target_type);
      setTargetId(rule.target_id ?? "");
      setMetric(rule.metric);
      setComparator(rule.comparator);
      setThreshold(String(rule.threshold));
      setForSeconds(String(rule.for_seconds));
      setSeverity(rule.severity);
      setChannelIds(rule.channel_ids);
      setEnabled(rule.enabled);
    } else if (!isEdit) {
      setName("");
      setTargetType("global");
      setTargetId("");
      setMetric("cpu_pct");
      setComparator("gt");
      setThreshold("80");
      setForSeconds("60");
      setSeverity("warning");
      setChannelIds([]);
      setEnabled(true);
    }
    setError(null);
  }, [open, isEdit, rule]);

  const pending = create.isPending || update.isPending;

  function toggleChannel(id: string) {
    setChannelIds((prev) => (prev.includes(id) ? prev.filter((c) => c !== id) : [...prev, id]));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const payload = {
      name,
      target_type: targetType,
      target_id: targetType === "server" ? targetId : undefined,
      metric,
      comparator,
      threshold: Number(threshold),
      for_seconds: Number(forSeconds),
      severity,
      channel_ids: channelIds,
      enabled,
    };
    try {
      if (isEdit && rule) {
        await update.mutateAsync({ id: rule.id, ...payload });
      } else {
        await create.mutateAsync(payload);
      }
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : `Failed to ${isEdit ? "update" : "add"} rule`);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={isEdit ? "Edit alert rule" : "New alert rule"}>
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="High CPU" required />
        </Field>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Scope">
            <select className="input" value={targetType} onChange={(e) => setTargetType(e.target.value)}>
              <option value="global">Any server</option>
              <option value="server">Specific server</option>
            </select>
          </Field>
          {targetType === "server" ? (
            <Field label="Server">
              <select className="input" value={targetId} onChange={(e) => setTargetId(e.target.value)} required>
                <option value="">Select…</option>
                {servers?.items.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            </Field>
          ) : <div />}
        </div>
        <div className="grid" style={{ gridTemplateColumns: "1.3fr .7fr 1fr", gap: ".75rem" }}>
          <Field label="Metric">
            <select className="input" value={metric} onChange={(e) => setMetric(e.target.value)}>
              {Object.entries(METRIC_LABEL).map(([v, l]) => (
                <option key={v} value={v}>{l}</option>
              ))}
            </select>
          </Field>
          <Field label="When">
            <select className="input" value={comparator} onChange={(e) => setComparator(e.target.value)}>
              <option value="gt">&gt;</option>
              <option value="gte">≥</option>
              <option value="lt">&lt;</option>
              <option value="lte">≤</option>
            </select>
          </Field>
          <Field label="Threshold">
            <input className="input" type="number" step="any" value={threshold} onChange={(e) => setThreshold(e.target.value)} required />
          </Field>
        </div>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="For (seconds)">
            <input className="input" type="number" min={0} value={forSeconds} onChange={(e) => setForSeconds(e.target.value)} required />
          </Field>
          <Field label="Severity">
            <select className="input" value={severity} onChange={(e) => setSeverity(e.target.value)}>
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="critical">Critical</option>
            </select>
          </Field>
        </div>
        <Field label="Channels (none = all enabled)">
          <div className="card" style={{ padding: ".5rem .7rem", maxHeight: "8rem", overflowY: "auto" }}>
            {channels?.items.length ? (
              channels.items.map((c) => (
                <label key={c.id} className="flex items-center gap-2" style={{ padding: ".2rem 0", cursor: "pointer" }}>
                  <input type="checkbox" checked={channelIds.includes(c.id)} onChange={() => toggleChannel(c.id)} />
                  <span className="text-sm">{c.name}</span>
                  <span className="muted text-sm">({CHANNEL_LABEL[c.type] ?? c.type})</span>
                </label>
              ))
            ) : (
              <span className="muted text-sm">No channels yet — add one above.</span>
            )}
          </div>
        </Field>
        <Field label="Status">
          <select className="input" value={enabled ? "yes" : "no"} onChange={(e) => setEnabled(e.target.value === "yes")}>
            <option value="yes">Enabled</option>
            <option value="no">Disabled</option>
          </select>
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending}>
            {pending ? "Saving…" : isEdit ? "Save changes" : "Create rule"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
