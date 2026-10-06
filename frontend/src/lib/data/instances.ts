"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api";
import type {
  Process,
  Setting,
  CreateInstanceInput,
  ProvisionInstanceInput,
  Instance,
  InstanceHealth,
  Paginated,
  UpdateInstanceInput,
} from "../types";
import { pageQS } from "./core";

// ---- Instances ----
export function useInstances(serverId?: string, kind?: string, page?: number, enabled = true) {
  const q = new URLSearchParams();
  if (serverId) q.set("server_id", serverId);
  if (kind) q.set("kind", kind);
  // page undefined = fetch a large first page (dropdown consumers)
  const paging = page === undefined ? "limit=100" : pageQS(page);
  const qs = q.toString();
  return useQuery({
    queryKey: ["instances", serverId ?? "all", kind ?? "all", page ?? "all"],
    queryFn: () => api.get<Paginated<Instance>>(`/v1/instances?${qs ? `${qs}&` : ""}${paging}`),
    enabled,
    placeholderData: (prev) => prev,
  });
}

export function useCreateInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateInstanceInput) => api.post<Instance>("/v1/instances", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["instances"] }),
  });
}

export function useUpdateInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: UpdateInstanceInput & { id: string }) =>
      api.patch<Instance>(`/v1/instances/${id}`, input),
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ["instance", v.id] });
      qc.invalidateQueries({ queryKey: ["instances"] });
      // Credentials gate live admin (db users, grants, table browsing), so a
      // credential change has to invalidate those reads too.
      qc.invalidateQueries({ queryKey: ["db-users"] });
      qc.invalidateQueries({ queryKey: ["databases"] });
    },
  });
}

export function useDeleteInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, removeVolume }: { id: string; removeVolume?: boolean }) =>
      api.del<void>(`/v1/instances/${id}${removeVolume ? "?remove_volume=true" : ""}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["instances"] });
      qc.invalidateQueries({ queryKey: ["operations"] });
    },
  });
}

export function useProvisionInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ProvisionInstanceInput) =>
      api.post<{ instance: Instance; operation_id: string }>("/v1/instances/provision", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["instances"] });
      qc.invalidateQueries({ queryKey: ["operations"] });
    },
  });
}

export function useInstanceLifecycle() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: "start" | "stop" | "restart" }) =>
      api.post<{ operation_id: string }>(`/v1/instances/${id}/${action}`, {}),
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ["instance", v.id] });
      qc.invalidateQueries({ queryKey: ["instances"] });
      qc.invalidateQueries({ queryKey: ["operations"] });
    },
  });
}

/** useProbeInstance checks a database server now and syncs its database list. */
export function useProbeInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<InstanceHealth>(`/v1/instances/${id}/probe`, {}),
    onSuccess: (_d, id) => {
      qc.invalidateQueries({ queryKey: ["instance", id] });
      qc.invalidateQueries({ queryKey: ["instances"] });
      qc.invalidateQueries({ queryKey: ["databases"] });
    },
  });
}

// ---- Instance monitoring ----
export function useProcesses(instanceId: string, enabled = true) {
  return useQuery({
    queryKey: ["processes", instanceId],
    queryFn: () => api.get<{ items: Process[] }>(`/v1/instances/${instanceId}/processes`).then((r) => r.items),
    enabled: Boolean(instanceId) && enabled,
    refetchInterval: 5000,
  });
}

export function useKillProcess() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ instanceId, pid, connection }: { instanceId: string; pid: number; connection: boolean }) =>
      api.post<void>(`/v1/instances/${instanceId}/processes/${pid}/kill`, { connection }),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["processes", v.instanceId] }),
  });
}

export function useServerStatus(instanceId: string, enabled = true) {
  return useQuery({
    queryKey: ["server-status", instanceId],
    queryFn: () => api.get<{ items: Setting[] }>(`/v1/instances/${instanceId}/status`).then((r) => r.items),
    enabled: Boolean(instanceId) && enabled,
    refetchInterval: 15000,
  });
}

export function useServerVariables(instanceId: string, enabled = true) {
  return useQuery({
    queryKey: ["server-variables", instanceId],
    queryFn: () => api.get<{ items: Setting[] }>(`/v1/instances/${instanceId}/variables`).then((r) => r.items),
    enabled: Boolean(instanceId) && enabled,
  });
}
