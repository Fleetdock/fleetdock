"use client";

// A small, dependency-free SVG line/area chart for time-series metrics.
// Measures its own width (so text and strokes never stretch), draws a
// recessive grid with y ticks and time labels, and has a crosshair tooltip
// that works with the pointer and with the keyboard (←/→ when focused). A
// visually hidden table carries the same numbers for screen readers.

import { useEffect, useId, useRef, useState } from "react";

export type ChartPoint = { t: string; v: number | null };

const H = 132; // plot + axes, px
const PAD = { top: 8, right: 8, bottom: 20, left: 34 };

export function MetricChart({
  title,
  points,
  unit = "",
  color = "var(--chart-1)",
  max,
}: {
  title: string;
  points: ChartPoint[];
  unit?: string;
  color?: string;
  /** Fixed upper bound (e.g. 100 for percentages); otherwise auto-scaled. */
  max?: number;
}) {
  const wrap = useRef<HTMLDivElement>(null);
  const tableId = useId();
  const [width, setWidth] = useState(320);
  const [hover, setHover] = useState<number | null>(null);

  useEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) =>
      setWidth(Math.max(160, Math.round(e.contentRect.width))),
    );
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const values = points.map((p) => p.v).filter((v): v is number => v != null);
  const hasData = values.length > 0;
  const last = hasData ? values[values.length - 1] : null;
  const lo = 0;
  const hi = max ?? niceCeil(Math.max(1, ...values) * 1.1);

  const plotW = width - PAD.left - PAD.right;
  const plotH = H - PAD.top - PAD.bottom;
  const n = points.length;
  const x = (i: number) =>
    PAD.left + (n <= 1 ? plotW / 2 : (i / (n - 1)) * plotW);
  const y = (v: number) =>
    PAD.top + plotH - ((v - lo) / (hi - lo || 1)) * plotH;

  const { line, area } = (() => {
    const segs: string[] = [];
    const areaParts: string[] = [];
    let run: [number, number][] = [];
    const flush = () => {
      if (run.length) {
        areaParts.push(
          `M${run[0][0].toFixed(1)},${PAD.top + plotH} ` +
            run.map(([a, b]) => `L${a.toFixed(1)},${b.toFixed(1)}`).join(" ") +
            ` L${run[run.length - 1][0].toFixed(1)},${PAD.top + plotH} Z`,
        );
      }
      run = [];
    };
    points.forEach((p, i) => {
      if (p.v == null) {
        flush();
        return;
      }
      const px = x(i);
      const py = y(p.v);
      segs.push(`${run.length ? "L" : "M"}${px.toFixed(1)},${py.toFixed(1)}`);
      run.push([px, py]);
    });
    flush();
    return { line: segs.join(" "), area: areaParts.join(" ") };
  })();

  const ticks = [lo, (lo + hi) / 2, hi];
  const timeTicks =
    n > 1 ? [0, Math.floor((n - 1) / 2), n - 1] : n === 1 ? [0] : [];
  const spanMs =
    n > 1 ? Date.parse(points[n - 1].t) - Date.parse(points[0].t) : 0;

  const summary = hasData
    ? `${title}: latest ${fmt(last!)}${unit}, min ${fmt(Math.min(...values))}${unit}, max ${fmt(Math.max(...values))}${unit} over ${points.length} samples.`
    : `${title}: no data yet.`;

  const hp = hover != null ? points[hover] : null;

  function indexAt(clientX: number): number | null {
    const rect = wrap.current?.querySelector("svg")?.getBoundingClientRect();
    if (!rect || n === 0) return null;
    const rel = (clientX - rect.left - PAD.left) / plotW;
    return Math.max(0, Math.min(n - 1, Math.round(rel * (n - 1))));
  }

  return (
    <figure className="m-0 card chart-card">
      <figcaption className="chart-head">
        <span className="chart-title">{title}</span>
        <span className="chart-value">
          {last != null ? `${fmt(last)}${unit}` : "—"}
        </span>
      </figcaption>
      <div ref={wrap} className="relative">
        {hasData ? (
          <svg
            className="chart-svg"
            width={width}
            height={H}
            viewBox={`0 0 ${width} ${H}`}
            role="img"
            aria-label={summary}
            aria-describedby={tableId}
            tabIndex={0}
            onPointerMove={(e) => setHover(indexAt(e.clientX))}
            onPointerLeave={() => setHover(null)}
            onBlur={() => setHover(null)}
            onKeyDown={(e) => {
              if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
              e.preventDefault();
              setHover((h) => {
                const start = h ?? n - 1;
                return Math.max(
                  0,
                  Math.min(n - 1, start + (e.key === "ArrowRight" ? 1 : -1)),
                );
              });
            }}
          >
            {ticks.map((t) => (
              <g key={t}>
                <line
                  className="chart-grid"
                  x1={PAD.left}
                  x2={width - PAD.right}
                  y1={y(t)}
                  y2={y(t)}
                />
                <text
                  className="chart-axis"
                  x={PAD.left - 6}
                  y={y(t)}
                  dy="0.32em"
                  textAnchor="end"
                >
                  {fmtTick(t)}
                </text>
              </g>
            ))}
            {timeTicks.map((i, k) => (
              <text
                key={i}
                className="chart-axis"
                x={x(i)}
                y={H - 4}
                textAnchor={
                  k === 0 && timeTicks.length > 1
                    ? "start"
                    : k === timeTicks.length - 1 && timeTicks.length > 1
                      ? "end"
                      : "middle"
                }
              >
                {fmtTime(points[i].t, spanMs)}
              </text>
            ))}
            {area ? <path d={area} fill={color} opacity={0.1} /> : null}
            <path
              d={line}
              fill="none"
              stroke={color}
              strokeWidth={2}
              strokeLinejoin="round"
              strokeLinecap="round"
            />
            {hp && hover != null ? (
              <g>
                <line
                  className="chart-cross"
                  x1={x(hover)}
                  x2={x(hover)}
                  y1={PAD.top}
                  y2={PAD.top + plotH}
                />
                {hp.v != null ? (
                  <circle
                    cx={x(hover)}
                    cy={y(hp.v)}
                    r={4}
                    fill={color}
                    stroke="var(--panel)"
                    strokeWidth={2}
                  />
                ) : null}
              </g>
            ) : null}
          </svg>
        ) : (
          <div className="chart-empty">No data yet</div>
        )}
        {hp && hover != null ? (
          <div
            className="chart-tip"
            style={{
              left: Math.min(Math.max(x(hover) - 60, 0), width - 130),
              top: 0,
            }}
            aria-hidden
          >
            <div className="muted">
              {new Date(hp.t).toLocaleString(undefined, {
                month: "short",
                day: "numeric",
                hour: "2-digit",
                minute: "2-digit",
              })}
            </div>
            <div className="font-medium chart-tip-row">
              <i style={{ background: color }} />
              {hp.v != null ? `${fmt(hp.v)}${unit}` : "no sample"}
            </div>
          </div>
        ) : null}
      </div>
      <table id={tableId} className="sr-only">
        <caption>{title}</caption>
        <thead>
          <tr>
            <th>Time</th>
            <th>Value</th>
          </tr>
        </thead>
        <tbody>
          {points.slice(-24).map((p) => (
            <tr key={p.t}>
              <td>{new Date(p.t).toLocaleString()}</td>
              <td>{p.v != null ? `${fmt(p.v)}${unit}` : "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}

function fmt(v: number): string {
  if (Math.abs(v) >= 100) return v.toFixed(0);
  return v.toFixed(1).replace(/\.0$/, "");
}

function fmtTick(v: number): string {
  if (v >= 1_000_000)
    return `${(v / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
  if (v >= 1000) return `${(v / 1000).toFixed(1).replace(/\.0$/, "")}k`;
  return Number.isInteger(v) ? String(v) : v.toFixed(1);
}

function fmtTime(t: string, spanMs: number): string {
  const d = new Date(t);
  if (spanMs > 36 * 3600_000)
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return d.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

// niceCeil rounds up to 1, 2, 2.5 or 5 × 10^n so ticks are round numbers.
function niceCeil(v: number): number {
  const p = 10 ** Math.floor(Math.log10(v));
  for (const m of [1, 2, 2.5, 5, 10]) if (v <= m * p) return m * p;
  return 10 * p;
}
