/**
 * formatBytes renders a byte count with a binary unit (1.5 MB). Zero renders
 * as `empty` — "0 B" where a real zero is meaningful (a table's size), "—"
 * where it means "not known yet" (a backup still running, an unprobed size).
 */
export function formatBytes(n?: number | null, empty = "—"): string {
  if (!n) return empty;
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let u = 0;
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024;
    u++;
  }
  return `${v.toFixed(u === 0 ? 0 : 1)} ${units[u]}`;
}

/**
 * formatRelative renders a date relative to `now`: "just now", "5 min ago",
 * "3 h ago", "yesterday", "4 days ago", then the date itself for anything
 * older than a week. Future dates read "in 5 min".
 */
export function formatRelative(date: Date | string | number, now: number = Date.now()): string {
  const d = new Date(date).getTime();
  if (Number.isNaN(d)) return "—";
  const diff = Math.round((now - d) / 1000);
  const abs = Math.abs(diff);
  const fmt = (n: number, unit: string) => (diff >= 0 ? `${n} ${unit} ago` : `in ${n} ${unit}`);
  if (abs < 45) return diff >= 0 ? "just now" : "in a moment";
  if (abs < 90) return fmt(1, "min");
  if (abs < 3600) return fmt(Math.round(abs / 60), "min");
  if (abs < 86400) return fmt(Math.round(abs / 3600), "h");
  if (abs < 2 * 86400 && diff > 0) return "yesterday";
  if (abs < 7 * 86400) return fmt(Math.round(abs / 86400), "days");
  return new Date(d).toLocaleDateString();
}

/** formatDuration renders milliseconds as "850 ms", "12.3 s", "4 min 5 s", "2 h 3 min". */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "—";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${Math.round(s % 60)} s`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
}
