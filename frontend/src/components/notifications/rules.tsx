"use client";

import { Bell, Pencil, Plus, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState, type FormEvent } from "react";

import { FormError } from "@/components/access/form-error";
import { useConfirm } from "@/components/confirm";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { useErrorToast } from "@/components/toast";
import { Badge, Button, Checkbox, Field, IconButton, Input, Modal, SectionHeader, Select, StatusBadge, Switch, type Tone } from "@/components/ui";
import { fieldError, friendlyError } from "@/lib/errors";
import { useAlertRules, useChannels, useCreateAlertRule, useDeleteAlertRule, useServers, useUpdateAlertRule } from "@/lib/hooks";
import type { AlertRule } from "@/lib/types";

import { CHANNEL_LABEL, COMPARATOR_LABEL, METRIC_LABEL } from "./labels";

const SEVERITY_TONE: Record<string, Tone> = {
  critical: "danger",
  warning: "warning",
  info: "info",
};

/** RulesSection lists the metric thresholds that raise an alert. */
export function RulesSection({ canWrite }: { canWrite: boolean }) {
  const { data, isLoading, error, refetch } = useAlertRules();
  const { data: channels } = useChannels();
  const del = useDeleteAlertRule();
  const toastError = useErrorToast();
  const confirm = useConfirm();
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
        render: (r) =>
          `${METRIC_LABEL[r.metric] ?? r.metric} ${COMPARATOR_LABEL[r.comparator] ?? r.comparator} ${r.threshold} for ${r.for_seconds}s`,
      },
      {
        id: "severity",
        header: "Severity",
        render: (r) => (
          <Badge tone={SEVERITY_TONE[r.severity] ?? "neutral"} dot>
            {r.severity}
          </Badge>
        ),
      },
      {
        id: "channels",
        header: "Channels",
        className: "muted",
        hideOnMobile: true,
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
          <div className="flex items-center justify-end gap-1">
            <IconButton label={`Edit ${r.name}`} onClick={() => setEditTarget(r)}>
              <Pencil size={15} aria-hidden />
            </IconButton>
            <IconButton
              label={`Delete ${r.name}`}
              variant="danger"
              onClick={async () => {
                const ok = await confirm({
                  title: `Delete "${r.name}"?`,
                  message: "You will no longer be alerted for this event.",
                  confirmLabel: "Delete rule",
                  danger: true,
                });
                if (ok) del.mutate(r.id, { onError: toastError("Failed to delete the rule") });
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
  }, [toastError, canWrite, channelName, confirm, del]);

  const addButton = canWrite ? (
    <Button variant="primary" size="sm" icon={<Plus size={15} aria-hidden />} onClick={() => setAddOpen(true)}>
      Add rule
    </Button>
  ) : null;

  return (
    <section className="section" aria-labelledby="rules-title">
      <SectionHeader
        icon={<Bell size={16} aria-hidden />}
        title={<span id="rules-title">Alert rules</span>}
        description="When a server metric crosses a threshold, these send an alert."
        actions={addButton}
      />
      <DataTable<AlertRule>
        columns={columns}
        rows={data?.items ?? []}
        rowKey={(r) => r.id}
        isLoading={isLoading}
        error={error ? friendlyError(error) : undefined}
        errorTitle="Could not load alert rules"
        onRetry={() => void refetch()}
        emptyTitle="No alert rules yet"
        emptyHint="Create a rule to be notified when a server metric crosses a threshold."
        emptyAction={addButton}
      />
      <RuleModal mode="create" open={addOpen} onClose={() => setAddOpen(false)} />
      <RuleModal mode="edit" rule={editTarget} open={editTarget !== null} onClose={() => setEditTarget(null)} />
    </section>
  );
}

type RuleModalProps =
  | { mode: "create"; rule?: undefined; open: boolean; onClose: () => void }
  | { mode: "edit"; rule: AlertRule | null; open: boolean; onClose: () => void };

export function RuleModal({ mode, rule, open, onClose }: RuleModalProps) {
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
  const [error, setError] = useState<unknown>(null);

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
      setError(err);
    }
  }

  const fe = (field: string) => fieldError(error, field);

  return (
    <Modal open={open} onClose={onClose} title={isEdit ? "Edit alert rule" : "New alert rule"} wide>
      <form onSubmit={onSubmit}>
        <Field label="Name" error={fe("name")}>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="High CPU" required />
        </Field>
        <div className="form-grid">
          <Field label="Scope" error={fe("target_type")}>
            <Select value={targetType} onChange={(e) => setTargetType(e.target.value)}>
              <option value="global">Any server</option>
              <option value="server">Specific server</option>
            </Select>
          </Field>
          {targetType === "server" ? (
            <Field label="Server" error={fe("target_id")}>
              <Select value={targetId} onChange={(e) => setTargetId(e.target.value)} required>
                <option value="">Select…</option>
                {servers?.items.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </Select>
            </Field>
          ) : (
            <div />
          )}
        </div>
        <div className="grid gap-x-3 sm:grid-cols-[1.3fr_0.7fr_1fr]">
          <Field label="Metric" error={fe("metric")}>
            <Select value={metric} onChange={(e) => setMetric(e.target.value)}>
              {Object.entries(METRIC_LABEL).map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="When" error={fe("comparator")}>
            <Select value={comparator} onChange={(e) => setComparator(e.target.value)}>
              <option value="gt">&gt;</option>
              <option value="gte">≥</option>
              <option value="lt">&lt;</option>
              <option value="lte">≤</option>
            </Select>
          </Field>
          <Field label="Threshold" error={fe("threshold")}>
            <Input type="number" step="any" value={threshold} onChange={(e) => setThreshold(e.target.value)} required />
          </Field>
        </div>
        <div className="form-grid">
          <Field label="For (seconds)" hint="How long the condition must hold first." error={fe("for_seconds")}>
            <Input type="number" min={0} value={forSeconds} onChange={(e) => setForSeconds(e.target.value)} required />
          </Field>
          <Field label="Severity" error={fe("severity")}>
            <Select value={severity} onChange={(e) => setSeverity(e.target.value)}>
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="critical">Critical</option>
            </Select>
          </Field>
        </div>
        <fieldset className="field m-0 border-0 p-0">
          <legend className="label">Channels</legend>
          <div className="card px-3 py-2 max-h-32 overflow-y-auto grid gap-1.5">
            {channels?.items.length ? (
              channels.items.map((c) => (
                <Checkbox
                  key={c.id}
                  className="text-sm"
                  label={
                    <>
                      {c.name} <span className="muted">({CHANNEL_LABEL[c.type] ?? c.type})</span>
                    </>
                  }
                  checked={channelIds.includes(c.id)}
                  onChange={() => toggleChannel(c.id)}
                />
              ))
            ) : (
              <span className="text-sm muted">No channels yet — add one in Channels above.</span>
            )}
          </div>
          <p className="field-hint">Leave all unchecked to send to every enabled channel.</p>
          {fe("channel_ids") ? <p className="field-error">{fe("channel_ids")}</p> : null}
        </fieldset>
        <Switch label="Enabled" checked={enabled} onCheckedChange={setEnabled} />
        <div className="mt-3">
          <FormError
            error={error}
            fields={["name", "target_type", "target_id", "metric", "comparator", "threshold", "for_seconds", "severity", "channel_ids"]}
            fallback={`Failed to ${isEdit ? "update" : "add"} rule`}
          />
        </div>
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={pending}>
            {pending ? "Saving…" : isEdit ? "Save changes" : "Create rule"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
