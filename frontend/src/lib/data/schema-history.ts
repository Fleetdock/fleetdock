"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api";
import type { SchemaComparison, SchemaHistory, SchemaSnapshot, SchemaSnapshotDetail, SchemaWatch } from "../types";

// Schema history: snapshots of a database's structure over time, and
// comparisons of two structures with the script that aligns them.

const base = (databaseId: string) => `/v1/databases/${databaseId}/schema`;

export function useSchemaHistory(databaseId: string) {
  return useQuery({
    queryKey: ["schema-history", databaseId],
    queryFn: () => api.get<SchemaHistory>(`${base(databaseId)}/snapshots`),
    enabled: Boolean(databaseId),
    retry: false,
  });
}

export function useSchemaSnapshot(databaseId: string, snapshotId: string | null) {
  return useQuery({
    queryKey: ["schema-snapshot", databaseId, snapshotId],
    queryFn: () => api.get<SchemaSnapshotDetail>(`${base(databaseId)}/snapshots/${snapshotId}`),
    enabled: Boolean(databaseId && snapshotId),
    staleTime: Infinity, // snapshots never change
  });
}

export function useSchemaHistoryMutations(databaseId: string) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["schema-history", databaseId] });
  return {
    setWatch: useMutation({
      mutationFn: (enabled: boolean) => api.put<SchemaWatch>(`${base(databaseId)}/watch`, { enabled }),
      onSettled: invalidate,
    }),
    snapshot: useMutation({
      mutationFn: () => api.post<{ snapshot: SchemaSnapshot; created: boolean }>(`${base(databaseId)}/snapshots`, {}),
      onSettled: invalidate,
    }),
  };
}

export interface CompareRequest {
  source_database_id: string;
  source_snapshot_id?: string;
  target_database_id: string;
}

export function useCompareSchemas() {
  return useMutation({
    mutationFn: (req: CompareRequest) => api.post<SchemaComparison>("/v1/schema/compare", req),
  });
}
