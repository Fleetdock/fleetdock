// Labels and filters for the audit log page. Pure, so it is unit-tested.

import type { AuditEvent } from "./types";

/** ACTION_LABEL names every action the API records. */
export const ACTION_LABEL: Record<string, string> = {
  "auth.login": "Signed in",
  "auth.logout": "Signed out",
  "profile.update": "Updated their profile",
  "profile.password_change": "Changed their password",
  "user.create": "Created user",
  "user.update": "Updated user",
  "user.password_reset": "Reset password",
  "user.delete": "Deleted user",
  "user.grant_add": "Granted a role",
  "user.grant_remove": "Removed a role",
  "role.create": "Created role",
  "role.update": "Updated role",
  "role.delete": "Deleted role",
  "api_token.create": "Created API token",
  "api_token.revoke": "Revoked API token",
  "agent_token.create": "Created agent install token",
  "agent_token.revoke": "Revoked agent install token",
  "server.register": "Added server",
  "server.update": "Updated server",
  "server.delete": "Removed server",
  "server.agent_token_revoke": "Revoked agent token",
  "server.agent_token_rotate": "Rotated agent token",
  "instance.connect": "Connected database server",
  "instance.provision": "Created database server",
  "instance.update": "Updated database server",
  "instance.delete": "Removed database server",
  "instance.start": "Started database server",
  "instance.stop": "Stopped database server",
  "instance.restart": "Restarted database server",
  "instance.import_databases": "Imported databases",
  "instance.kill_process": "Killed a query",
  "db_user.create": "Created database user",
  "db_user.drop": "Dropped database user",
  "db_user.password": "Changed database user password",
  "db_grant.add": "Granted privileges",
  "db_grant.revoke": "Revoked privileges",
  "database.create": "Created database",
  "database.lock": "Locked database",
  "database.unlock": "Unlocked database",
  "database.delete": "Deleted database",
  "database.move": "Moved database",
  "public_access.enable": "Enabled public access",
  "public_access.update_cidrs": "Changed allowed networks",
  "public_access.disable": "Disabled public access",
  "credential.create": "Created app credentials",
  "credential.rotate": "Rotated app credentials",
  "credential.revoke": "Revoked app credentials",
  "schema.watch": "Changed schema tracking",
  "table_view.create": "Saved a table view",
  "table_view.update": "Changed a table view",
  "table_view.delete": "Deleted a table view",
  "sql.execute": "Ran SQL that writes",
  "data.row_insert": "Inserted rows",
  "data.row_update": "Edited rows",
  "data.row_delete": "Deleted rows",
  "data.import_csv": "Imported CSV",
  "data.import_sql": "Imported SQL",
  "data.undo": "Undid changes",
  "data.restore_rows": "Restored rows from a backup",
  "data.export": "Exported data",
  "data.dump": "Downloaded a dump",
  "table.create": "Created table",
  "table.alter": "Changed table",
  "table.drop": "Dropped table",
  "table.truncate": "Emptied table",
  "table.rename": "Renamed table",
  "index.create": "Created index",
  "index.drop": "Dropped index",
  "backup_copy.create": "Opened a backup for comparison",
  "backup_copy.extend": "Kept a backup copy longer",
  "backup_copy.discard": "Discarded a backup copy",
  "fleet_run.create": "Started a fleet query",
  "fleet_run.resume": "Resumed a fleet query",
  "fleet_run.cancel": "Canceled a fleet query",
  "fleet_run.retry": "Retried a fleet query",
  "fleet_run.export": "Downloaded fleet query results",
  "backup.create": "Started a backup",
  "backup.restore": "Restored a backup",
  "backup.verify": "Verified a backup",
  "backup.delete": "Deleted a backup",
  "backup.download": "Downloaded a backup",
  "backup_destination.create": "Added backup storage",
  "backup_destination.update": "Updated backup storage",
  "backup_destination.delete": "Removed backup storage",
  "backup_schedule.create": "Created backup schedule",
  "backup_schedule.update": "Updated backup schedule",
  "backup_schedule.delete": "Deleted backup schedule",
  "notification_channel.create": "Added notification channel",
  "notification_channel.update": "Updated notification channel",
  "notification_channel.delete": "Removed notification channel",
  "alert_rule.create": "Created alert rule",
  "alert_rule.update": "Updated alert rule",
  "alert_rule.delete": "Deleted alert rule",
};

/** actionLabel names an action, falling back to its code for actions this build does not know. */
export function actionLabel(action: string): string {
  return ACTION_LABEL[action] ?? action;
}

