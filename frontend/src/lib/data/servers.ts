"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api";
import type {
  AgentToken,
  CreateServerInput,
  CreatedAgentToken,
  Paginated,
  Server,
  UpdateServerInput,
  MetricSample,
} from "../types";
import { pageQS } from "./core";

// ---- Servers ----

export function useServers(search?: string, page?: number, enabled = true) {
  const q = new URLSearchParams();
  if (search) q.set("search", search);
  // page undefined = fetch a large first page (dropdown consumers)
  const paging = page === undefined ? "limit=100" : pageQS(page);
  return useQuery({
    queryKey: ["servers", search ?? "", page ?? "all"],
    queryFn: () => api.get<Paginated<Server>>(`/v1/servers?${q.toString()}&${paging}`),
    enabled,
    refetchInterval: 15_000,
    placeholderData: (prev) => prev,
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

export function useUpdateServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: UpdateServerInput & { id: string }) =>
      api.patch<Server>(`/v1/servers/${id}`, input),
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ["servers"] });
      qc.invalidateQueries({ queryKey: ["server", v.id] });
    },
  });
}

export function useDeleteServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/servers/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["servers"] }),
  });
}

// ---- Agent registration tokens (connect server flow) ----
export function useAgentTokens() {
  return useQuery({
    queryKey: ["agent-tokens"],
    queryFn: () => api.get<{ items: AgentToken[] }>("/v1/agent-tokens"),
  });
}

export function useCreateAgentToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name?: string; ttl_hours?: number }) =>
      api.post<CreatedAgentToken>("/v1/agent-tokens", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["agent-tokens"] }),
  });
}

// ---- Server metrics history ----
export function useServerMetrics(serverId: string, hours = 6) {
  return useQuery({
    queryKey: ["server-metrics", serverId, hours],
    queryFn: () => api.get<{ items: MetricSample[] }>(`/v1/servers/${serverId}/metrics?hours=${hours}`),
    enabled: Boolean(serverId),
    refetchInterval: 30_000,
  });
}
