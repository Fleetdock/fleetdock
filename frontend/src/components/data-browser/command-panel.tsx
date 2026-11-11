"use client";

import { useMemo, useRef, useState } from "react";
import { Play, Square, X } from "lucide-react";

import { SqlEditor, type SqlEditorHandle } from "@/components/sql-editor";
import { useToast } from "@/components/toast";
import { Button, ConfirmModal, IconButton } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { load, save } from "@/lib/data-browser/storage";
import { cancelQuery, useRunQuery, useSchemaColumns } from "@/lib/hooks";
import { previewBlocker, splitStatements, writeStatements } from "@/lib/sql";
import { sqlCompletion } from "@/lib/sql-schema";
import type { QueryOutput, QueryResult } from "@/lib/types";
import { randomUUID } from "@/lib/uuid";

/** Rows a statement in the panel returns at most (the API caps it at 5000). */
const LIMIT = 1000;
/** Panel height, in pixels. */
const HEIGHT_MIN = 140;
const HEIGHT_DEFAULT = 280;

/** What a statement that would change data does, for the confirm dialog. */
type PendingWrite = {
  sql: string;
  writes: string[];
  /** Rows each write would change, from a rolled-back dry run; null when it can't be previewed. */
  affected: { statement: string; rows: number }[] | null;
  /** Why the dry run could not tell, when it couldn't. */
  note?: string;
};

/**
 * CommandPanel is a SQL editor under a table's grid. Reads replace the grid
 * (onResult); writes are confirmed with a dry run, then the table reloads.
 */
