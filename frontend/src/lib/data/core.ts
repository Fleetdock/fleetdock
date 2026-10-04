"use client";

import { useState } from "react";

import { useQuery } from "@tanstack/react-query";

import { api } from "../api";
import type {
  Me,
} from "../types";

export function useMe() {
  return useQuery({ queryKey: ["me"], queryFn: () => api.get<Me>("/v1/auth/me") });
}


// ---- Paging ----
export const LIST_PAGE_SIZE = 20;

export function pageQS(page?: number) {
  const p = Math.max(1, page ?? 1);
  return `limit=${LIST_PAGE_SIZE}&offset=${(p - 1) * LIST_PAGE_SIZE}`;
}

// ---- Permission helper ----
// Returns a checker for the current user's permissions. While /auth/me is
// still loading, every check returns false (UI renders read-only until the
// permissions arrive).
export function useCan() {
  const { data: me } = useMe();
  const perms = me?.permissions;
  return (perm: string) => (perms ? perms.includes(perm) : false);
}

// useCanAny returns a checker that is true if the current user holds a
// permission at ANY scope (global or scoped). Used to reveal nav sections and
// list pages for scoped users.
export function useCanAny() {
  const { data: me } = useMe();
  const grants = me?.grants;
  return (perm: string) => (grants ? grants.some((g) => g.permission === perm) : false);
}

// useCanOn returns a resource-scoped checker (defense-in-depth; the server is
// authoritative). Pass the ids the caller knows for the resource — a global
// grant always allows, a server grant matches serverId, a database grant
// matches databaseId.
export function useCanOn() {
  const { data: me } = useMe();
  const grants = me?.grants;
  return (perm: string, res: { serverId?: string | null; databaseId?: string | null }) => {
    if (!grants) return false;
    return grants.some((g) => {
      if (g.permission !== perm) return false;
      switch (g.scope_type) {
        case "global":
          return true;
        case "server":
          return Boolean(res.serverId) && g.scope_id === res.serverId;
        case "database":
          return Boolean(res.databaseId) && g.scope_id === res.databaseId;
        default:
          return false;
      }
    });
  };
}

// ---- Client-side pagination helper (for endpoints returning full lists) ----
export function useClientPage<T>(items: T[] | undefined, pageSize = LIST_PAGE_SIZE) {
  const [page, setPage] = useState(1);
  const all = items ?? [];
  const pageCount = Math.max(1, Math.ceil(all.length / pageSize));
  const current = Math.min(page, pageCount);
  return {
    page: current,
    setPage,
    pageCount,
    items: all.slice((current - 1) * pageSize, current * pageSize),
    total: all.length,
  };
}
