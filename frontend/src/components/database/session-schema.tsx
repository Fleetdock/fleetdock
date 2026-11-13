"use client";

import { useState } from "react";

import { Select } from "@/components/ui";
import { load, save } from "@/lib/data-browser/storage";
import { useSchemas } from "@/lib/hooks";

/**
 * useSessionSchema keeps the schema a PostgreSQL console session looks in
 * first, per database in this browser. searchPath is what the query API takes:
 * the schema, then public (where extensions usually live); undefined keeps the
 * database's own search_path.
 */
export function useSessionSchema(databaseId: string, enabled: boolean) {
  const key = `session-schema:${databaseId}`;
  const [schema, setState] = useState(() => (enabled ? load(key) ?? "" : ""));
  const set = (v: string) => {
    setState(v);
    save(key, v || null);
  };
  const searchPath = enabled && schema ? (schema === "public" ? ["public"] : [schema, "public"]) : undefined;
  return { schema: enabled ? schema : "", setSchema: set, searchPath };
}

/** SessionSchemaSelect picks the session's first schema (PostgreSQL only). */
export function SessionSchemaSelect({
  databaseId,
  value,
  onChange,
}: {
  databaseId: string;
  value: string;
  onChange: (schema: string) => void;
}) {
  const { data } = useSchemas(databaseId, true);
  const names = (data ?? []).map((s) => s.name);
  // A remembered schema that was dropped since still shows, so it is clear
  // what the session uses.
  if (value && !names.includes(value)) names.unshift(value);
  return (
    <Select
      size="sm"
      className="w-auto"
      aria-label="Schema for names without one"
      title="Where names without a schema are looked up (search_path) in this session"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">Default search_path</option>
      {names.map((n) => (
        <option key={n} value={n}>
          {n === "public" ? "search_path: public" : `search_path: ${n}, public`}
        </option>
      ))}
    </Select>
  );
}