export function CommandPanel({
  databaseId,
  table,
  quotedTable,
  engine,
  canWrite,
  onResult,
  onWrote,
  onClose,
}: {
  databaseId: string;
  /** The table's name, used to keep a draft per table. */
  table: string;
  quotedTable: string;
  engine?: string;
  canWrite: boolean;
  /** Show a read's rows in place of the table (the last statement that returned rows). */
  onResult: (sql: string, result: QueryResult) => void;
  /** A write ran: reload the table and show it. */
  onWrote: () => void;
  onClose: () => void;
}) {
  const run = useRunQuery(databaseId);
  const columns = useSchemaColumns(databaseId);
  const completion = useMemo(() => sqlCompletion(columns.data, engine === "postgres"), [columns.data, engine]);
  const editorRef = useRef<SqlEditorHandle>(null);

  const draftKey = `command-draft:${databaseId}:${table}`;
  const [sql, setSqlState] = useState(() => load(draftKey) || `SELECT * FROM ${quotedTable} LIMIT 100;`);
  const setSql = (v: string) => {
    setSqlState(v);
    save(draftKey, v.trim() ? v.slice(0, 100_000) : null);
  };

  const [height, setHeight] = useState<number>(() => {
    const v = Number(load("command-panel-height"));
    return v >= HEIGHT_MIN ? v : HEIGHT_DEFAULT;
  });
  const startResize = (e: React.PointerEvent) => {
    e.preventDefault();
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    const startY = e.clientY;
    const startH = height;
    const maxH = Math.max(HEIGHT_MIN, window.innerHeight * 0.75);
    const clamp = (y: number) => Math.round(Math.max(HEIGHT_MIN, Math.min(maxH, startH + (startY - y))));
    const onMove = (ev: PointerEvent) => setHeight(clamp(ev.clientY));
    const onUp = (ev: PointerEvent) => {
      target.removeEventListener("pointermove", onMove);
      target.removeEventListener("pointerup", onUp);
      const h = clamp(ev.clientY);
      setHeight(h);
      save("command-panel-height", h === HEIGHT_DEFAULT ? null : String(h));
    };
    target.addEventListener("pointermove", onMove);
    target.addEventListener("pointerup", onUp);
  };

  const { push } = useToast();
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<PendingWrite | null>(null);
  const [checking, setChecking] = useState(false);
  const [queryId, setQueryId] = useState<string | null>(null);

  async function onRun() {
    const text = editorRef.current?.getQueryToRun() ?? sql.trim();
    if (!text || run.isPending || checking) return;
    const writes = writeStatements(text);
    if (writes.length === 0) {
      void execute(text);
      return;
    }
    if (!canWrite) {
      setError("This contains write statements, and you only have read access to this database.");
      return;
    }
    // Dry-run it (rolled back) so the confirm can say how many rows change.
    const blocked = previewBlocker(text);
    if (blocked) {
      setConfirm({ sql: text, writes, affected: null, note: blocked });
      return;
    }
    setError(null);
    setChecking(true);
    try {
      const out = await run.mutateAsync({ sql: text, limit: 1, query_id: randomUUID(), preview: true });
      if (out.error) {
        setError(`Statement ${out.error.statement} would fail: ${out.error.message}`);
        return;
      }
      const stmts = splitStatements(text);
      const affected = out.results
        .map((r, i) => ({ r, statement: stmts[i] ?? "" }))
        .filter(({ r }) => !r.read_only)
        .map(({ r, statement }) => ({ statement, rows: r.rows_affected }));
      setConfirm({ sql: text, writes, affected });
    } catch (err) {
      setConfirm({
        sql: text,
        writes,
        affected: null,
        note: err instanceof ApiError ? `The dry run failed: ${err.message}` : "The dry run failed.",
      });
    } finally {
      setChecking(false);
    }
  }

  async function execute(text: string) {
    setError(null);
    setConfirm(null);
    const id = randomUUID();
    setQueryId(id);
    try {
      const out: QueryOutput = await run.mutateAsync({ sql: text, limit: LIMIT, query_id: id, preview: false });
      if (out.error) setError(`Statement ${out.error.statement} failed: ${out.error.message}`);
      const wrote = out.results.filter((r) => !r.read_only);
      if (wrote.length > 0) {
        const rows = wrote.reduce((n, r) => n + r.rows_affected, 0);
        push("success", `${rows.toLocaleString()} row${rows === 1 ? "" : "s"} affected`);
        onWrote();
        return;
      }
      const rows = [...out.results].reverse().find((r) => r.columns.length > 0);
      if (rows) onResult(text, rows);
    } catch (err) {
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

  return (
    <section className="dbx-cmd" style={{ height }} aria-label="Command panel">
      <div
        className="dbx-cmd-resize"
        role="separator"
        aria-orientation="horizontal"
        aria-label="Resize command panel"
        tabIndex={0}
        onPointerDown={startResize}
        onKeyDown={(e) => {
          if (e.key === "ArrowUp" || e.key === "ArrowDown") {
            e.preventDefault();
            const h = Math.max(HEIGHT_MIN, height + (e.key === "ArrowUp" ? 24 : -24));
            setHeight(h);
            save("command-panel-height", String(h));
          }
        }}
      />
      <div className="dbx-cmd-head">
        <span className="font-medium text-sm">Command</span>
        <span className="muted text-sm hide-sm">
          <span className="kbd">⌘/Ctrl</span> + <span className="kbd">Enter</span> to run
        </span>
        {error ? (
          <span className="dbx-cmd-error text-sm text-danger" role="alert" title={error}>
            {error}
          </span>
        ) : null}
        <div className="flex-1" />
        {run.isPending ? (
          <Button
            size="sm"
            variant="danger"
            icon={<Square size={13} aria-hidden />}
            onClick={() => void onCancel()}
            disabled={!queryId}
          >
            Cancel
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="primary"
          icon={<Play size={14} aria-hidden />}
          loading={run.isPending || checking}
          onClick={() => void onRun()}
          disabled={!sql.trim()}
        >
          Run
        </Button>
        <IconButton label="Close command panel" onClick={onClose}>
          <X size={15} aria-hidden />
        </IconButton>
      </div>
      <div className="dbx-cmd-body">
        <div className="dbx-cmd-editor">
          <SqlEditor
            ref={editorRef}
            value={sql}
            onChange={setSql}
            onRun={() => void onRun()}
            placeholder={`SELECT * FROM ${quotedTable} ...`}
            minHeight="100%"
            engine={engine}
            completion={completion}
          />
        </div>
      </div>
      <ConfirmModal
        open={confirm !== null}
        danger
        title={`Change data in ${quotedTable}?`}
        confirmLabel={confirm && confirm.writes.length > 1 ? `Run ${confirm.writes.length} write statements` : "Run write statement"}
        message={
          confirm ? (
            <>
              <p className="mt-0">
                {confirm.affected
                  ? summarize(confirm.affected)
                  : "This will modify the database."}{" "}
                There is no undo.
              </p>
              {confirm.note ? <p className="text-sm muted">{confirm.note}</p> : null}
              <pre className="dbx-review">
                {confirm.affected
                  ? confirm.affected.map((a) => `-- ${a.rows.toLocaleString()} row${a.rows === 1 ? "" : "s"}\n${a.statement}`).join("\n\n")
                  : confirm.writes.join("\n")}
              </pre>
            </>
          ) : null
        }
        busy={run.isPending}
        onConfirm={() => confirm && void execute(confirm.sql)}
        onCancel={() => setConfirm(null)}
      />
    </section>
  );
}

/** summarize: "It will change 3 rows." for the confirm dialog. */
function summarize(affected: { rows: number }[]): string {
  const rows = affected.reduce((n, a) => n + a.rows, 0);
  return `It will change ${rows.toLocaleString()} row${rows === 1 ? "" : "s"}${
    affected.length > 1 ? ` across ${affected.length} statements` : ""
  }.`;
}
