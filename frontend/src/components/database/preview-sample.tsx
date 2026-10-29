"use client";

import type { PreviewSample } from "@/lib/types";

const show = (v: string | null) => (v === null ? "NULL" : v);

/**
 * PreviewSampleView shows the rows a previewed UPDATE / DELETE touched: each
 * row as it was, with the changed cells struck through, and as the
 * statement left it, with them highlighted. Deleted rows are marked.
 */
export function PreviewSampleView({ sample }: { sample: PreviewSample }) {
  // Only columns that changed in some row, plus the key, unless nothing
  // changed (a delete): then every column.
  const changed = new Set<number>();
  sample.rows.forEach((r) =>
    r.after?.forEach((v, i) => {
      if (v !== r.before[i]) changed.add(i);
    }),
  );
  const cols = sample.columns
    .map((name, i) => ({ name, i }))
    .filter(({ name, i }) => changed.size === 0 || changed.has(i) || sample.key.includes(name));
  return (
    <div className="preview-sample">
      <div className="text-sm muted" style={{ marginBottom: ".35rem" }}>
        {sample.rows.length === 0
          ? `No rows of ${sample.table} match.`
          : `${sample.truncated ? "First " : ""}${sample.rows.length} row${sample.rows.length === 1 ? "" : "s"} of ${sample.table}, before → after`}
      </div>
      {sample.rows.length ? (
        <div style={{ overflowX: "auto" }}>
          <table className="table preview-table">
            <thead>
              <tr>
                <th />
                {cols.map((c) => (
                  <th key={c.name}>{c.name}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sample.rows.map((r, ri) => (
                <PreviewRows key={ri} row={r} cols={cols} />
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}

function PreviewRows({ row, cols }: { row: PreviewSample["rows"][number]; cols: { name: string; i: number }[] }) {
  return (
    <>
      <tr className="preview-before">
        <td className="muted text-sm">before</td>
        {cols.map((c) => (
          <td key={c.name} className={row.after && row.after[c.i] !== row.before[c.i] ? "is-old" : ""}>
            {show(row.before[c.i])}
          </td>
        ))}
      </tr>
      <tr className="preview-after">
        <td className="muted text-sm">after</td>
        {row.after ? (
          cols.map((c) => (
            <td key={c.name} className={row.after![c.i] !== row.before[c.i] ? "is-new" : "muted"}>
              {show(row.after![c.i])}
            </td>
          ))
        ) : (
          <td colSpan={cols.length} className="is-deleted">
            deleted
          </td>
        )}
      </tr>
    </>
  );
}
