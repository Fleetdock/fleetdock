"use client";

import { useErrorToast } from "@/components/toast";

import { useCallback, useMemo, useRef, useState, type FormEvent } from "react";

import { ExplainView } from "@/components/database/explain-view";
import { PreviewSampleView } from "@/components/database/preview-sample";
import { ExportButton } from "@/components/database/export-button";
import { SqlEditor, type SqlEditorHandle } from "@/components/sql-editor";
import { ResultGrid } from "@/components/data-browser/result-grid";
import { dialectOf, type Dialect } from "@/lib/data-browser/cells";
import { load, save } from "@/lib/data-browser/storage";
import { randomUUID } from "@/lib/uuid";
import {
  Button,
  Callout,
  Checkbox,
  ConfirmModal,
  ErrorText,
  Field,
  HelpText,
  IconButton,
  Input,
  Modal,
  Select,
  Skeleton,
  Table,
} from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  cancelQuery,
  exportQueryCSV,
  useQueryHistory,
  useRunQuery,
  useSavedQueries,
  useSavedQueryMutations,
  useExplain,
  useSchemaColumns,
} from "@/lib/hooks";
import { previewBlocker, splitStatements, writeStatements } from "@/lib/sql";
import { sqlCompletion } from "@/lib/sql-schema";
import type { ExplainResult, QueryOutput, QueryResult } from "@/lib/types";
import {
  Bookmark,
  FlaskConical,
  Gauge,
  History,
  ListTree,
  KeyRound,
  Play,
  Save,
  Square,
  Trash2,
} from "lucide-react";

/** Rows a statement may return, as offered in the console (the API caps it at 5000). */
const ROW_LIMITS = [200, 1000, 5000] as const;
const DEFAULT_LIMIT = 200;
/** Editor share of the console's height, in percent. */
const SPLIT_MIN = 15;
const SPLIT_MAX = 85;

type Panel = "none" | "history" | "saved";

