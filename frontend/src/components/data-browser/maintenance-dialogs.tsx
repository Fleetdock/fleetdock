"use client";

import { useState, type FormEvent } from "react";

import { Wrench } from "lucide-react";

import { useConfirm } from "@/components/confirm";
import { useToast } from "@/components/toast";
import { Badge, Button, Checkbox, ErrorText, Field, Input, Modal, Spinner } from "@/components/ui";
import { KeyValueList } from "@/components/ui/detail";
import { ApiError } from "@/lib/api";
import { formatBytes, formatRelative } from "@/lib/format";
import { useMaintenanceMutations, usePartitions, useSequence, useTableStats } from "@/lib/hooks";
import type { MaintenanceOp, TableStat } from "@/lib/types";

// PostgreSQL housekeeping from the data browser's tree: sequence values,
// table statistics with VACUUM / ANALYZE / REINDEX, and partitions.

export type MaintenanceDialog =
  | { kind: "sequence"; schema: string; name: string }
  | { kind: "stats"; schema: string }
  | { kind: "partitions"; schema: string; table: string };

const errorText = (err: unknown, fallback: string) => (err instanceof ApiError ? err.message : fallback);

export const OP_LABEL: Record<MaintenanceOp, string> = {
  vacuum: "VACUUM",
  vacuum_analyze: "VACUUM ANALYZE",
  analyze: "ANALYZE",
  reindex: "REINDEX",
};

const OP_HINT: Record<MaintenanceOp, string> = {
  vacuum: "Frees the space of deleted and updated rows for reuse. Reads and writes go on meanwhile.",
  vacuum_analyze: "VACUUM, then fresh statistics for the query planner. Reads and writes go on meanwhile.",
  analyze: "Fresh statistics for the query planner. Reads and writes go on meanwhile.",
  reindex: "Rebuilds the indexes. Writes to the table wait until it is done, and so do reads that use an index.",
};

/**
 * useMaintenance runs a housekeeping command after asking, and reports the
 * outcome in a toast.
 */
export function useMaintenance(databaseId: string) {
  const { maintain } = useMaintenanceMutations(databaseId);
  const confirm = useConfirm();
  const { push } = useToast();
  return {
    pending: maintain.isPending,
    async run(op: MaintenanceOp, schema: string, table?: string) {
      const target = table ? `${schema}.${table}` : `every table in ${schema}`;
      const ok = await confirm({
        title: `${OP_LABEL[op]} ${target}?`,
        message: OP_HINT[op],
        confirmLabel: `Run ${OP_LABEL[op]}`,
        danger: op === "reindex",
      });
      if (!ok) return;
      try {
        const res = await maintain.mutateAsync({ op, schema, table });
        push("success", res.ran.length === 1 ? `Ran ${res.ran[0]}` : `Ran ${OP_LABEL[op]} on ${res.ran.length} tables`);
      } catch (err) {
        push("error", errorText(err, `${OP_LABEL[op]} failed`));
      }
    },
  };
}

export function MaintenanceDialogs({
  databaseId,
  dialog,
  canWrite,
  onClose,
}: {
  databaseId: string;
  dialog: MaintenanceDialog | null;
  canWrite: boolean;
  onClose: () => void;
}) {
  if (!dialog) return null;
  if (dialog.kind === "sequence") {
    return <SequenceDialog databaseId={databaseId} schema={dialog.schema} name={dialog.name} canWrite={canWrite} onClose={onClose} />;
  }
  if (dialog.kind === "stats") {
    return <StatsDialog databaseId={databaseId} schema={dialog.schema} canWrite={canWrite} onClose={onClose} />;
  }
  return <PartitionsDialog databaseId={databaseId} schema={dialog.schema} table={dialog.table} onClose={onClose} />;
}

