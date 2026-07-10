"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { Activity, Archive, Bell, Box, CalendarClock, Cloud, Database, Key, LayoutDashboard, ScrollText, Server, ShieldCheck, Users, type LucideIcon } from "lucide-react";

import { useCan, useCanAny } from "@/lib/hooks";

// perm: the read permission required to see the section (empty = always).
// scoped: resource sections a user may reach with a scoped (non-global) grant.
const NAV: { href: string; label: string; Icon: LucideIcon; perm: string; scoped?: boolean }[] = [
  { href: "/dashboard", label: "Dashboard", Icon: LayoutDashboard, perm: "" },
  { href: "/servers", label: "Servers", Icon: Server, perm: "server:read", scoped: true },
  { href: "/instances", label: "Instances", Icon: Box, perm: "instance:read", scoped: true },
  { href: "/databases", label: "Databases", Icon: Database, perm: "database:read", scoped: true },
  { href: "/backups", label: "Backups", Icon: Archive, perm: "backup:read", scoped: true },
  { href: "/schedules", label: "Schedules", Icon: CalendarClock, perm: "schedule:read" },
  { href: "/destinations", label: "Destinations", Icon: Cloud, perm: "destination:read" },
  { href: "/operations", label: "Operations", Icon: Activity, perm: "operation:read", scoped: true },
  { href: "/notifications", label: "Notifications", Icon: Bell, perm: "notification:read" },
  { href: "/audit", label: "Audit log", Icon: ScrollText, perm: "audit:read" },
  { href: "/users", label: "Users", Icon: Users, perm: "user:read" },
  { href: "/roles", label: "Roles", Icon: ShieldCheck, perm: "user:read" },
  { href: "/tokens", label: "API Tokens", Icon: Key, perm: "token:read" },
];

export function Sidebar() {
  const pathname = usePathname();
  const can = useCan();
  const canAny = useCanAny();
  return (
    <aside
      className="flex flex-col"
      style={{ width: 232, flexShrink: 0, borderRight: "1px solid var(--border)", background: "var(--panel)" }}
    >
      <div
        className="flex items-center gap-2"
        style={{ padding: "1.05rem 1rem", borderBottom: "1px solid var(--border)" }}
      >
        <div
          className="flex items-center justify-center"
          style={{ width: 26, height: 26, borderRadius: 7, background: "var(--primary)", color: "var(--primary-fg)", fontWeight: 700, fontSize: 12 }}
        >
          db
        </div>
        <div>
          <div className="font-semibold text-sm">Fleetdock</div>
          <div className="muted" style={{ fontSize: 11 }}>MariaDB control plane</div>
        </div>
      </div>
      <nav className="flex flex-col gap-1" style={{ padding: ".6rem" }}>
        {NAV.filter(({ perm, scoped }) => !perm || (scoped ? canAny(perm) : can(perm))).map(({ href, label, Icon }) => {
          const active = pathname === href || pathname.startsWith(`${href}/`);
          return (
            <Link key={href} href={href} className={`sidebar-link${active ? " active" : ""}`}>
              <Icon size={17} />
              {label}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
