"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api";
import type { BackupCopy, CopyDiff, DiffCursor, DiffItem, RestoreRowResult } from "../types";

// Backup copies: a backup restored next to its database so a table can be
// compared with it and rows put back.

const base = (databaseId: string) => `/v1/databases/${databaseId}/backup-copies`;

export function useBackupCopies(databaseId: string, enabled = true) {
  return useQuery({
    queryKey: ["backup-copies", databaseId],
    queryFn: () => api.get<{ items: BackupCopy[] }>(base(databaseId)).then((r) => r.items),
    enabled: Boolean(databaseId) && enabled,
    // While a copy restores or drops, follow it.
    refetchInterval: (q) =>
      (q.state.data ?? []).some((c) => c.status === "restoring" || c.status === "dropping") ? 3000 : false,
    retry: false,
  });
}

export function useBackupCopyMutations(databaseId: string) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["backup-copies", databaseId] });
  return {
    create: useMutation({
      mutationFn: (backupId: string) => api.post<BackupCopy>(base(databaseId), { backup_id: backupId }),
      onSettled: invalidate,
    }),
    extend: useMutation({
      mutationFn: (copyId: string) => api.post<BackupCopy>(`${base(databaseId)}/${copyId}/extend`, {}),
      onSettled: invalidate,
    }),
    discard: useMutation({
      mutationFn: (copyId: string) => api.del<void>(`${base(databaseId)}/${copyId}`),
      onSettled: invalidate,
    }),
    restoreRows: useMutation({
      mutationFn: (input: { copyId: string; table: string; rows: DiffItem[] }) =>
        api.post<{ items: RestoreRowResult[]; batch_id: string }>(`${base(databaseId)}/${input.copyId}/restore-rows`, {
          table: input.table,
          rows: input.rows.map((r) => ({ key: r.key, kind: r.kind, live_hash: r.live_hash ?? "" })),
        }),
      onSettled: () =>
        Promise.all([
          qc.invalidateQueries({ queryKey: ["browse-rows", databaseId] }),
          qc.invalidateQueries({ queryKey: ["row-changes", databaseId] }),
          qc.invalidateQueries({ queryKey: ["tables", databaseId] }),
        ]),
    }),
  };
}

/** diffCopy runs one step of comparing a table with a backup copy. */
export function diffCopy(databaseId: string, copyId: string, table: string, cursor?: DiffCursor) {
  return api.post<CopyDiff>(`${base(databaseId)}/${copyId}/diff`, { table, cursor });
}