function SequenceDialog({
  databaseId,
  schema,
  name,
  canWrite,
  onClose,
}: {
  databaseId: string;
  schema: string;
  name: string;
  canWrite: boolean;
  onClose: () => void;
}) {
  const { data, isLoading, error } = useSequence(databaseId, schema, name);
  const { setval } = useMaintenanceMutations(databaseId);
  const confirm = useConfirm();
  const { push } = useToast();
  const [next, setNext] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const value = Number(next);
  const valid = next.trim() !== "" && Number.isSafeInteger(value);
  const outOfRange = Boolean(data && valid && (value < data.min || value > data.max));

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!valid || !data) return;
    const ok = await confirm({
      title: `Set ${schema}.${name}?`,
      message:
        data.next_value !== null && value < data.next_value
          ? `The next value will be ${value.toLocaleString()}, below the ${data.next_value.toLocaleString()} it would be now: values may be handed out again, and new rows may collide with existing keys.`
          : `The next value will be ${value.toLocaleString()}.`,
      confirmLabel: "Set value",
      danger: data.next_value !== null && value < data.next_value,
    });
    if (!ok) return;
    setFormError(null);
    try {
      // called=false: the next nextval returns exactly this value.
      await setval.mutateAsync({ schema, name, value, called: false });
      push("success", `${name} continues at ${value.toLocaleString()}`);
      onClose();
    } catch (err) {
      setFormError(errorText(err, "Could not set the value."));
    }
  }

  return (
    <Modal open onClose={onClose} title={`Sequence ${schema}.${name}`}>
      {isLoading ? (
        <Spinner label="Reading the sequence…" />
      ) : error || !data ? (
        <ErrorText message={errorText(error, "Could not read the sequence.")} />
      ) : (
        <form onSubmit={submit}>
          <KeyValueList
            items={[
              {
                label: "Next value",
                value:
                  data.next_value === null ? <span className="muted">none: the sequence is used up</span> : data.next_value.toLocaleString(),
              },
              {
                label: "Last value handed out",
                value: data.last_value === null ? <span className="muted">none since it was set</span> : data.last_value.toLocaleString(),
              },
              { label: "Increment", value: data.increment.toLocaleString() },
              {
                label: "Range",
                value: (
                  <>
                    {data.min.toLocaleString()} … {data.max.toLocaleString()} {data.cycle ? <Badge>cycles</Badge> : null}
                  </>
                ),
              },
              { label: "Type", value: data.data_type },
              { label: "Belongs to", value: data.owned_by, mono: true, hidden: !data.owned_by },
            ]}
          />
          {canWrite ? (
            <div className="mt-4">
              <Field
                label="Continue at"
                hint="The value the next insert gets (setval with is_called false)."
                error={outOfRange ? `Outside the sequence's range ${data.min} … ${data.max}.` : undefined}
              >
                <Input
                  type="number"
                  inputMode="numeric"
                  value={next}
                  onChange={(e) => setNext(e.target.value)}
                  placeholder={data.next_value === null ? undefined : String(data.next_value)}
                />
              </Field>
              <ErrorText message={formError ?? undefined} />
            </div>
          ) : null}
          <div className="modal-actions">
            <Button type="button" onClick={onClose}>
              {canWrite ? "Cancel" : "Close"}
            </Button>
            {canWrite ? (
              <Button type="submit" variant="primary" loading={setval.isPending} disabled={!valid || outOfRange}>
                Set value
              </Button>
            ) : null}
          </div>
        </form>
      )}
    </Modal>
  );
}

/** when picks the later of a manual and an automatic run. */
function when(a: string | null, b: string | null): { text: string; full: string; auto: boolean } | null {
  const ta = a ? Date.parse(a) : 0;
  const tb = b ? Date.parse(b) : 0;
  if (!ta && !tb) return null;
  const auto = tb > ta;
  const t = auto ? tb : ta;
  return { text: formatRelative(t), full: `${new Date(t).toLocaleString()}${auto ? " by autovacuum" : ""}`, auto };
}

/** deadShare is dead rows as a share of all rows, 0 … 1. */
export const deadShare = (s: Pick<TableStat, "live_rows" | "dead_rows">) =>
  s.live_rows + s.dead_rows === 0 ? 0 : s.dead_rows / (s.live_rows + s.dead_rows);

