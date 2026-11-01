"use client";

import { useQuery } from "@tanstack/react-query";

import { api, download } from "../api";
import type { AuditPage } from "../types";

/**
 * useAuditEvents loads one page of the audit log. query is the filter
 * string (lib/audit.ts auditQuery); cursor continues from a previous page.
 */
export function useAuditEvents(query: string, cursor: string, enabled = true) {
  return useQuery({
    queryKey: ["audit-events", query, cursor],
    queryFn: () => {
      const q = new URLSearchParams(query);
      if (cursor) q.set("cursor", cursor);
      q.set("limit", "50");
      return api.get<AuditPage>(`/v1/audit-events?${q.toString()}`);
    },
    enabled,
    placeholderData: (prev) => prev,
    staleTime: 15_000,
  });
}

/** downloadAuditCSV saves every event matching query (up to 10000) as CSV. */
export function downloadAuditCSV(query: string) {
  const q = new URLSearchParams(query);
  q.set("format", "csv");
  return download(`/v1/audit-events?${q.toString()}`);
}
