/** Plain-language names for operation (activity) types. */
const TYPE_LABEL: Record<string, string> = {
  create_database: "Create database",
  delete_database: "Remove database",
  clone_database: "Copy database",
  rename_database: "Rename database",
  lock_database: "Lock database",
  unlock_database: "Unlock database",
  backup: "Backup",
  restore: "Restore",
  migrate: "Copy or move database",
  provision_instance: "Create database server",
  start_instance: "Start database server",
  stop_instance: "Stop database server",
  restart_instance: "Restart database server",
  remove_instance: "Remove database server",
  enroll_server: "Connect server",
  test_connection: "Connection check",
  import_databases: "Find databases",
  reconcile_gateway: "Update external access",
};

/** operationLabel names an operation type, e.g. "restart_instance" → "Restart database server". */
export function operationLabel(type: string): string {
  if (TYPE_LABEL[type]) return TYPE_LABEL[type];
  const s = type.replace(/_/g, " ");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** operationTitle names an operation with its resource: "Backup · orders". */
export function operationTitle(op: { type: string; resource_name?: string }): string {
  const label = operationLabel(op.type);
  return op.resource_name ? `${label} · ${op.resource_name}` : label;
}

/** resourceHref links an operation's or alert's resource to its page. */
export function resourceHref(type: string, id?: string | null): string | null {
  if (!id) return null;
  switch (type) {
    case "database":
      return `/databases/${id}`;
    case "instance":
      return `/instances/${id}`;
    case "server":
      return `/servers/${id}`;
    case "backup":
      return "/backups";
    case "operation":
      return `/activity/${id}`;
    default:
      return null;
  }
}
