// What a saved table view keeps of a tab's view, and how it comes back.

import { parseViewState, type TableView } from "./tabs";

/** The parts of a tab's view a saved view stores. Paging and mode are left out. */
export type SavedState = Pick<TableView, "filters" | "sort" | "search" | "hidden" | "frozen" | "widths" | "pageSize">;

export function pickSavedState(v: TableView): SavedState {
  return {
    filters: v.filters,
    sort: v.sort,
    search: v.search,
    hidden: v.hidden ?? [],
    frozen: v.frozen ?? 0,
    widths: v.widths,
    pageSize: v.pageSize,
  };
}

/** applySavedState turns a stored state into a patch for the tab (validated, back to page 1, data mode). */
export function applySavedState(state: unknown, id: string): Partial<TableView> {
  const v = parseViewState(state);
  return { ...pickSavedState(v), mode: "data", page: 1, savedView: id };
}

// Column widths are a matter of taste; changing them does not count as
// drifting from the saved view.
const comparable = (s: SavedState) =>
  JSON.stringify([s.filters, s.sort, s.search.trim(), [...(s.hidden ?? [])].sort(), s.frozen ?? 0, s.pageSize]);

/** isModified reports that the tab no longer matches the saved view it was set to. */
export function isModified(view: TableView, state: unknown): boolean {
  return comparable(pickSavedState(view)) !== comparable(pickSavedState(parseViewState(state)));
}

/** savedViewLink is the link that opens a table in a saved view. */
export function savedViewLink(origin: string, databaseId: string, table: string, viewId: string): string {
  const q = new URLSearchParams({ db: databaseId, table, saved: viewId });
  return `${origin}/data?${q.toString()}`;
}
