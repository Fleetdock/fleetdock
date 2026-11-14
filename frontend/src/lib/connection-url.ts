/**
 * connectionURL builds a credential-free URL apps can start from. ClickHouse
 * is its HTTP interface, which takes the database as ?database= (it ignores
 * the path); tlsMode picks http or https for it.
 */
export function connectionURL(protocol: string, host: string, port: number, database?: string, tlsMode?: string): string {
  if (protocol === "clickhouse") {
    const scheme = tlsMode === "required" ? "https" : "http";
    const db = database ? `?database=${encodeURIComponent(database)}` : "";
    return `${scheme}://${host}:${port}/${db}`;
  }
  const scheme = protocol === "postgresql" ? "postgresql" : "mysql";
  const db = database ? `/${encodeURIComponent(database)}` : "";
  return `${scheme}://${host}:${port}${db}`;
}
