"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, download } from "../api";
import type {
  Backup,
  CreateDestinationInput,
  Destination,
  Paginated,
  RestoreBackupInput,
  TriggerBackupInput,
  UpdateDestinationInput,
  Schedule,
  CreateScheduleInput,
  UpdateScheduleInput,
} from "../types";
import { pageQS } from "./core";

// ---- Backups ----
export function useBackups(databaseId?: string, page?: number, search?: string) {
  const q = new URLSearchParams();
  if (databaseId) q.set("database_id", databaseId);
  if (search?.trim()) q.set("search", search.trim());
  const qs = q.toString() ? `&${q}` : "";
  return useQuery({
    queryKey: ["backups", databaseId ?? "all", page ?? 1, search?.trim() ?? ""],
    queryFn: () => api.get<Paginated<Backup>>(`/v1/backups?${pageQS(page)}${qs}`),
    refetchInterval: 5_000,
    placeholderData: (prev) => prev,
  });
}

export function useTriggerBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: TriggerBackupInput) => api.post<Backup>("/v1/backups", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["backups"] });
      qc.invalidateQueries({ queryKey: ["operations"] });
    },
  });
}

export function useRestoreBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ backup_id, ...rest }: RestoreBackupInput) =>
      api.post<{ operation_id: string }>(`/v1/backups/${backup_id}/restore`, rest),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["operations"] }),
  });
}

export function useBackupActions() {
  const qc = useQueryClient();
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["backups"] });
    qc.invalidateQueries({ queryKey: ["operations"] });
  };
  const verify = useMutation({
    mutationFn: (id: string) => api.post<{ operation_id: string }>(`/v1/backups/${id}/verify`, {}),
    onSuccess: invalidate,
  });
  const remove = useMutation({ mutationFn: (id: string) => api.del<void>(`/v1/backups/${id}`), onSuccess: invalidate });
  return { verify, remove };
}

/** downloadBackup opens a short-lived presigned link to the backup's .sql.gz. */
export async function downloadBackup(id: string) {
  const { url } = await api.get<{ url: string; expires_at: string }>(`/v1/backups/${id}/download`);
  window.location.assign(url);
}

// ---- Backup destinations ----
export function useDestinations(enabled = true) {
  return useQuery({
    queryKey: ["destinations"],
    queryFn: () => api.get<{ items: Destination[] }>("/v1/backup-destinations"),
    enabled,
  });
}

export function useCreateDestination() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateDestinationInput) =>
      api.post<Destination>("/v1/backup-destinations", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["destinations"] }),
  });
}

export function useUpdateDestination() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: UpdateDestinationInput & { id: string }) =>
      api.patch<Destination>(`/v1/backup-destinations/${id}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["destinations"] }),
  });
}

export function useDeleteDestination() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/backup-destinations/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["destinations"] }),
  });
}

export function useTestDestination() {
  return useMutation({
    mutationFn: (id: string) => api.post<{ ok: boolean }>(`/v1/backup-destinations/${id}/test`, {}),
  });
}

// ---- Backup schedules ----
export function useSchedules(databaseId?: string) {
  const qs = databaseId ? `?database_id=${databaseId}` : "";
  return useQuery({
    queryKey: ["schedules", databaseId ?? "all"],
    queryFn: () => api.get<{ items: Schedule[] }>(`/v1/backup-schedules${qs}`),
  });
}

export function useCreateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateScheduleInput) => api.post<Schedule>("/v1/backup-schedules", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["schedules"] }),
  });
}

export function useUpdateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: UpdateScheduleInput & { id: string }) =>
      api.patch<Schedule>(`/v1/backup-schedules/${id}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["schedules"] }),
  });
}

export function useDeleteSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/backup-schedules/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["schedules"] }),
  });
}