export function QueryConsole({
  databaseId,
  canWrite,
  initialSql = "",
  engine,
}: {
  databaseId: string;
  canWrite: boolean;
  /** Engine of the database ("postgres", "mysql", "mariadb"), for autocompletion. */
  engine?: string;
  /** Text the editor starts with (a Data Browser "New query on this table"). */
  initialSql?: string;
}) {
  const run = useRunQuery(databaseId);
  const dialect = dialectOf(engine);
  const [limit, setLimit] = useState<number>(() => {
    const v = Number(load("console-limit"));
    return (ROW_LIMITS as readonly number[]).includes(v) ? v : DEFAULT_LIMIT;
  });
  // The editor/results split, dragged or moved with the arrow keys.
  const [split, setSplit] = useState<number>(() => {
    const v = Number(load("console-split"));
    return v >= SPLIT_MIN && v <= SPLIT_MAX ? v : 38;
  });
  const consoleRef = useRef<HTMLDivElement>(null);
  const setSplitSaved = (v: number) => {
    const next = Math.round(Math.max(SPLIT_MIN, Math.min(SPLIT_MAX, v)));
    setSplit(next);
    save("console-split", String(next));
  };
  const startSplit = (e: React.PointerEvent) => {
    const box = consoleRef.current?.getBoundingClientRect();
    if (!box) return;
    e.preventDefault();
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    const onMove = (ev: PointerEvent) => setSplit(Math.max(SPLIT_MIN, Math.min(SPLIT_MAX, ((ev.clientY - box.top) / box.height) * 100)));
    const onUp = (ev: PointerEvent) => {
      target.removeEventListener("pointermove", onMove);
      target.removeEventListener("pointerup", onUp);
      setSplitSaved(((ev.clientY - box.top) / box.height) * 100);
    };
    target.addEventListener("pointermove", onMove);
    target.addEventListener("pointerup", onUp);
  };
  const columns = useSchemaColumns(databaseId);
  const completion = useMemo(() => sqlCompletion(columns.data, engine === "postgres"), [columns.data, engine]);
  const editorRef = useRef<SqlEditorHandle>(null);
  // The editor's text is kept per database in this browser, so a reload or
  // an expired session (which goes through the sign-in page) does not lose
  // a script being written. A "New query on this table" text wins.
  const draftKey = `console-draft:${databaseId}`;
  const [sql, setSqlState] = useState(() => initialSql || load(draftKey) || "");
  const setSql = useCallback(
    (v: string) => {
      setSqlState(v);
      save(draftKey, v.trim() ? v.slice(0, 200_000) : null);
    },
    [draftKey],
  );
  const [ranSql, setRanSql] = useState("");
  const [output, setOutput] = useState<QueryOutput | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<{
    sql: string;
    writes: string[];
    /** The preview of this script, when it was previewed first. */
    preview?: QueryOutput;
  } | null>(null);
  const [queryId, setQueryId] = useState<string | null>(null);
  const [panel, setPanel] = useState<Panel>("none");
  const [saving, setSaving] = useState(false);
  const explain = useExplain(databaseId);
  const [plan, setPlan] = useState<ExplainResult | null>(null);

  // Explain works on one statement: the selection, or the whole editor.
  const explainTarget = (): string | null => {
    const text = editorRef.current?.getQueryToRun() ?? sql.trim();
    const stmts = splitStatements(text);
    if (stmts.length !== 1) {
      setError(stmts.length === 0 ? null : "Explain works on one statement — select the one to explain.");
      return null;
    }
    return stmts[0];
  };

  async function onExplain(analyze: boolean) {
    const stmt = explainTarget();
    if (!stmt) return;
    if (analyze && writeStatements(stmt).length > 0) {
      setError("Only reads can be analyzed: analyzing runs the statement. Use Explain for writes.");
      return;
    }
    setError(null);
    try {
      const res = await explain.mutateAsync({ sql: stmt, analyze });
      setOutput(null);
      setPlan(res);
    } catch (err) {
      setPlan(null);
      setError(err instanceof ApiError ? err.message : "Explain failed");
    }
  }

  function onRun() {
    const text = editorRef.current?.getQueryToRun() ?? sql.trim();
    if (!text) return;
    const writes = writeStatements(text);
    if (writes.length > 0) {
      if (!canWrite) {
        setError(
          "This script contains write statements, and you only have read access to this database.",
        );
        return;
      }
      setConfirm({ sql: text, writes });
      return;
    }
    void execute(text);
  }

  async function execute(text: string, preview = false) {
    setError(null);
    setConfirm(null);
    const id = randomUUID();
    setQueryId(id);
    try {
      const res = await run.mutateAsync({
        sql: text,
        limit,
        query_id: id,
        preview,
      });
      setOutput(res);
      setPlan(null);
      setRanSql(text);
    } catch (err) {
      setOutput(null);
      setError(err instanceof ApiError ? err.message : "Query failed");
    } finally {
      setQueryId(null);
    }
  }

  async function onPreview() {
    const text = editorRef.current?.getQueryToRun() ?? sql.trim();
    if (!text) return;
    const blocked = previewBlocker(text);
    if (blocked) {
      setError(blocked);
      return;
    }
    await execute(text, true);
  }

  async function onCancel() {
    if (!queryId) return;
    try {
      await cancelQuery(databaseId, queryId);
    } catch {
      /* already finished */
    }
  }

  // Export reruns a single read; only offer it for one-statement reads.
  const single =
    output &&
    output.results.length === 1 &&
    splitStatements(ranSql).length === 1;
  const canExport = Boolean(
    single &&
    output.results[0].columns.length > 0 &&
    writeStatements(ranSql).length === 0,
  );

  return (
    <div className="dbx-console" ref={consoleRef}>
      <div className="dbx-console-top" style={{ flexBasis: `${split}%` }}>
        <div className="dbx-console-editor">
          <SqlEditor
            ref={editorRef}
            value={sql}
            onChange={setSql}
            onRun={onRun}
            placeholder="SELECT * FROM ..."
            minHeight="100%"
            engine={engine}
            completion={completion}
          />
        </div>
        <div className="dbx-console-bar">
          <span className="flex items-center gap-2 text-sm muted">
            <span className="hide-sm">
              <span className="kbd">⌘/Ctrl</span> +{" "}
              <span className="kbd">Enter</span> to run
              {sql.trim() ? " · selection runs when highlighted" : ""}
            </span>
            <HelpText label="About the SQL console">
              {canWrite
                ? 'Several statements separated by ";" run in order on one session. You are asked to confirm before anything writes; Preview runs a script and rolls it back.'
                : "Run read-only SQL against this database. You do not have write access."}
            </HelpText>
            <Button
              size="sm"
              variant={panel === "history" ? "primary" : "ghost"}
              aria-pressed={panel === "history"}
              icon={<History size={14} aria-hidden />}
              onClick={() => setPanel(panel === "history" ? "none" : "history")}
            >
              History
            </Button>
            <Button
              size="sm"
              variant={panel === "saved" ? "primary" : "ghost"}
              aria-pressed={panel === "saved"}
              icon={<Bookmark size={14} aria-hidden />}
              onClick={() => setPanel(panel === "saved" ? "none" : "saved")}
            >
              Saved
            </Button>
          </span>
          <div className="flex items-center gap-2">
            {sql.trim() ? (
              <>
                <Button
                  size="sm"
                  icon={<Save size={14} aria-hidden />}
                  onClick={() => setSaving(true)}
                  disabled={run.isPending}
                >
                  Save
                </Button>
                <Button size="sm" onClick={() => setSql("")} disabled={run.isPending}>
                  Clear
                </Button>
              </>
            ) : null}
            <Select
              size="sm"
              className="w-auto"
              aria-label="Rows per statement"
              title="Rows a statement returns at most"
              value={limit}
              onChange={(e) => {
                const v = Number(e.target.value);
                setLimit(v);
                save("console-limit", v === DEFAULT_LIMIT ? null : String(v));
              }}
            >
              {ROW_LIMITS.map((n) => (
                <option key={n} value={n}>
                  {n.toLocaleString()} rows
                </option>
              ))}
            </Select>
            {canExport ? (
              <ExportButton
                label="Export CSV"
                run={() => exportQueryCSV(databaseId, ranSql)}
              />
            ) : null}
            {run.isPending ? (
              <Button
                size="sm"
                variant="danger"
                icon={<Square size={14} aria-hidden />}
                onClick={() => void onCancel()}
                disabled={!queryId}
              >
                Cancel
              </Button>
            ) : null}
            {canWrite && writeStatements(sql).length > 0 ? (
              <Button
                size="sm"
                icon={<FlaskConical size={14} aria-hidden />}
                onClick={() => void onPreview()}
                disabled={run.isPending || explain.isPending}
                title="Run the script in a transaction that is rolled back, and see what it would change"
              >
                Preview
              </Button>
            ) : null}
            <Button
              size="sm"
              icon={<ListTree size={14} aria-hidden />}
              loading={explain.isPending}
              onClick={() => void onExplain(false)}
              disabled={run.isPending || !sql.trim()}
              title="Show how the database would run this statement, without running it"
            >
              Explain
            </Button>
            <Button
              size="sm"
              icon={<Gauge size={14} aria-hidden />}
              onClick={() => void onExplain(true)}
              disabled={run.isPending || explain.isPending || !sql.trim()}
              title="Run this read and show where the time went (rolled back)"
            >
              Analyze
            </Button>
            <Button
              size="sm"
              variant="primary"
              icon={<Play size={15} aria-hidden />}
              loading={run.isPending}
              onClick={onRun}
              disabled={!sql.trim()}
            >
              Run
            </Button>
          </div>
        </div>

      </div>
      <div
        className="dbx-console-split"
        role="separator"
        aria-orientation="horizontal"
        aria-label="Resize editor and results"
        aria-valuemin={SPLIT_MIN}
        aria-valuemax={SPLIT_MAX}
        aria-valuenow={split}
        tabIndex={0}
        onPointerDown={startSplit}
        onKeyDown={(e) => {
          if (e.key === "ArrowUp" || e.key === "ArrowDown") {
            e.preventDefault();
            setSplitSaved(split + (e.key === "ArrowDown" ? 5 : -5));
          }
        }}
      />
      <div className={`dbx-console-results${output && output.results.filter((r) => r.columns.length > 0).length === 1 ? " is-single" : ""}`}>
        {!output && !plan && !error && panel === "none" ? (
          <p className="dbx-console-empty muted text-sm">Results show here. Run a statement with Ctrl+Enter.</p>
        ) : null}
        {panel === "history" ? (
          <HistoryPanel databaseId={databaseId} onPick={(s) => setSql(s)} />
        ) : null}
        {panel === "saved" ? (
          <SavedPanel databaseId={databaseId} onPick={(s) => setSql(s)} />
        ) : null}

        <ErrorText message={error ?? undefined} />

        {plan ? <ExplainView result={plan} /> : null}

        {output ? (
          <div>
            {output.rolled_back ? (
              <Callout
                tone="info"
                role="status"
                className="mt-4"
                title="Preview — rolled back, nothing was saved."
                actions={
                  canWrite && ranSql ? (
                    <Button
                      size="sm"
                      variant="primary"
                      icon={<Play size={14} aria-hidden />}
                      onClick={() => setConfirm({ sql: ranSql, writes: writeStatements(ranSql), preview: output })}
                    >
                      Apply for real
                    </Button>
                  ) : null
                }
              >
                {previewSummary(output)}
                <ul className="preview-notes">
                  {(output.results.find((r) => r.notes)?.notes ?? []).slice(1).map((n) => (
                    <li key={n}>{n}</li>
                  ))}
                </ul>
              </Callout>
            ) : null}
            {output.results.map((r, i) => (
              <QueryResultView
                key={i}
                dialect={dialect}
                preview={output.rolled_back}
                result={r}
                label={
                  output.results.length > 1 || output.error
                    ? `Statement ${i + 1}`
                    : undefined
                }
              />
            ))}
            {output.error ? (
              <Callout
                tone="danger"
                className="mt-3.5"
                title={`Statement ${output.error.statement} failed: ${output.error.message}`}
              >
                <span className="text-sm muted">
                  The statements before it ran; the ones after it did not.
                </span>
              </Callout>
            ) : null}
          </div>
        ) : null}

      </div>

      <ConfirmModal
        open={confirm !== null}
        danger
        title="Run statements that change data?"
        confirmLabel={
          confirm && confirm.writes.length > 1
            ? `Run ${confirm.writes.length} write statements`
            : "Run write statement"
        }
        message={
          <>
            <p className="mt-0">
              This script will modify the database. There is no undo.
            </p>
            {confirm?.preview ? (
              <Callout tone="warning">
                <strong>Preview:</strong> {previewSummary(confirm.preview)} Data may have changed since, so the
                real run can differ.
              </Callout>
            ) : null}
            <pre className="card p-2.5 max-h-50 overflow-auto text-xs whitespace-pre-wrap">
              <code>
                {confirm?.writes
                  .map((w) => (w.length > 400 ? w.slice(0, 400) + "…" : w))
                  .join(";\n\n")}
              </code>
            </pre>
          </>
        }
        onConfirm={() => confirm && void execute(confirm.sql)}
        onCancel={() => setConfirm(null)}
      />
      {saving ? (
        <SaveQueryModal
          databaseId={databaseId}
          sql={sql}
          onClose={() => setSaving(false)}
        />
      ) : null}
    </div>
  );
}

