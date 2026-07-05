"use client";

// A small, dependency-free SVG area chart for time-series metrics. Renders in
// a fixed viewBox and scales responsively; theme-aware via CSS variables.

export type ChartPoint = { t: string; v: number | null };

export function MetricChart({
  title,
  points,
  unit = "",
  color = "var(--accent)",
  max,
}: {
  title: string;
  points: ChartPoint[];
  unit?: string;
  color?: string;
  /** Fixed upper bound (e.g. 100 for percentages); otherwise auto-scaled. */
  max?: number;
}) {
  const values = points.map((p) => p.v).filter((v): v is number => v != null);
  const hasData = values.length > 0;

  const W = 320;
  const H = 90;
  const pad = 4;

  let body = null;
  let latestLabel = "—";

  if (hasData) {
    const lo = 0;
    const hi = max ?? Math.max(1, ...values) * 1.15;
    const span = hi - lo || 1;
    const n = points.length;
    const x = (i: number) => (n <= 1 ? W / 2 : pad + (i / (n - 1)) * (W - 2 * pad));
    const y = (v: number) => H - pad - ((v - lo) / span) * (H - 2 * pad);

    // Build the line across defined points (gaps skipped).
    const segments: string[] = [];
    let started = false;
    points.forEach((p, i) => {
      if (p.v == null) {
        started = false;
        return;
      }
      segments.push(`${started ? "L" : "M"}${x(i).toFixed(1)},${y(p.v).toFixed(1)}`);
      started = true;
    });
    const line = segments.join(" ");

    // Area under the line (only when contiguous enough — simple version).
    const firstIdx = points.findIndex((p) => p.v != null);
    const lastIdx = points.length - 1 - [...points].reverse().findIndex((p) => p.v != null);
    const area =
      firstIdx >= 0
        ? `M${x(firstIdx).toFixed(1)},${H - pad} ` +
          points
            .map((p, i) => (p.v == null ? null : `L${x(i).toFixed(1)},${y(p.v).toFixed(1)}`))
            .filter(Boolean)
            .join(" ") +
          ` L${x(lastIdx).toFixed(1)},${H - pad} Z`
        : "";

    const last = values[values.length - 1];
    latestLabel = `${formatNum(last)}${unit}`;

    body = (
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" style={{ width: "100%", height: 90, display: "block" }}>
        {area ? <path d={area} fill={color} opacity={0.12} /> : null}
        <path d={line} fill="none" stroke={color} strokeWidth={1.5} vectorEffect="non-scaling-stroke" />
      </svg>
    );
  } else {
    body = (
      <div className="flex items-center justify-center muted text-sm" style={{ height: 90 }}>
        No data yet
      </div>
    );
  }

  return (
    <div className="card" style={{ padding: ".9rem 1rem" }}>
      <div className="flex items-center justify-between" style={{ marginBottom: ".4rem" }}>
        <span className="muted text-sm font-medium">{title}</span>
        <span className="font-semibold text-sm">{latestLabel}</span>
      </div>
      {body}
    </div>
  );
}

function formatNum(v: number): string {
  if (Math.abs(v) >= 100) return v.toFixed(0);
  return v.toFixed(1).replace(/\.0$/, "");
}
