"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api";
import type { SavedTableView } from "../types";

// Saved data-browser table views: yours and the shared ones.
export function useTableViews(databaseId: string, table: string, enabled = true) {
  return useQuery({
    queryKey: ["table-views", databaseId, table],
    queryFn: () =>
      api
        .get<{ items: SavedTableView[] }>(`/v1/databases/${databaseId}/table-views?table=${encodeURIComponent(table)}`)
        .then((r) => r.items),
    enabled: Boolean(databaseId && table) && enabled,
    staleTime: 30_000,
    retry: false,
  });
}

export type TableViewInput = { table?: string; name?: string; state?: Record<string, unknown>; shared?: boolean };

export function useTableViewMutations(databaseId: string, table: string) {
  const qc = useQueryClient();
  const base = `/v1/databases/${databaseId}/table-views`;
  const invalidate = () => qc.invalidateQueries({ queryKey: ["table-views", databaseId, table] });
  const create = useMutation({
    mutationFn: (in_: TableViewInput) => api.post<SavedTableView>(base, { ...in_, table }),
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: ({ id, ...in_ }: TableViewInput & { id: string }) => api.patch<SavedTableView>(`${base}/${id}`, in_),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del<void>(`${base}/${id}`),
    onSuccess: invalidate,
  });
  return { create, update, remove };
}
