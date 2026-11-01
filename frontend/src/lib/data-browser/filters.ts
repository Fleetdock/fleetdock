// Filter operators of the data browser: what each needs, how it reads, and
// how the editor's draft (a list typed as text) becomes the API's filter.

import type { FilterOp, RowFilter } from "../types";

export type Arity = "none" | "one" | "list" | "range";

export const OPS: { op: FilterOp; label: string; arity: Arity }[] = [
  { op: "eq", label: "=", arity: "one" },
  { op: "ne", label: "≠", arity: "one" },
  { op: "lt", label: "<", arity: "one" },
  { op: "lte", label: "≤", arity: "one" },
  { op: "gt", label: ">", arity: "one" },
  { op: "gte", label: "≥", arity: "one" },
  { op: "between", label: "between", arity: "range" },
  { op: "in", label: "is one of", arity: "list" },
  { op: "not_in", label: "is none of", arity: "list" },
  { op: "contains", label: "contains", arity: "one" },
  { op: "not_contains", label: "doesn't contain", arity: "one" },
  { op: "starts_with", label: "starts with", arity: "one" },
  { op: "ends_with", label: "ends with", arity: "one" },
  { op: "is_null", label: "is NULL", arity: "none" },
  { op: "not_null", label: "is not NULL", arity: "none" },
];

/** The most values an in / not_in filter takes (the API's limit). */
export const MAX_LIST = 1000;

export const arityOf = (op: FilterOp): Arity => OPS.find((o) => o.op === op)?.arity ?? "one";
export const opLabel = (op: FilterOp): string => OPS.find((o) => o.op === op)?.label ?? op;
export const needsValue = (op: FilterOp) => arityOf(op) !== "none";

/** parseList splits a typed list on new lines or commas; blank entries are dropped. */
export function parseList(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s !== "")
    .slice(0, MAX_LIST);
}

/** toDraft prepares a filter for the editor, where a list is edited as text. */
export function toDraft(f: RowFilter): RowFilter {
  const a = arityOf(f.op);
  if (a === "list") return { column: f.column, op: f.op, value: (f.values ?? []).join(", ") };
  if (a === "range") return { column: f.column, op: f.op, values: [f.values?.[0] ?? "", f.values?.[1] ?? ""] };
  return f;
}

/** fromDraft turns an edited filter into what the API takes, or null when it is incomplete. */
export function fromDraft(f: RowFilter): RowFilter | null {
  if (!f.column) return null;
  switch (arityOf(f.op)) {
    case "none":
      return { column: f.column, op: f.op };
    case "list": {
      const values = f.values?.length ? f.values : parseList(f.value ?? "");
      return values.length ? { column: f.column, op: f.op, values } : null;
    }
    case "range": {
      const [lo, hi] = [f.values?.[0]?.trim() ?? "", f.values?.[1]?.trim() ?? ""];
      return lo !== "" && hi !== "" ? { column: f.column, op: f.op, values: [lo, hi] } : null;
    }
    default:
      return (f.value ?? "") !== "" ? { column: f.column, op: f.op, value: f.value } : null;
  }
}

/** describeValue is a filter's operand as shown on its chip. */
export function describeValue(f: RowFilter): string {
  switch (arityOf(f.op)) {
    case "none":
      return "";
    case "range":
      return `${f.values?.[0] ?? ""} … ${f.values?.[1] ?? ""}`;
    case "list": {
      const v = f.values ?? [];
      return v.length > 3 ? `${v.slice(0, 3).join(", ")} +${v.length - 3}` : v.join(", ");
    }
    default:
      return f.value ?? "";
  }
}

/** filterFor starts a filter on a column, typed to suit it. */
export function filterFor(column: string, textLike: boolean): RowFilter {
  return textLike ? { column, op: "contains", value: "" } : { column, op: "eq", value: "" };
}
