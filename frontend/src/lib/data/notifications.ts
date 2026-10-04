"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api";
import type {
  NotificationChannel,
  ChannelInput,
  AlertRule,
  RuleInput,
} from "../types";

// ---- Notification channels ----
export function useChannels() {
  return useQuery({
    queryKey: ["channels"],
    queryFn: () => api.get<{ items: NotificationChannel[] }>("/v1/notification-channels"),
  });
}

export function useCreateChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ChannelInput) => api.post<NotificationChannel>("/v1/notification-channels", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["channels"] }),
  });
}

export function useUpdateChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: ChannelInput & { id: string }) =>
      api.patch<NotificationChannel>(`/v1/notification-channels/${id}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["channels"] }),
  });
}

export function useDeleteChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/notification-channels/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["channels"] }),
  });
}

export function useTestChannel() {
  return useMutation({
    mutationFn: (id: string) => api.post<{ ok: boolean }>(`/v1/notification-channels/${id}/test`, {}),
  });
}

// ---- Alert rules ----
export function useAlertRules() {
  return useQuery({
    queryKey: ["alert-rules"],
    queryFn: () => api.get<{ items: AlertRule[] }>("/v1/alert-rules"),
  });
}

export function useCreateAlertRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: RuleInput) => api.post<AlertRule>("/v1/alert-rules", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alert-rules"] }),
  });
}

export function useUpdateAlertRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: RuleInput & { id: string }) =>
      api.patch<AlertRule>(`/v1/alert-rules/${id}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alert-rules"] }),
  });
}

export function useDeleteAlertRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/alert-rules/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alert-rules"] }),
  });
}
