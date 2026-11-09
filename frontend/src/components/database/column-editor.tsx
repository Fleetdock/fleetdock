"use client";

import { useState } from "react";

import { Input, Select, cx } from "@/components/ui";
import type { Dialect } from "@/lib/data-browser/cells";
import type { ColumnSpec } from "@/lib/types";

type TypeGroup = { label: string; types: string[] };

/**
 * Column types offered per engine. Each must pass the backend allowlist
 * (engine/structure.go); other allowed types go through "Custom…".
 */
export const TYPE_GROUPS: Record<Dialect, TypeGroup[]> = {
  mysql: [
    { label: "Integer", types: ["tinyint", "smallint", "mediumint", "int", "bigint", "int unsigned", "bigint unsigned"] },
    { label: "Decimal", types: ["decimal(10,2)", "float", "double"] },
    { label: "Boolean", types: ["boolean", "bit(1)"] },
    {
      label: "Text",
      types: ["char(10)", "varchar(50)", "varchar(100)", "varchar(255)", "varchar(1000)", "tinytext", "text", "mediumtext", "longtext"],
    },
    { label: "Date & time", types: ["date", "time", "datetime", "timestamp", "year"] },
    { label: "Binary", types: ["binary(16)", "varbinary(255)", "blob", "mediumblob", "longblob"] },
    { label: "Other", types: ["json", "geometry", "point"] },
  ],
  postgres: [
    { label: "Integer", types: ["smallint", "integer", "bigint"] },
    { label: "Decimal", types: ["numeric(10,2)", "real", "double precision", "money"] },
    { label: "Boolean", types: ["boolean"] },
    { label: "Text", types: ["char(10)", "varchar(50)", "varchar(100)", "varchar(255)", "text", "citext"] },
    { label: "Date & time", types: ["date", "time", "timetz", "timestamp", "timestamptz", "interval"] },
    { label: "Other", types: ["uuid", "json", "jsonb", "bytea", "inet", "cidr", "macaddr", "tsvector", "xml", "text[]", "integer[]"] },
  ],
};

const CUSTOM = "__custom__";

/** Lower-cases and collapses whitespace, like the backend's normalizeType. */
const normalizeType = (t: string) => t.trim().toLowerCase().replace(/\s+/g, " ");

const isListed = (type: string, dialect: Dialect) =>
  TYPE_GROUPS[dialect].some((g) => g.types.includes(normalizeType(type)));

export const emptyColumn = (): ColumnSpec => ({
  name: "",
  type: "varchar(255)",
  nullable: true,
  default: null,
});

/**
 * TypePicker picks one of the engine's common types, or any type via "Custom…".
 * A <select> lists every option, unlike <input list>, which filters by what is typed.
 */
function TypePicker({ value, onChange, dialect }: { value: string; onChange: (t: string) => void; dialect: Dialect }) {
  const [custom, setCustom] = useState(() => value !== "" && !isListed(value, dialect));
  const listed = !custom && isListed(value, dialect);
  return (
    <>
      <Select
        className="w-44"
        aria-label="Column type"
        value={listed ? normalizeType(value) : CUSTOM}
        onChange={(e) => {
          if (e.target.value === CUSTOM) {
            setCustom(true);
          } else {
            setCustom(false);
            onChange(e.target.value);
          }
        }}
      >
        {TYPE_GROUPS[dialect].map((g) => (
          <optgroup key={g.label} label={g.label}>
            {g.types.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </optgroup>
        ))}
        <option value={CUSTOM}>Custom…</option>
      </Select>
      {listed ? null : (
        <Input
          className="w-40"
          placeholder={dialect === "postgres" ? "e.g. varchar(80)" : "e.g. decimal(12,4)"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          aria-label="Custom column type"
          required
        />
      )}
    </>
  );
}

/**
 * ColumnFields edits one column spec: name, type, NULL, default (a literal or
 * an expression such as CURRENT_TIMESTAMP) and auto-increment.
 */
export function ColumnFields({
  value,
  onChange,
  compact,
  hideDefault,
  dialect = "mysql",
}: {
  value: ColumnSpec;
  onChange: (c: ColumnSpec) => void;
  compact?: boolean;
  /** Hide the default / auto-increment controls (the column keeps its own). */
  hideDefault?: boolean;
  /** Engine whose types the type picker offers. */
  dialect?: Dialect;
}) {
  const set = (patch: Partial<ColumnSpec>) => onChange({ ...value, ...patch });
  const hasDefault = value.default !== null && value.default !== undefined;
  return (
    <div className={cx("flex flex-wrap items-center gap-2", compact ? "mb-1.5" : "mb-2.5")}>
      <Input
        className="w-[170px]"
        placeholder="column name"
        value={value.name}
        onChange={(e) => set({ name: e.target.value })}
        aria-label="Column name"
        required
      />
      <TypePicker value={value.type} onChange={(type) => set({ type })} dialect={dialect} />
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
              <Input
                className="w-40"
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
