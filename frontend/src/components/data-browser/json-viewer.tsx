"use client";

import { ChevronRight } from "lucide-react";
import { useState } from "react";

import { Button, SegmentedControl } from "@/components/ui";

/** Containers nested deeper than this start collapsed. */
const OPEN_DEPTH = 2;
/** Children shown per container before a "show more" button. */
const PAGE = 100;

type Expand = "default" | "all" | "none";

/** JsonViewer shows a parsed JSON document as a collapsible tree or as raw text. It only reads. */
export function JsonViewer({
  doc,
  compact,
}: {
  doc: object;
  /** Smaller toolbar and height cap, for the row panel's per-field view. */
  compact?: boolean;
}) {
  const [mode, setMode] = useState<"tree" | "raw">("tree");
  // Expand/collapse all remount the tree with a new default for every node.
  const [expand, setExpand] = useState<{ mode: Expand; gen: number }>({ mode: "default", gen: 0 });
  const setAll = (m: Expand) => setExpand((e) => ({ mode: m, gen: e.gen + 1 }));

  return (
    <div className={`dbx-json${compact ? " is-compact" : ""}`}>
      <div className="dbx-json-bar">
        <SegmentedControl
          size="sm"
          label="JSON view"
          value={mode}
          onChange={setMode}
          options={[
            { value: "tree", label: "Tree" },
            { value: "raw", label: "Raw" },
          ]}
        />
        {mode === "tree" ? (
          <>
            <div className="flex-1" />
            <Button size="sm" variant="ghost" onClick={() => setAll("all")}>
              Expand all
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setAll("none")}>
              Collapse all
            </Button>
          </>
        ) : null}
      </div>
      {mode === "raw" ? (
        <pre className="dbx-json-raw">{JSON.stringify(doc, null, 2)}</pre>
      ) : (
        <div className="dbx-json-tree" role="tree" aria-label="JSON document" key={expand.gen}>
          <JsonNode value={doc} depth={0} expand={expand.mode} last />
        </div>
      )}
    </div>
  );
}

function isContainer(v: unknown): v is object {
  return typeof v === "object" && v !== null;
}

function summary(v: object): string {
  if (Array.isArray(v)) return `${v.length} item${v.length === 1 ? "" : "s"}`;
  const n = Object.keys(v).length;
  return `${n} key${n === 1 ? "" : "s"}`;
}

function JsonNode({
  name,
  value,
  depth,
  expand,
  last,
}: {
  /** Object key (string) or array index (number); absent for the root. */
  name?: string | number;
  value: unknown;
  depth: number;
  expand: Expand;
  last: boolean;
}) {
  const container = isContainer(value);
  const [open, setOpen] = useState(
    expand === "all" ? true : expand === "none" ? depth === 0 : depth < OPEN_DEPTH,
  );
  const [shown, setShown] = useState(PAGE);
  const comma = last ? null : <span className="dbx-json-punct">,</span>;
  const label =
    name === undefined ? null : (
      <>
        <span className={typeof name === "number" ? "dbx-json-index" : "dbx-json-key"}>
          {typeof name === "number" ? name : JSON.stringify(name)}
        </span>
        <span className="dbx-json-punct">: </span>
      </>
    );

  if (!container) {
    return (
      <div className="dbx-json-row" role="treeitem" aria-selected={false} style={{ paddingLeft: `${depth + 1.15}em` }}>
        {label}
        <JsonScalar value={value} />
        {comma}
      </div>
    );
  }

  const isArray = Array.isArray(value);
  const entries: [string | number, unknown][] = isArray
    ? (value as unknown[]).map((v, i) => [i, v])
    : Object.entries(value);
  const [openB, closeB] = isArray ? ["[", "]"] : ["{", "}"];
  const empty = entries.length === 0;

  return (
    <div role="treeitem" aria-selected={false} aria-expanded={empty ? undefined : open}>
      <div className="dbx-json-row" style={{ paddingLeft: `${depth}em` }}>
        {empty ? (
          <span className="dbx-json-toggle" aria-hidden />
        ) : (
          <button
            type="button"
            className="dbx-json-toggle"
            aria-label={open ? "Collapse" : "Expand"}
            onClick={() => setOpen((o) => !o)}
          >
            <ChevronRight size={12} aria-hidden className={open ? "is-open" : undefined} />
          </button>
        )}
        {label}
        <span className="dbx-json-punct">{openB}</span>
        {empty ? (
          <>
            <span className="dbx-json-punct">{closeB}</span>
            {comma}
          </>
        ) : !open ? (
          <>
            <button type="button" className="dbx-json-summary" onClick={() => setOpen(true)}>
              {summary(value)}
            </button>
            <span className="dbx-json-punct">{closeB}</span>
            {comma}
          </>
        ) : null}
      </div>
      {open && !empty ? (
        <>
          <div role="group">
            {entries.slice(0, shown).map(([k, v], i) => (
              <JsonNode
                key={k}
                name={k}
                value={v}
                depth={depth + 1}
                expand={expand}
                last={i === entries.length - 1}
              />
            ))}
            {entries.length > shown ? (
              <div className="dbx-json-row" style={{ paddingLeft: `${depth + 2.15}em` }}>
                <button type="button" className="dbx-json-summary" onClick={() => setShown((s) => s + PAGE)}>
                  Show {Math.min(PAGE, entries.length - shown)} more of {entries.length - shown}
                </button>
              </div>
            ) : null}
          </div>
          <div className="dbx-json-row" style={{ paddingLeft: `${depth + 1.15}em` }}>
            <span className="dbx-json-punct">{closeB}</span>
            {comma}
          </div>
        </>
      ) : null}
    </div>
  );
}

function JsonScalar({ value }: { value: unknown }) {
  if (value === null) return <span className="dbx-json-null">null</span>;
  switch (typeof value) {
    case "string":
      return <span className="dbx-json-string">{JSON.stringify(value)}</span>;
    case "number":
      return <span className="dbx-json-number">{String(value)}</span>;
    case "boolean":
      return <span className="dbx-json-bool">{String(value)}</span>;
    default:
      return <span>{String(value)}</span>;
  }
}
