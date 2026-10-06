"use client";

import type { ColumnSpec } from "@/lib/types";

/** Common column types offered as suggestions; any allowlisted type works. */
export const TYPE_SUGGESTIONS = [
  "int",
  "integer",
  "bigint",
  "smallint",
  "numeric(10,2)",
  "decimal(10,2)",
  "real",
  "double precision",
  "boolean",
  "varchar(255)",
  "char(10)",
  "text",
  "date",
  "timestamp",
  "timestamptz",
  "datetime",
  "time",
  "json",
  "jsonb",
  "uuid",
  "bytea",
  "blob",
];

export const emptyColumn = (): ColumnSpec => ({
  name: "",
  type: "varchar(255)",
  nullable: true,
  default: null,
});

/**
 * ColumnFields edits one column spec: name, type, NULL, default (a literal or
 * an expression such as CURRENT_TIMESTAMP) and auto-increment.
 */
export function ColumnFields({
  value,
  onChange,
  compact,
  hideDefault,
}: {
  value: ColumnSpec;
  onChange: (c: ColumnSpec) => void;
  compact?: boolean;
  /** Hide the default / auto-increment controls (the column keeps its own). */
  hideDefault?: boolean;
}) {
  const set = (patch: Partial<ColumnSpec>) => onChange({ ...value, ...patch });
  const hasDefault = value.default !== null && value.default !== undefined;
  return (
    <div
      className="flex items-center gap-2"
      style={{ flexWrap: "wrap", marginBottom: compact ? ".35rem" : ".6rem" }}
    >
      <input
        className="input"
        style={{ width: 170 }}
        placeholder="column name"
        value={value.name}
        onChange={(e) => set({ name: e.target.value })}
        aria-label="Column name"
        required
      />
      <input
        className="input"
        style={{ width: 160 }}
        list="fd-column-types"
        placeholder="type"
        value={value.type}
        onChange={(e) => set({ type: e.target.value })}
        aria-label="Column type"
        required
      />
      <datalist id="fd-column-types">
        {TYPE_SUGGESTIONS.map((t) => (
          <option key={t} value={t} />
        ))}
      </datalist>
      <label className="flex items-center gap-1 text-sm">
        <input
          type="checkbox"
          checked={value.nullable}
          onChange={(e) => set({ nullable: e.target.checked })}
        />{" "}
        NULL
      </label>
      {hideDefault ? null : (
        <>
          <label
            className="flex items-center gap-1 text-sm"
            title="Auto-increment / identity (integer columns)"
          >
            <input
              type="checkbox"
              checked={Boolean(value.auto_increment)}
              onChange={(e) =>
                set({
                  auto_increment: e.target.checked,
                  default: e.target.checked ? null : value.default,
                })
              }
            />{" "}
            auto
          </label>
          <label className="flex items-center gap-1 text-sm">
            <input
              type="checkbox"
              checked={hasDefault}
              disabled={Boolean(value.auto_increment)}
              onChange={(e) => set({ default: e.target.checked ? "" : null })}
            />{" "}
            default
          </label>
          {hasDefault ? (
            <>
              <input
                className="input"
                style={{ width: 160 }}
                placeholder={
                  value.default_is_expression ? "CURRENT_TIMESTAMP" : "value"
                }
                value={value.default ?? ""}
                onChange={(e) => set({ default: e.target.value })}
                aria-label="Default"
              />
              <label
                className="flex items-center gap-1 text-sm"
                title="Treat as an expression (CURRENT_TIMESTAMP, NOW(), NULL, TRUE, a number)"
              >
                <input
                  type="checkbox"
                  checked={Boolean(value.default_is_expression)}
                  onChange={(e) =>
                    set({ default_is_expression: e.target.checked })
                  }
                />{" "}
                expr
              </label>
            </>
          ) : null}
        </>
      )}
    </div>
  );
}
