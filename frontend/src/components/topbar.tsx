"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { useQueryClient } from "@tanstack/react-query";
import {
  ChevronRight,
  LogOut,
  Menu as MenuIcon,
  Search,
  UserRound,
} from "lucide-react";

import { openCommandPalette } from "@/components/command-palette";
import { ThemeToggle } from "@/components/theme-toggle";
import { RunningOps } from "@/components/running-ops";
import { useActiveSection } from "@/components/sidebar";
import { IconButton, Menu } from "@/components/ui";
import { api } from "@/lib/api";
import { useMe } from "@/lib/hooks";


// useIsMac picks the shortcut label shown on the search button.
function useIsMac(): boolean {
  const [mac, setMac] = useState(false);
  useEffect(() => {
    setMac(/Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent));
  }, []);
  return mac;
}

/**
 * TopbarCrumbs shows where you are: the section, then the page within it
 * when the section has several pages. Detail pages carry their own, deeper
 * breadcrumbs in the page header.
 */
function TopbarCrumbs() {
  const pathname = usePathname();
  const section = useActiveSection();
  if (!section) return null;
  const page =
    section.pages.length > 1
      ? // The most specific page: /activity/audit is "Audit log", not "Tasks".
        (section.pages.find((p) => pathname === p.href) ??
        [...section.pages]
          .sort((a, b) => b.href.length - a.href.length)
          .find((p) => pathname.startsWith(`${p.href}/`)))
      : undefined;
  const sectionHref = section.pages.some((p) => p.href === section.href)
    ? section.href
    : section.pages[0].href;
  return (
    <nav aria-label="Section" className="breadcrumbs topbar-crumbs">
      <ol>
        <li>
          <section.Icon size={15} aria-hidden className="text-muted" />
          {page ? (
            <Link href={sectionHref}>{section.label}</Link>
          ) : (
            <span aria-current="page">{section.label}</span>
          )}
          {page ? <ChevronRight size={13} aria-hidden /> : null}
        </li>
        {page ? (
          <li>
            <span aria-current="page">{page.label}</span>
          </li>
        ) : null}
      </ol>
    </nav>
  );
}

function initials(email?: string): string {
  if (!email) return "?";
  const name = email.split("@")[0];
  const parts = name.split(/[._-]+/).filter(Boolean);
  return (
    ((parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "")).toUpperCase() ||
    name.slice(0, 2).toUpperCase()
  );
}

export function Topbar({
  onOpenNav,
  extra,
}: {
  onOpenNav: () => void;
  extra?: React.ReactNode;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { data: me } = useMe();
  const mac = useIsMac();

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
      <IconButton
        label="Open navigation"
        className="nav-toggle"
        size="md"
        onClick={onOpenNav}
        noTooltip
      >
        <MenuIcon size={18} aria-hidden />
      </IconButton>
      <TopbarCrumbs />
      <div className="topbar-actions">
        <button
          type="button"
          className="topbar-search"
          onClick={openCommandPalette}
          aria-label={`Search (${mac ? "⌘" : "Ctrl"} K)`}
        >
          <Search size={15} aria-hidden />
          <span className="hide-sm">Search or jump to…</span>
          <kbd className="hide-sm">{mac ? "⌘ K" : "Ctrl K"}</kbd>
        </button>
        {extra}
        <RunningOps />
        <ThemeToggle />
        <Menu
          label="Account"
          triggerClassName="topbar-account"
          trigger={
            <>
              <span className="topbar-avatar" aria-hidden>
                {initials(me?.email)}
              </span>
              <span className="max-w-56 truncate hide-sm">
                {me?.email ?? "Account"}
              </span>
            </>
          }
          items={[
            {
              label: "Your profile",
              icon: <UserRound size={15} />,
              onSelect: () => router.push("/settings/profile"),
            },
            {
              label: "Sign out",
              icon: <LogOut size={15} />,
              onSelect: () => void logout(),
            },
          ]}
        />
      </div>
    </header>
  );
}