/** Filter categories: the value is an action prefix the API accepts. */
export const ACTION_CATEGORIES: { group: string; options: { value: string; label: string }[] }[] = [
  {
    group: "People and access",
    options: [
      { value: "auth.", label: "Sign-ins" },
      { value: "user.", label: "Users" },
      { value: "role.", label: "Roles" },
      { value: "api_token.", label: "API tokens" },
      { value: "profile.", label: "Profiles" },
    ],
  },
  {
    group: "Infrastructure",
    options: [
      { value: "server.", label: "Servers" },
      { value: "agent_token.", label: "Agent install tokens" },
      { value: "instance.", label: "Database servers" },
      { value: "database.", label: "Databases" },
      { value: "public_access.", label: "Public access" },
      { value: "credential.", label: "App credentials" },
      { value: "db_user.", label: "Database users" },
      { value: "db_grant.", label: "Database privileges" },
    ],
  },
  {
    group: "Data",
    options: [
      { value: "sql.", label: "SQL console" },
      { value: "data.", label: "Data changes and exports" },
      { value: "table.", label: "Tables" },
      { value: "index.", label: "Indexes" },
      { value: "schema.", label: "Schema tracking" },
      { value: "table_view.", label: "Saved table views" },
      { value: "fleet_run.", label: "Fleet queries" },
    ],
  },
  {
    group: "Backups and alerts",
    options: [
      { value: "backup.", label: "Backups" },
      { value: "backup_copy.", label: "Backup copies" },
      { value: "backup_destination.", label: "Backup storage" },
      { value: "backup_schedule.", label: "Backup schedules" },
      { value: "notification_channel.", label: "Notification channels" },
      { value: "alert_rule.", label: "Alert rules" },
    ],
  },
];

export const PERIODS = [
  { value: "24h", label: "Last 24 hours", hours: 24 },
  { value: "7d", label: "Last 7 days", hours: 24 * 7 },
  { value: "30d", label: "Last 30 days", hours: 24 * 30 },
  { value: "90d", label: "Last 90 days", hours: 24 * 90 },
  { value: "all", label: "All time", hours: 0 },
] as const;

export type AuditFilters = {
  actor?: string;
  action?: string;
  outcome?: string;
  period?: string;
  resource_id?: string;
};

/** auditQuery turns the page's filters into the API's query string (without the cursor). */
export function auditQuery(f: AuditFilters, now: number = Date.now()): URLSearchParams {
  const q = new URLSearchParams();
  if (f.actor) q.set("actor", f.actor);
  if (f.action) q.set("action", f.action);
  if (f.outcome) q.set("outcome", f.outcome);
  if (f.resource_id) q.set("resource_id", f.resource_id);
  const period = PERIODS.find((p) => p.value === (f.period || "7d"));
  if (period && period.hours > 0) {
    // Rounded to the minute so the query key stays stable between renders.
    const since = Math.floor((now - period.hours * 3_600_000) / 60_000) * 60_000;
    q.set("since", new Date(since).toISOString());
  }
  return q;
}

/** actorLabel says who acted. */
export function actorLabel(e: AuditEvent): string {
  if (e.actor_email) return e.actor_email;
  if (e.actor_kind === "system") return "Fleetdock CLI";
  if (e.actor_kind === "anonymous") return "Unknown";
  return e.actor_user_id ? `Deleted user ${e.actor_user_id.slice(0, 8)}` : "Unknown";
}

/** auditResourceHref links an event's resource, when it has a page. */
export function auditResourceHref(e: AuditEvent): string | null {
  const id = e.resource_id;
  if (!id) return null;
  switch (e.resource_type) {
    case "database":
      return `/databases/${id}`;
    case "instance":
      return `/instances/${id}`;
    case "server":
      return `/servers/${id}`;
    case "operation":
      return `/activity/${id}`;
    case "fleet_run":
      return `/fleet/${id}`;
    case "user":
      return "/access/users";
    case "role":
      return "/access/roles";
    case "api_token":
      return "/access/tokens";
    case "backup":
      return "/backups";
    case "backup_schedule":
      return "/backups/schedules";
    case "backup_destination":
      return "/backups/storage";
    case "notification_channel":
    case "alert_rule":
      return "/settings/notifications";
    default:
      return null;
  }
}

/** detailLine summarises the metadata that matters most in a list row. */
export function detailLine(e: AuditEvent): string {
  const m = e.metadata ?? {};
  const parts: string[] = [];
  if (typeof m.table === "string") parts.push(m.table);
  if (typeof m.name === "string" && m.name !== e.resource_name) parts.push(m.name);
  if (typeof m.statements === "number") parts.push(`${m.statements} statement${m.statements === 1 ? "" : "s"}`);
  if (m.via === "cli") parts.push("from the command line");
  return parts.join(" · ");
}
