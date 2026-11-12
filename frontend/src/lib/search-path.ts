// PostgreSQL search_path helpers.

/** Splits a search_path as PostgreSQL prints it ("$user", public) into names. */
export function parseSearchPath(path: string): string[] {
  const out: string[] = [];
  const re = /\s*(?:"((?:[^"]|"")*)"|([^,\s][^,]*?))\s*(?:,|$)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(path)) && m[0] !== "") {
    out.push(m[1] !== undefined ? m[1].replace(/""/g, '"') : m[2]);
  }
  return out;
}