function StatsDialog({
  databaseId,
  schema,
  canWrite,
  onClose,
}: {
  databaseId: string;
  schema: string;
  canWrite: boolean;
  onClose: () => void;
}) {
  const { data, isLoading, error, refetch, isFetching } = useTableStats(databaseId, schema);
  const m = useMaintenance(databaseId);
  const [onlyDead, setOnlyDead] = useState(false);
  const rows = (data?.items ?? []).filter((s) => !onlyDead || deadShare(s) >= 0.1);

  return (
    <Modal open onClose={onClose} title={`Tables in ${schema}`} wide>
      <div className="toolbar">
        <Checkbox label="Only tables with 10% or more dead rows" checked={onlyDead} onChange={(e) => setOnlyDead(e.target.checked)} />
        <div className="toolbar-spacer" />
        <Button size="sm" onClick={() => void refetch()} loading={isFetching}>
          Reload
        </Button>
        {canWrite ? (
          <>
            <Button size="sm" disabled={m.pending} onClick={() => void m.run("vacuum_analyze", schema)}>
              VACUUM ANALYZE all
            </Button>
            <Button size="sm" disabled={m.pending} onClick={() => void m.run("reindex", schema)}>
              REINDEX schema
            </Button>
          </>
        ) : null}
      </div>
      {isLoading ? (
        <Spinner label="Reading statistics…" />
      ) : error ? (
        <ErrorText message={errorText(error, "Could not read the statistics.")} />
      ) : rows.length === 0 ? (
        <p className="muted text-sm">{onlyDead ? "No table has that many dead rows." : `${schema} has no tables.`}</p>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Table</th>
                <th className="text-right">Size</th>
                <th className="text-right">Rows</th>
                <th className="text-right">Dead rows</th>
                <th>Last vacuum</th>
                <th>Last analyze</th>
                {canWrite ? <th className="text-right">Actions</th> : null}
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => {
                const v = when(s.last_vacuum, s.last_autovacuum);
                const a = when(s.last_analyze, s.last_autoanalyze);
                const share = deadShare(s);
                return (
                  <tr key={s.name}>
                    <td className="font-medium">{s.name}</td>
                    <td className="text-right num whitespace-nowrap">{formatBytes(s.total_bytes)}</td>
                    <td className="text-right num">{s.live_rows.toLocaleString()}</td>
                    <td className="text-right num" style={share >= 0.2 ? { color: "var(--warning)" } : undefined}>
                      {s.dead_rows.toLocaleString()}
                      {share >= 0.01 ? <span className="muted"> ({Math.round(share * 100)}%)</span> : null}
                    </td>
                    <td className="text-sm whitespace-nowrap" title={v?.full}>
                      {v ? `${v.text}${v.auto ? " (auto)" : ""}` : <span className="muted">never</span>}
                    </td>
                    <td className="text-sm whitespace-nowrap" title={a?.full}>
                      {a ? `${a.text}${a.auto ? " (auto)" : ""}` : <span className="muted">never</span>}
                    </td>
                    {canWrite ? (
                      <td className="text-right whitespace-nowrap">
                        <Button
                          size="sm"
                          icon={<Wrench size={13} aria-hidden />}
                          disabled={m.pending}
                          title={`VACUUM ANALYZE ${schema}.${s.name}`}
                          aria-label={`VACUUM ANALYZE ${schema}.${s.name}`}
                          onClick={() => void m.run("vacuum_analyze", schema, s.name)}
                        >
                          Vacuum
                        </Button>
                      </td>
                    ) : null}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <div className="modal-actions">
        <Button variant="primary" onClick={onClose}>
          Done
        </Button>
      </div>
    </Modal>
  );
}

function PartitionsDialog({
  databaseId,
  schema,
  table,
  onClose,
}: {
  databaseId: string;
  schema: string;
  table: string;
  onClose: () => void;
}) {
  const { data, isLoading, error } = usePartitions(databaseId, schema, table);
  return (
    <Modal open onClose={onClose} title={`Partitions of ${schema}.${table}`} wide>
      {isLoading ? (
        <Spinner label="Reading partitions…" />
      ) : error ? (
        <ErrorText message={errorText(error, "Could not read the partitions.")} />
      ) : (data?.items.length ?? 0) === 0 ? (
        <p className="muted text-sm">{table} has no partitions yet.</p>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Partition</th>
                <th>Bound</th>
                <th className="text-right">Rows</th>
                <th className="text-right">Size</th>
              </tr>
            </thead>
            <tbody>
              {data!.items.map((p) => (
                <tr key={`${p.schema}.${p.name}`}>
                  <td className="font-medium">
                    {p.schema !== schema ? <span className="muted">{p.schema}.</span> : null}
                    {p.name} {p.partitioned ? <Badge>partitioned</Badge> : null}
                  </td>
                  <td>
                    <code className="text-xs">{p.bound}</code>
                  </td>
                  <td className="text-right num">~{p.rows.toLocaleString()}</td>
                  <td className="text-right num">{formatBytes(p.total_bytes)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="modal-actions">
        <Button variant="primary" onClick={onClose}>
          Done
        </Button>
      </div>
    </Modal>
  );
}
