"use client";

import { useErrorToast } from "@/components/toast";

import { useRef, useState, type FormEvent } from "react";

import { ExportButton } from "@/components/database/export-button";
import { SqlEditor, type SqlEditorHandle } from "@/components/sql-editor";
import {
  ConfirmModal,
  ErrorText,
  Field,
  Modal,
  Spinner,
} from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  cancelQuery,
  exportQueryCSV,
  useQueryHistory,
  useRunQuery,
  useSavedQueries,
  useSavedQueryMutations,
} from "@/lib/hooks";
import { splitStatements, writeStatements } from "@/lib/sql";
import type { QueryOutput, QueryResult } from "@/lib/types";
import {
  Bookmark,
  History,
  KeyRound,
  Play,
  Save,
  Square,
  Trash2,
} from "lucide-react";

const ROW_LIMIT = 200;

type Panel = "none" | "history" | "saved";

export function QueryConsole({
  databaseId,
  canWrite,
  initialSql = "",
}: {
  databaseId: string;
  canWrite: boolean;
  /** Text the editor starts with (a Data Browser "New query on this table"). */
  initialSql?: string;
}) {
  const run = useRunQuery(databaseId);
  const editorRef = useRef<SqlEditorHandle>(null);
  const [sql, setSql] = useState(initialSql);
  const [ranSql, setRanSql] = useState("");
  const [output, setOutput] = useState<QueryOutput | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<{
    sql: string;
    writes: string[];
  } | null>(null);
  const [queryId, setQueryId] = useState<string | null>(null);
  const [panel, setPanel] = useState<Panel>("none");
  const [saving, setSaving] = useState(false);

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

  async function execute(text: string) {
    setError(null);
    setConfirm(null);
    const id = crypto.randomUUID();
    setQueryId(id);
    try {
      const res = await run.mutateAsync({
        sql: text,
        limit: ROW_LIMIT,
        query_id: id,
      });
      setOutput(res);
      setRanSql(text);
    } catch (err) {
      setOutput(null);
      setError(err instanceof ApiError ? err.message : "Query failed");
    } finally {
      setQueryId(null);
    }
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
    <div>
      <p className="text-sm muted" style={{ marginBottom: ".5rem" }}>
        {canWrite
          ? `Run SQL against this database — several statements separated by ";" run in order on one session. Reads return up to ${ROW_LIMIT} rows; you are asked to confirm before anything writes.`
          : `Run read-only SQL against this database (up to ${ROW_LIMIT} rows per statement). You do not have write access.`}
      </p>
      <SqlEditor
        ref={editorRef}
        value={sql}
        onChange={setSql}
        onRun={onRun}
        placeholder="SELECT * FROM ..."
        minHeight="220px"
      />
      <div
        className="flex justify-between items-center"
        style={{ marginTop: ".5rem", flexWrap: "wrap", gap: ".5rem" }}
      >
        <span className="flex items-center gap-2 text-sm muted">
          <span>
            <span className="kbd">⌘/Ctrl</span> +{" "}
            <span className="kbd">Enter</span> to run
            {sql.trim() ? " · selection runs when highlighted" : ""}
          </span>
          <button
            className={`btn btn-ghost btn-sm${panel === "history" ? " btn-primary" : ""}`}
            onClick={() => setPanel(panel === "history" ? "none" : "history")}
          >
            <History size={14} /> History
          </button>
          <button
            className={`btn btn-ghost btn-sm${panel === "saved" ? " btn-primary" : ""}`}
            onClick={() => setPanel(panel === "saved" ? "none" : "saved")}
          >
            <Bookmark size={14} /> Saved
          </button>
        </span>
        <div className="flex items-center gap-2">
          {sql.trim() ? (
            <>
              <button
                className="btn btn-sm"
                onClick={() => setSaving(true)}
                disabled={run.isPending}
              >
                <Save size={14} /> Save
              </button>
              <button
                className="btn btn-sm"
                onClick={() => setSql("")}
                disabled={run.isPending}
              >
                Clear
              </button>
            </>
          ) : null}
          {canExport ? (
            <ExportButton
              label="Export CSV"
              run={() => exportQueryCSV(databaseId, ranSql)}
            />
          ) : null}
          {run.isPending ? (
            <button
              className="btn btn-danger btn-sm"
              onClick={() => void onCancel()}
              disabled={!queryId}
            >
              <Square size={14} /> Cancel
            </button>
          ) : null}
          <button
            className="btn btn-primary btn-sm"
            onClick={onRun}
            disabled={run.isPending || !sql.trim()}
          >
            {run.isPending ? <Spinner /> : <Play size={15} />} Run
          </button>
        </div>
      </div>

      {panel === "history" ? (
        <HistoryPanel databaseId={databaseId} onPick={(s) => setSql(s)} />
      ) : null}
      {panel === "saved" ? (
        <SavedPanel databaseId={databaseId} onPick={(s) => setSql(s)} />
      ) : null}

      <ErrorText message={error ?? undefined} />

      {output ? (
        <div>
          {output.results.map((r, i) => (
            <QueryResultView
              key={i}
              result={r}
              label={
                output.results.length > 1 || output.error
                  ? `Statement ${i + 1}`
                  : undefined
              }
            />
          ))}
          {output.error ? (
            <div
              className="card"
              style={{
                padding: ".8rem .9rem",
                marginTop: ".9rem",
                borderColor: "var(--danger)",
              }}
            >
              <span className="text-sm" style={{ color: "var(--danger)" }}>
                Statement {output.error.statement} failed:{" "}
                {output.error.message}
              </span>
              <p className="text-sm muted" style={{ margin: ".3rem 0 0" }}>
                The statements before it ran; the ones after it did not.
              </p>
            </div>
          ) : null}
        </div>
      ) : null}

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
            <p style={{ marginTop: 0 }}>
              This script will modify the database. There is no undo.
            </p>
            <pre
              className="card"
              style={{
                padding: ".6rem",
                maxHeight: 200,
                overflow: "auto",
                fontSize: 12,
                whiteSpace: "pre-wrap",
              }}
            >
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
  if (isLoading) return <Spinner />;
  if (error)
    return (
      <ErrorText
        message={
          error instanceof ApiError ? error.message : "Could not load history"
        }
      />
    );
  if (!data?.length) return <p className="text-sm muted">No history yet.</p>;
  return (
    <div
      className="card"
      style={{ marginTop: ".6rem", maxHeight: 300, overflowY: "auto" }}
    >
      <table className="table" style={{ fontSize: 13 }}>
        <tbody>
          {data.map((h) => (
            <tr
              key={h.id}
              style={{ cursor: "pointer" }}
              onClick={() => onPick(h.sql)}
              title="Load into the editor"
            >
              <td style={{ width: 150 }} className="muted">
                {new Date(h.created_at).toLocaleString()}
              </td>
              <td>
                <code
                  className="text-sm"
                  style={{ whiteSpace: "pre-wrap", wordBreak: "break-word" }}
                >
                  {h.sql.length > 240 ? h.sql.slice(0, 240) + "…" : h.sql}
                </code>
              </td>
              <td
                style={{ width: 160, textAlign: "right" }}
                className={h.error ? undefined : "muted"}
              >
                {h.error ? (
                  <span style={{ color: "var(--danger)" }} title={h.error}>
                    failed
                  </span>
                ) : (
                  `${h.duration_ms} ms`
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
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
  if (isLoading) return <Spinner />;
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
    <div
      className="card"
      style={{ marginTop: ".6rem", maxHeight: 300, overflowY: "auto" }}
    >
      <table className="table" style={{ fontSize: 13 }}>
        <tbody>
          {data.map((q) => (
            <tr key={q.id}>
              <td
                style={{ cursor: "pointer" }}
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
              <td style={{ width: 50, textAlign: "right" }}>
                <button
                  className="btn btn-ghost btn-sm"
                  aria-label={`Delete ${q.name}`}
                  onClick={() => remove.mutate(q.id, { onError: toastError("Failed to delete the saved query") })}
                >
                  <Trash2 size={14} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
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
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
          />
        </Field>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={allDatabases}
            onChange={(e) => setAllDatabases(e.target.checked)}
          />
          Show in every database&apos;s console
        </label>
        <ErrorText message={error ?? undefined} />
        <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
          <button
            className="btn btn-primary"
            type="submit"
            disabled={create.isPending}
          >
            Save
          </button>
          <button className="btn" type="button" onClick={onClose}>
            Cancel
          </button>
        </div>
      </form>
    </Modal>
  );
}

function QueryResultView({
  result,
  label,
}: {
  result: QueryResult;
  label?: string;
}) {
  const header = label ? <span className="font-medium">{label} · </span> : null;
  if (result.columns.length === 0) {
    return (
      <div
        className="card"
        style={{ padding: ".8rem .9rem", marginTop: ".9rem" }}
      >
        <span className="flex items-center gap-2 text-sm">
          <KeyRound size={15} /> {header}
          {result.rows_affected.toLocaleString()} row
          {result.rows_affected === 1 ? "" : "s"} affected
          <span className="muted">· {result.duration_ms} ms</span>
        </span>
      </div>
    );
  }
  return (
    <div style={{ marginTop: ".9rem" }}>
      <div
        className="flex justify-between items-center"
        style={{ marginBottom: ".5rem", flexWrap: "wrap", gap: ".4rem" }}
      >
        <span className="text-sm muted">
          {header}
          {result.row_count.toLocaleString()} row
          {result.row_count === 1 ? "" : "s"} · {result.duration_ms} ms
          {result.truncated
            ? ` · truncated to the first ${result.row_count}`
            : ""}
        </span>
      </div>
      <div className="card" style={{ overflowX: "auto" }}>
        <table className="table" style={{ fontSize: 13, whiteSpace: "nowrap" }}>
          <thead>
            <tr>
              {result.columns.map((c, i) => (
                <th key={`${c}-${i}`}>{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {result.rows.length === 0 ? (
              <tr>
                <td colSpan={result.columns.length} className="muted">
                  No rows returned.
                </td>
              </tr>
            ) : (
              result.rows.map((row, ri) => (
                <tr key={ri}>
                  {row.map((cell, ci) => (
                    <td
                      key={ci}
                      className={cell === null ? "muted" : undefined}
                      style={{
                        maxWidth: 320,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                      }}
                      title={cell ?? undefined}
                    >
                      {cell === null ? "NULL" : cell}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
