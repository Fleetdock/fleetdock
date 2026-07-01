"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { DatabaseIcon, KeyIcon, ServerIcon } from "./icons";

const NAV = [
  { href: "/servers", label: "Servers", Icon: ServerIcon },
  { href: "/databases", label: "Databases", Icon: DatabaseIcon },
  { href: "/tokens", label: "API Tokens", Icon: KeyIcon },
];

export function Sidebar() {
  const pathname = usePathname();
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
          <div className="font-semibold text-sm">db-manager</div>
          <div className="muted" style={{ fontSize: 11 }}>MariaDB control plane</div>
        </div>
      </div>
      <nav className="flex flex-col gap-1" style={{ padding: ".6rem" }}>
        {NAV.map(({ href, label, Icon }) => {
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
