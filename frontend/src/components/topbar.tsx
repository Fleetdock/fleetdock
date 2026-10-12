"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { useQueryClient } from "@tanstack/react-query";
import { CircleUser, LogOut, Menu as MenuIcon, Moon, Search, Sun, UserRound } from "lucide-react";

import { openCommandPalette } from "@/components/command-palette";
import { RunningOps } from "@/components/running-ops";
import { Menu } from "@/components/ui";
import { api } from "@/lib/api";
import { useMe } from "@/lib/hooks";

function useTheme(): [boolean, () => void] {
  const [dark, setDark] = useState(false);
  useEffect(() => {
    setDark(document.documentElement.classList.contains("dark"));
  }, []);
  const toggle = () => {
    const next = !document.documentElement.classList.contains("dark");
    document.documentElement.classList.toggle("dark", next);
    try {
      localStorage.setItem("theme", next ? "dark" : "light");
    } catch {
      /* storage unavailable */
    }
    setDark(next);
  };
  return [dark, toggle];
}

export function Topbar({ onOpenNav, extra }: { onOpenNav: () => void; extra?: React.ReactNode }) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { data: me } = useMe();
  const [dark, toggleTheme] = useTheme();

  async function logout() {
    // Ends every session of this user server-side and clears the httpOnly
    // session cookie. Best effort: an expired session is already signed out.
    try {
      await api.post("/v1/auth/logout", {});
    } catch {
      /* already expired or offline */
    }
    queryClient.clear();
    router.replace("/login");
  }

  return (
    <header className="topbar">
      <button className="btn btn-ghost btn-sm nav-toggle" onClick={onOpenNav} aria-label="Open navigation">
        <MenuIcon size={18} aria-hidden />
      </button>
      <button type="button" className="topbar-search" onClick={openCommandPalette} aria-label="Search (Ctrl K)">
        <Search size={15} aria-hidden />
        <span className="hide-sm">Search…</span>
        <kbd className="hide-sm">Ctrl K</kbd>
      </button>
      <div style={{ flex: 1 }} />
      <div className="flex items-center gap-1">
        {extra}
        <RunningOps />
        <Menu
          label="Account"
          trigger={
            <>
              <CircleUser size={16} aria-hidden />
              <span className="hide-sm truncate" style={{ maxWidth: "14rem" }}>
                {me?.email ?? "Account"}
              </span>
            </>
          }
          items={[
            { label: "Your profile", icon: <UserRound size={15} />, onSelect: () => router.push("/settings/profile") },
            { label: dark ? "Light theme" : "Dark theme", icon: dark ? <Sun size={15} /> : <Moon size={15} />, onSelect: toggleTheme },
            { label: "Sign out", icon: <LogOut size={15} />, onSelect: () => void logout() },
          ]}
        />
      </div>
    </header>
  );
}
