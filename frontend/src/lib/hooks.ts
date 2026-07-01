"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "./api";
import type {
  ApiToken,
  CreateDatabaseInput,
  CreateInstanceInput,
  CreateServerInput,
  CreateTokenInput,
  Database,
  Instance,
  Me,
  Paginated,
  Server,
} from "./types";

export function useMe() {
  return useQuery({ queryKey: ["me"], queryFn: () => api.get<Me>("/v1/auth/me") });
}

// ---- Servers ----
export function useServers(search?: string) {
  const qs = search ? `?search=${encodeURIComponent(search)}` : "";
  return useQuery({
    queryKey: ["servers", search ?? ""],
    queryFn: () => api.get<Paginated<Server>>(`/v1/servers${qs}`),
  });
}

export function useServer(id: string) {
  return useQuery({
    queryKey: ["server", id],
    queryFn: () => api.get<Server>(`/v1/servers/${id}`),
    enabled: Boolean(id),
  });
}

export function useCreateServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateServerInput) => api.post<Server>("/v1/servers", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["servers"] }),
  });
}

// ---- Instances ----
export function useInstances(serverId?: string) {
  const qs = serverId ? `?server_id=${serverId}` : "";
  return useQuery({
    queryKey: ["instances", serverId ?? "all"],
    queryFn: () => api.get<Paginated<Instance>>(`/v1/instances${qs}`),
  });
}

export function useCreateInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateInstanceInput) => api.post<Instance>("/v1/instances", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["instances"] }),
  });
}

// ---- Databases ----
export function useDatabases(params?: { instance_id?: string; search?: string }) {
  const q = new URLSearchParams();
  if (params?.instance_id) q.set("instance_id", params.instance_id);
  if (params?.search) q.set("search", params.search);
  const qs = q.toString();
  return useQuery({
    queryKey: ["databases", qs],
    queryFn: () => api.get<Paginated<Database>>(`/v1/databases${qs ? `?${qs}` : ""}`),
  });
}

export function useCreateDatabase() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateDatabaseInput) => api.post<Database>("/v1/databases", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["databases"] }),
  });
}

export function useLockDatabase() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<Database>(`/v1/databases/${id}/lock`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["databases"] }),
  });
}

export function useUnlockDatabase() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<Database>(`/v1/databases/${id}/unlock`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["databases"] }),
  });
}

export function useDeleteDatabase() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/databases/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["databases"] }),
  });
}

// ---- API tokens ----
export function useTokens() {
  return useQuery({
    queryKey: ["tokens"],
    queryFn: () => api.get<{ items: ApiToken[] }>("/v1/tokens"),
  });
}

export function useCreateToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTokenInput) =>
      api.post<ApiToken & { token: string }>("/v1/tokens", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tokens"] }),
  });
}

export function useRevokeToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/tokens/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tokens"] }),
  });
}
