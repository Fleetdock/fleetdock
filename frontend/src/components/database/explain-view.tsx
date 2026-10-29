"use client";

import { useMemo, useState } from "react";

import { AlertTriangle, ChevronDown, ChevronRight } from "lucide-react";

import { parsePlan, walk, type Plan, type PlanNode } from "@/lib/explain";
import type { ExplainResult } from "@/lib/types";

// ExplainView draws a query plan as a collapsible tree. Each step shows the
// rows the planner expected (and, analyzed, the rows it got) and a bar for its
// share of the total time — or of the total cost when nothing ran. Steps
// with a warning or most of the time are highlighted.

const HOT_SHARE = 0.3;

function fmtMs(ms: number) {
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)} s` : ms >= 10 ? `${ms.toFixed(0)} ms` : `${ms.toFixed(2)} ms`;
}

function fmtRows(n: number | undefined) {
  if (n === undefined) return "";
  return n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e4 ? `${(n / 1e3).toFixed(0)}k` : String(Math.round(n));
}

export function ExplainView({ result }: { result: ExplainResult }) {
  const [raw, setRaw] = useState(false);
  const parsed = useMemo((): { plan?: Plan; error?: string } => {
    try {
      return { plan: parsePlan(result) };
    } catch (e) {
      return { error: e instanceof Error ? e.message : String(e) };
    }
  }, [result]);
  const plan = parsed.plan;

  const warnings = useMemo(() => {
    const out: string[] = [];
    if (plan) walk(plan.root, (n) => n.warnings.forEach((w) => out.includes(w) || out.push(w)));
    return out;
  }, [plan]);

  // What the bars measure: time when the statement ran, else cost.
  const byTime = Boolean(plan?.analyzed && plan.root.timeMs);
  const total = plan ? (byTime ? plan.root.timeMs : maxCost(plan.root)) : undefined;

  return (
    <div className="explain card">
      <div className="explain-head">
        <span className={`badge ${plan?.analyzed ? "badge-green" : "badge-gray"}`}>
          {plan?.analyzed ? "Analyzed — the statement ran and was rolled back" : "Estimated plan — nothing was run"}
        </span>
        {plan?.totalTimeMs !== undefined ? <span className="text-sm muted">Execution {fmtMs(plan.totalTimeMs)}</span> : null}
        {plan?.planningTimeMs !== undefined ? <span className="text-sm muted">Planning {fmtMs(plan.planningTimeMs)}</span> : null}
        <span style={{ flex: 1 }} />
        <button className={`btn btn-ghost btn-sm${raw ? " btn-primary" : ""}`} onClick={() => setRaw(!raw)}>
          {raw ? "Tree" : "Raw"}
        </button>
      </div>

      {warnings.length ? (
        <ul className="explain-warnings">
          {warnings.map((w) => (
            <li key={w}>
              <AlertTriangle size={13} aria-hidden /> {w}
            </li>
          ))}
        </ul>
      ) : plan ? (
        <p className="text-sm muted" style={{ margin: "0 0 .6rem" }}>
          Nothing stands out in this plan.
        </p>
      ) : null}

      {raw || !plan ? (
        <>
          {parsed.error ? <p className="text-sm muted">This plan could not be drawn ({parsed.error}); here it is as returned.</p> : null}
          <pre className="explain-raw">{result.text ?? JSON.stringify(result.plan, null, 2)}</pre>
        </>
      ) : (
        <div className="explain-tree">
          <div className="explain-row explain-colhead" aria-hidden>
            <span>Step</span>
            <span className="num">Rows{plan.analyzed ? " (est → actual)" : " (est)"}</span>
            <span className="num">{byTime ? "Time" : "Cost"}</span>
          </div>
          <PlanRow node={plan.root} depth={0} total={total} byTime={byTime} analyzed={plan.analyzed} />
        </div>
      )}
    </div>
  );
}

function maxCost(n: PlanNode): number | undefined {
  let m: number | undefined;
  walk(n, (x) => {
    if (x.cost !== undefined && (m === undefined || x.cost > m)) m = x.cost;
  });
  return m;
}

function PlanRow({ node, depth, total, byTime, analyzed }: { node: PlanNode; depth: number; total?: number; byTime: boolean; analyzed: boolean }) {
  const [open, setOpen] = useState(true);
  const value = byTime ? node.timeMs : node.cost;
  const share = total && value !== undefined ? Math.min(1, value / total) : undefined;
  const hot = node.warnings.length > 0 || (share !== undefined && share >= HOT_SHARE && depth > 0);
  const rows = analyzed && node.actualRows !== undefined
    ? `${fmtRows(node.estRows)} → ${fmtRows(node.actualRows)}${node.loops && node.loops > 1 ? ` ×${node.loops}` : ""}`
    : fmtRows(node.estRows);
  return (
    <div className="explain-node">
      <div className={`explain-row${hot ? " is-hot" : ""}`} style={{ paddingLeft: `${0.4 + depth * 1.1}rem` }}>
        <span className="explain-step">
          {node.children.length ? (
            <button className="explain-toggle" onClick={() => setOpen(!open)} aria-expanded={open} aria-label={open ? "Collapse step" : "Expand step"}>
              {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
            </button>
          ) : (
            <span className="explain-toggle" />
          )}
          <span>
            <span className="explain-label">
              {node.warnings.length ? <AlertTriangle size={12} className="explain-warn-icon" aria-label="Has a warning" /> : null}
              {node.label}
            </span>
            {node.detail ? <span className="explain-detail">{node.detail}</span> : null}
          </span>
        </span>
        <span className="num text-sm">{rows}</span>
        <span className="num text-sm explain-metric">
          {value !== undefined ? (byTime ? fmtMs(value) : value.toFixed(value >= 100 ? 0 : 2)) : ""}
          {share !== undefined ? (
            <span className="explain-bar" aria-hidden>
              <span style={{ width: `${Math.max(2, share * 100)}%` }} />
            </span>
          ) : null}
        </span>
      </div>
      {open ? node.children.map((c, i) => <PlanRow key={i} node={c} depth={depth + 1} total={total} byTime={byTime} analyzed={analyzed} />) : null}
    </div>
  );
}
