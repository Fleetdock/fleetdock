// Display labels shared by the notification channel and alert rule views.

export const CHANNEL_LABEL: Record<string, string> = {
  email: "Email",
  slack: "Slack",
  webhook: "Webhook",
};

export const METRIC_LABEL: Record<string, string> = {
  cpu_pct: "CPU %",
  mem_used_pct: "Memory used %",
  disk_used_pct: "Disk used %",
  connections: "Connections",
};

export const COMPARATOR_LABEL: Record<string, string> = {
  gt: ">",
  gte: "≥",
  lt: "<",
  lte: "≤",
};
