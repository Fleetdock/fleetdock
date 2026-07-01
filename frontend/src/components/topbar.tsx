"use client";

import { useRouter } from "next/navigation";

import { clearToken } from "@/lib/auth";
import { useMe } from "@/lib/hooks";

import { LogOutIcon } from "./icons";
import { ThemeToggle } from "./theme-toggle";

export function Topbar() {
  const router = useRouter();
  const { data: me } = useMe();

  function logout() {
    clearToken();
    router.replace("/login");
  }

  return (
    <header
      className="flex items-center justify-between"
      style={{ height: 56, flexShrink: 0, borderBottom: "1px solid var(--border)", background: "var(--panel)" }}
    >
      <div style={{ paddingLeft: "1.5rem" }} />
      <div className="flex items-center gap-2" style={{ paddingRight: "1rem" }}>
        {me ? (
          <span className="muted text-sm" style={{ marginRight: ".25rem" }}>
            {me.email}
          </span>
        ) : null}
        <ThemeToggle />
        <button className="btn btn-ghost btn-sm" onClick={logout} aria-label="Log out">
          <LogOutIcon size={16} />
        </button>
      </div>
    </header>
  );
}
