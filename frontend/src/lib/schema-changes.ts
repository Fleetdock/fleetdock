import type { SchemaChange } from "./types";

/** changeLabel names what a change is about: "column items.qty". */
export function changeLabel(c: SchemaChange): string {
  let table = c.table ?? "";
  if (c.schema && c.schema !== "public" && table) table = `${c.schema}.${table}`;
  switch (c.kind) {
    case "table":
    case "options":
      return `${c.kind === "options" ? "table options" : "table"} ${table}`;
    case "column":
    case "index":
    case "constraint":
    case "trigger":
      return `${c.kind} ${table}.${c.name ?? ""}`;
  }
  const name = c.schema && c.schema !== "public" ? `${c.schema}.${c.name ?? ""}` : (c.name ?? "");
  return `${c.kind} ${name}`;
}
