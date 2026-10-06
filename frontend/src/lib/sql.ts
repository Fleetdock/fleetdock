/**
 * Client-side mirror of the server's console statement handling
 * (backend/internal/platform/engine/sqlsplit.go and admin_shared.go). It is
 * only used to warn before running writes; the server re-checks everything.
 */

const READ_ONLY = new Set([
  "SELECT",
  "SHOW",
  "DESCRIBE",
  "DESC",
  "EXPLAIN",
  "WITH",
  "TABLE",
]);

/** leadingKeyword returns the first SQL keyword, skipping comments and "(". */
export function leadingKeyword(sql: string): string {
  let s = sql;
  for (;;) {
    s = s.replace(/^[\s(]+/, "");
    if (s.startsWith("--") || s.startsWith("#")) {
      const i = s.search(/[\r\n]/);
      if (i < 0) return "";
      s = s.slice(i + 1);
      continue;
    }
    if (s.startsWith("/*")) {
      const i = s.indexOf("*/");
      if (i < 0) return "";
      s = s.slice(i + 2);
      continue;
    }
    break;
  }
  const m = /^[A-Za-z0-9_]+/.exec(s);
  return m ? m[0].toUpperCase() : "";
}

/** isReadOnly reports whether a statement is classified as a read. */
export function isReadOnly(stmt: string): boolean {
  const kw = leadingKeyword(stmt);
  if (!READ_ONLY.has(kw)) return false;
  // EXPLAIN ANALYZE executes the statement it explains.
  if (
    (kw === "EXPLAIN" || kw === "DESC" || kw === "DESCRIBE") &&
    /\bANALYZE\b/i.test(stmt)
  )
    return false;
  return true;
}

/** splitStatements splits a script on ";" outside literals, comments and dollar quotes. */
export function splitStatements(script: string): string[] {
  const out: string[] = [];
  let cur = "";
  let delim = ";";
  let i = 0;
  const s = script;
  const flush = () => {
    const t = cur.trim();
    cur = "";
    if (t && leadingKeyword(t)) out.push(t);
  };
  while (i < s.length) {
    if ((i === 0 || s[i - 1] === "\n") && cur.trim() === "") {
      const m = /^[ \t]*DELIMITER[ \t]+(\S+)[ \t]*(\r?\n|$)/i.exec(s.slice(i));
      if (m) {
        cur = "";
        delim = m[1];
        i += m[0].length;
        continue;
      }
    }
    const c = s[i];
    if (s.startsWith(delim, i)) {
      flush();
      i += delim.length;
      continue;
    }
    if (c === "'" || c === '"' || c === "`") {
      let j = i + 1;
      while (j < s.length) {
        if (s[j] === "\\" && c === "'") {
          j += 2;
          continue;
        }
        if (s[j] === c) {
          if (s[j + 1] === c) {
            j += 2;
            continue;
          }
          j++;
          break;
        }
        j++;
      }
      cur += s.slice(i, j);
      i = j;
      continue;
    }
    if (s.startsWith("--", i) || c === "#") {
      const j = s.indexOf("\n", i);
      const end = j < 0 ? s.length : j;
      cur += s.slice(i, end);
      i = end;
      continue;
    }
    if (s.startsWith("/*", i)) {
      const j = s.indexOf("*/", i + 2);
      const end = j < 0 ? s.length : j + 2;
      cur += s.slice(i, end);
      i = end;
      continue;
    }
    if (c === "$") {
      const m = /^\$([A-Za-z_][A-Za-z0-9_]*)?\$/.exec(s.slice(i));
      if (m) {
        const tag = m[0];
        const j = s.indexOf(tag, i + tag.length);
        const end = j < 0 ? s.length : j + tag.length;
        cur += s.slice(i, end);
        i = end;
        continue;
      }
    }
    cur += c;
    i++;
  }
  flush();
  return out;
}

/** writeStatements returns the statements of a script that are not reads. */
export function writeStatements(script: string): string[] {
  return splitStatements(script).filter((s) => !isReadOnly(s));
}
