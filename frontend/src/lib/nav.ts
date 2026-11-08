import {
  Activity,
  Archive,
  Database,
  LayoutDashboard,
  Layers,
  Server,
  Settings,
  ShieldCheck,
  Table2,
  type LucideIcon,
} from "lucide-react";

/**
 * A permission requirement: the user may open the page when they hold `perm`
 * (globally, or at any scope when `scoped`). Several entries mean "any of".
 */
export type Requirement = { perm: string; scoped?: boolean };

export type NavPage = {
  href: string;
  label: string;
  /** Who may open it; empty = everyone signed in. */
  requires?: Requirement[];
  /** Shown in the command palette as extra search words. */
  keywords?: string;
};

/** Sidebar groups, in display order. */
export const NAV_GROUPS = ["Workspace", "Data", "Operations", "Admin"] as const;
export type NavGroup = (typeof NAV_GROUPS)[number];

export type NavSection = {
  label: string;
  /** Sidebar group heading the section sits under. */
  group: NavGroup;
  Icon: LucideIcon;
  /** The section's landing page. */
  href: string;
  /** Path prefixes that belong to this section (for highlighting). */
  match: string[];
  /** Pages inside the section, shown as tabs. */
  pages: NavPage[];
};

const backupRead: Requirement[] = [{ perm: "backup:read", scoped: true }];

/**
 * NAV is the whole dashboard structure: nine sections in four groups, each a
 * set of pages. The sidebar, section tabs, breadcrumbs and command palette all derive from it.
 */
export const NAV: NavSection[] = [
  {
    label: "Overview",
    group: "Workspace",
    Icon: LayoutDashboard,
    href: "/dashboard",
    match: ["/dashboard"],
    pages: [{ href: "/dashboard", label: "Overview", keywords: "home dashboard health attention" }],
  },
  {
    label: "Servers",
    group: "Workspace",
    Icon: Server,
    href: "/servers",
    match: ["/servers"],
    pages: [{ href: "/servers", label: "Servers", requires: [{ perm: "server:read", scoped: true }], keywords: "hosts agents connect" }],
  },
  {
    label: "Databases",
    group: "Workspace",
    Icon: Database,
    href: "/databases",
    match: ["/databases", "/instances"],
    pages: [
      {
        href: "/databases",
        label: "Databases",
        requires: [
          { perm: "database:read", scoped: true },
          { perm: "instance:read", scoped: true },
        ],
        keywords: "database servers instances mysql mariadb postgres tables sql console",
      },
    ],
  },
  {
    label: "Data browser",
    group: "Data",
    Icon: Table2,
    href: "/data",
    match: ["/data"],
    pages: [
      {
        href: "/data",
        label: "Data browser",
        requires: [{ perm: "database:read", scoped: true }],
        keywords: "tables rows views browse grid edit data sql query",
      },
    ],
  },
  {
    label: "Fleet query",
    group: "Data",
    Icon: Layers,
    href: "/fleet",
    match: ["/fleet"],
    pages: [
      {
        href: "/fleet",
        label: "Fleet query",
        requires: [{ perm: "database:read", scoped: true }],
        keywords: "run sql on many databases all servers bulk multi fleet query",
      },
    ],
  },
  {
    label: "Backups",
    group: "Operations",
    Icon: Archive,
    href: "/backups",
    match: ["/backups"],
    pages: [
      { href: "/backups", label: "History", requires: backupRead, keywords: "backups restore download" },
      { href: "/backups/schedules", label: "Schedules", requires: [{ perm: "schedule:read" }], keywords: "cron automatic backups retention" },
      { href: "/backups/storage", label: "Storage", requires: [{ perm: "destination:read" }], keywords: "s3 r2 bucket destination" },
    ],
  },
  {
    label: "Activity",
    group: "Operations",
    Icon: Activity,
    href: "/activity",
    match: ["/activity"],
    pages: [
      { href: "/activity", label: "Tasks", requires: [{ perm: "operation:read", scoped: true }], keywords: "operations jobs logs history" },
      {
        href: "/activity/audit",
        label: "Audit log",
        requires: [{ perm: "audit:read" }],
        keywords: "audit trail who changed security sign-in login events compliance",
      },
    ],
  },
  {
    label: "Access",
    group: "Admin",
    Icon: ShieldCheck,
    href: "/access",
    match: ["/access"],
    pages: [
      { href: "/access/users", label: "Users", requires: [{ perm: "user:read" }], keywords: "people accounts invite" },
      { href: "/access/roles", label: "Roles", requires: [{ perm: "user:read" }], keywords: "permissions rbac" },
      { href: "/access/tokens", label: "API tokens", requires: [{ perm: "token:read" }], keywords: "api keys automation" },
    ],
  },
  {
    label: "Settings",
    group: "Admin",
    Icon: Settings,
    href: "/settings",
    match: ["/settings"],
    pages: [
      { href: "/settings/notifications", label: "Notifications", requires: [{ perm: "notification:read" }], keywords: "alerts email slack webhook" },
      { href: "/settings/profile", label: "Your profile", keywords: "password email account" },
      { href: "/settings/appearance", label: "Appearance", keywords: "theme dark light mode style classic neutral borderless borders" },
    ],
  },
];

/** Allowed reports whether the requirements are met, given permission checkers. */
export function allowed(
  requires: Requirement[] | undefined,
  can: (perm: string) => boolean,
  canAny: (perm: string) => boolean,
): boolean {
  if (!requires || requires.length === 0) return true;
  return requires.some((r) => (r.scoped ? canAny(r.perm) : can(r.perm)));
}

/** Old URLs, kept working after the navigation restructure (see next.config.mjs). */
export { default as LEGACY_REDIRECTS } from "./legacy-redirects.json";
