import {
  Activity,
  Archive,
  Database,
  LayoutDashboard,
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

export type NavSection = {
  label: string;
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
 * NAV is the whole dashboard structure: eight sections, each a set of pages.
 * The sidebar, section tabs, breadcrumbs and command palette all derive from it.
 */
export const NAV: NavSection[] = [
  {
    label: "Overview",
    Icon: LayoutDashboard,
    href: "/dashboard",
    match: ["/dashboard"],
    pages: [{ href: "/dashboard", label: "Overview", keywords: "home dashboard health attention" }],
  },
  {
    label: "Servers",
    Icon: Server,
    href: "/servers",
    match: ["/servers"],
    pages: [{ href: "/servers", label: "Servers", requires: [{ perm: "server:read", scoped: true }], keywords: "hosts agents connect" }],
  },
  {
    label: "Databases",
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
    label: "Backups",
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
    Icon: Activity,
    href: "/activity",
    match: ["/activity"],
    pages: [{ href: "/activity", label: "Activity", requires: [{ perm: "operation:read", scoped: true }], keywords: "operations jobs logs history" }],
  },
  {
    label: "Access",
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
    Icon: Settings,
    href: "/settings",
    match: ["/settings"],
    pages: [
      { href: "/settings/notifications", label: "Notifications", requires: [{ perm: "notification:read" }], keywords: "alerts email slack webhook" },
      { href: "/settings/profile", label: "Your profile", keywords: "password email account" },
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