function HistoryPanel({
  databaseId,
  onPick,
}: {
  databaseId: string;
  onPick: (sql: string) => void;
}) {
  const { data, isLoading, error } = useQueryHistory(databaseId, true);
  if (isLoading) return <PanelSkeleton label="Loading history" />;
  if (error)
    return (
      <ErrorText
        message={
          error instanceof ApiError ? error.message : "Could not load history"
        }
      />
    );
  if (!data?.length) return <p className="text-sm muted">No history yet. Queries you run show up here.</p>;
  return (
    <Table wrapClassName="mt-2.5 max-h-75 overflow-y-auto" className="text-[length:var(--fs-sm)]">
      <tbody>
        {data.map((h) => (
          <tr
            key={h.id}
            className="cursor-pointer"
            onClick={() => onPick(h.sql)}
            title="Load into the editor"
          >
            <td className="muted w-[150px]">
              {new Date(h.created_at).toLocaleString()}
            </td>
            <td>
              <code className="text-sm whitespace-pre-wrap break-words">
                {h.sql.length > 240 ? h.sql.slice(0, 240) + "…" : h.sql}
              </code>
            </td>
            <td className={h.error ? "w-40 text-right" : "w-40 text-right muted num"}>
              {h.error ? (
                <span className="text-danger" title={h.error}>
                  failed
                </span>
              ) : (
                `${h.duration_ms} ms`
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

function SavedPanel({
  databaseId,
  onPick,
}: {
  databaseId: string;
  onPick: (sql: string) => void;
}) {
  const { data, isLoading, error } = useSavedQueries(databaseId);
  const { remove } = useSavedQueryMutations(databaseId);
  const toastError = useErrorToast();
  if (isLoading) return <PanelSkeleton label="Loading saved queries" />;
  if (error)
    return (
      <ErrorText
        message={
          error instanceof ApiError
            ? error.message
            : "Could not load saved queries"
        }
      />
    );
  if (!data?.length)
    return (
      <p className="text-sm muted">
        No saved queries. Write one and press Save.
      </p>
    );
  return (
    <Table wrapClassName="mt-2.5 max-h-75 overflow-y-auto" className="text-[length:var(--fs-sm)]">
        <tbody>
          {data.map((q) => (
            <tr key={q.id}>
              <td
                className="cursor-pointer"
                onClick={() => onPick(q.sql)}
                title="Load into the editor"
              >
                <span className="font-medium">{q.name}</span>
                {q.database_id ? null : (
                  <span className="muted"> · all databases</span>
                )}
                <div>
                  <code className="text-sm muted">
                    {q.sql.length > 160 ? q.sql.slice(0, 160) + "…" : q.sql}
                  </code>
                </div>
              </td>
              <td className="w-[50px] text-right">
                <IconButton
                  label={`Delete ${q.name}`}
                  onClick={() => remove.mutate(q.id, { onError: toastError("Failed to delete the saved query") })}
                >
                  <Trash2 size={14} aria-hidden />
                </IconButton>
              </td>
            </tr>
          ))}
        </tbody>
    </Table>
  );
}

function SaveQueryModal({
  databaseId,
  sql,
  onClose,
}: {
  databaseId: string;
  sql: string;
  onClose: () => void;
}) {
  const { create } = useSavedQueryMutations(databaseId);
  const [name, setName] = useState("");
  const [allDatabases, setAllDatabases] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({
        name,
        sql,
        database_id: allDatabases ? undefined : databaseId,
      });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save");
    }
  }

  return (
    <Modal open onClose={onClose} title="Save query">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
          />
        </Field>
        <Checkbox
          label="Show in every database's console"
          checked={allDatabases}
          onChange={(e) => setAllDatabases(e.target.checked)}
        />
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={create.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function QueryResultView({
  result,
  label,
  preview = false,
  dialect,
}: {
  result: QueryResult;
  label?: string;
  /** Part of a rolled-back preview. */
  preview?: boolean;
  dialect: Dialect;
}) {
  const header = label ? <span className="font-medium">{label} · </span> : null;
  if (result.columns.length === 0) {
    return (
      <div className="card px-3.5 py-3 dbx-console-affected">
        <span className="flex items-center gap-2 text-sm">
          <KeyRound size={15} aria-hidden /> {header}
          {result.rows_affected.toLocaleString()} row
          {result.rows_affected === 1 ? "" : "s"} {preview ? "would be affected" : "affected"}
          <span className="muted">· {result.duration_ms} ms</span>
        </span>
        {result.sample ? <PreviewSampleView sample={result.sample} /> : null}
      </div>
    );
  }
  return (
    <section className="dbx-result" aria-label={label ?? "Result"}>
      <div className="dbx-result-head text-sm muted">
        {header}
        {result.row_count.toLocaleString()} row
        {result.row_count === 1 ? "" : "s"} · {result.duration_ms} ms
        {result.truncated ? (
          <span className="text-warning"> · stopped at {result.row_count.toLocaleString()}; raise the row limit or add LIMIT</span>
        ) : null}
      </div>
      <div className="dbx-result-grid">
        <ResultGrid result={result} dialect={dialect} />
      </div>
    </section>
  );
}

/** PanelSkeleton stands in for the history / saved list while it loads. */
function PanelSkeleton({ label }: { label: string }) {
  return (
    <div role="status" aria-label={label} className="card p-3 mt-2.5 stack-sm">
      {[0, 1, 2].map((i) => (
        <Skeleton key={i} width={`${85 - i * 15}%`} />
      ))}
    </div>
  );
}

/** previewSummary: "would change 2 rows (statement 1), delete 1 row (statement 3)." */
function previewSummary(out: QueryOutput): string {
  const parts = out.results
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => !r.read_only)
    .map(({ r, i }) => `${r.rows_affected.toLocaleString()} row${r.rows_affected === 1 ? "" : "s"} (statement ${i + 1})`);
  if (out.error) return `Statement ${out.error.statement} would fail: ${out.error.message}`;
  return parts.length ? `It would change ${parts.join(", ")}.` : "It would change no rows.";
}
