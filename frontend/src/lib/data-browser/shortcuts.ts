// The data browser's keyboard shortcuts, listed once for the welcome screen
// and the "?" help. Each entry is one or more key combinations (alternatives).

export type Shortcut = { keys: string[][]; label: string; welcome?: boolean };

export const DATA_BROWSER_SHORTCUTS: Shortcut[] = [
  { keys: [["Ctrl", "P"]], label: "Find a table", welcome: true },
  { keys: [["Alt", "N"]], label: "New SQL query", welcome: true },
  { keys: [["Alt", "["], ["Alt", "]"]], label: "Previous / next tab", welcome: true },
  { keys: [["Alt", "1…9"]], label: "Go to tab", welcome: true },
  { keys: [["Alt", "W"]], label: "Close tab", welcome: true },
  { keys: [["Ctrl", "S"]], label: "Save changes", welcome: true },
  { keys: [["Alt", "Shift", "F"]], label: "Full screen on / off", welcome: true },
  { keys: [["Alt", "PgUp"], ["Alt", "PgDn"]], label: "Previous / next page of rows" },
  { keys: [["Enter"], ["F2"]], label: "Edit the cell" },
  { keys: [["Shift", "Enter"]], label: "Show the row in the row panel" },
  { keys: [["Ctrl", "C"]], label: "Copy the cell or the selected rows" },
  { keys: [["Shift", "↑"], ["Shift", "↓"]], label: "Select rows" },
  { keys: [["Delete"]], label: "Delete the selected rows (until you save)" },
  { keys: [["Ctrl", "Enter"]], label: "Run the query (SQL console)" },
];

/** shortcutText renders a shortcut's first combination for compact lists ("Alt Shift F"). */
export const shortcutText = (s: Shortcut) => s.keys[0].join(" ");
