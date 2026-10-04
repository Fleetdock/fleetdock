const DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

const num = (s: string, min: number, max: number) => /^\d+$/.test(s) && +s >= min && +s <= max;
const pad = (n: string) => n.padStart(2, "0");
const ordinal = (n: number) => {
  const s = ["th", "st", "nd", "rd"];
  const v = n % 100;
  return n + (s[(v - 20) % 10] || s[v] || s[0]);
};

/**
 * describeCron puts a 5-field cron expression (UTC) into words, e.g.
 * "0 2 * * *" → "Every day at 02:00 UTC". Returns null for expressions it
 * can't describe simply (they still work; the UI then shows the raw value).
 */
export function describeCron(expr: string): string | null {
  const f = expr.trim().split(/\s+/);
  if (f.length !== 5) return null;
  const [min, hour, dom, mon, dow] = f;
  if (mon !== "*") return null;
  const step = (s: string) => (/^\*\/\d+$/.test(s) ? +s.slice(2) : null);

  if (min === "*" && hour === "*" && dom === "*" && dow === "*") return "Every minute";
  const minStep = step(min);
  if (minStep && hour === "*" && dom === "*" && dow === "*") return `Every ${minStep} minutes`;
  if (!num(min, 0, 59)) return null;
  if (hour === "*" && dom === "*" && dow === "*") return min === "0" ? "Every hour" : `Every hour at :${pad(min)}`;
  const hourStep = step(hour);
  if (hourStep && dom === "*" && dow === "*") return `Every ${hourStep} hours`;
  if (!num(hour, 0, 23)) return null;
  const at = `at ${pad(hour)}:${pad(min)} UTC`;
  if (dom === "*" && dow === "*") return `Every day ${at}`;
  if (dom === "*" && num(dow, 0, 7)) return `Every ${DAYS[+dow % 7]} ${at}`;
  if (dom === "*" && dow === "1-5") return `Every weekday ${at}`;
  if (dow === "*" && num(dom, 1, 31)) return `On the ${ordinal(+dom)} of every month ${at}`;
  return null;
}

/** isCron reports whether expr has the 5 fields a schedule needs. */
export function isCron(expr: string): boolean {
  return /^(\S+\s+){4}\S+$/.test(expr.trim());
}
