"use client";

import { useEffect, useRef } from "react";

import { useQuery } from "@tanstack/react-query";

import { useToast } from "@/components/toast";
import { api } from "../api";
import { operationTitle } from "../operations";
import type {
  Operation,
  OperationLog,
  Paginated,
} from "../types";
import { pageQS, useCanAny } from "./core";

// ---- Operations ----
export const TERMINAL_OP_STATUS = new Set(["succeeded", "failed", "canceled"]);

export function useOperations(params?: { status?: string; resource_id?: string; page?: number; enabled?: boolean }) {
  const q = new URLSearchParams();
  if (params?.status) q.set("status", params.status);
  if (params?.resource_id) q.set("resource_id", params.resource_id);
  return useQuery({
    queryKey: ["operations", q.toString(), params?.page ?? 1],
    queryFn: () => api.get<Paginated<Operation>>(`/v1/operations?${q.toString()}&${pageQS(params?.page)}`),
    enabled: params?.enabled ?? true,
    refetchInterval: 4_000,
    placeholderData: (prev) => prev,
  });
}

export function useOperation(id: string) {
  return useQuery({
    queryKey: ["operation", id],
    queryFn: () => api.get<Operation>(`/v1/operations/${id}`),
    enabled: Boolean(id),
    // Poll while the operation is still in flight; stop once it's terminal.
    refetchInterval: (query) =>
      query.state.data && TERMINAL_OP_STATUS.has(query.state.data.status) ? false : 4_000,
  });
}

export function useOperationLogs(id: string, running: boolean) {
  return useQuery({
    queryKey: ["operation-logs", id],
    queryFn: () => api.get<{ items: OperationLog[] }>(`/v1/operations/${id}/logs?limit=2000`),
    enabled: Boolean(id),
    // Tail while running; when the op turns terminal `running` flips false and
    // React Query does one final fetch, then stops.
    refetchInterval: running ? 2_000 : false,
  });
}

// ---- Operation-completion toasts ----
// useOperationToasts watches the polled operations list and fires a toast when
// an operation transitions to a terminal state, so users don't have to refresh.
export function useOperationToasts() {
  const { push } = useToast();
  const canAny = useCanAny();
  const { data } = useOperations({ page: 1, enabled: canAny("operation:read") });
  const seen = useRef<Map<string, string>>(new Map());
  const bootstrapped = useRef(false);

  useEffect(() => {
    const items = data?.items ?? [];
    if (!bootstrapped.current) {
      for (const op of items) seen.current.set(op.id, op.status);
      bootstrapped.current = true;
      return;
    }
    for (const op of items) {
      const prev = seen.current.get(op.id);
      if (prev && prev !== op.status && TERMINAL_OP_STATUS.has(op.status)) {
        const title = operationTitle(op);
        const view = { label: "View", href: `/activity/${op.id}` };
        if (op.status === "succeeded") push("success", `${title} finished`, view);
        else if (op.status === "failed") push("error", `${title} failed`, view);
        else push("info", `${title} ${op.status}`, view);
      }
      seen.current.set(op.id, op.status);
    }
  }, [data, push]);
}
