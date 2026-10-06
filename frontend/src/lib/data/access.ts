"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api";
import type {
  ApiToken,
  CreateTokenInput,
  ChangePasswordInput,
  CreateUserInput,
  Profile,
  Role,
  RoleInput,
  UpdateProfileInput,
  UpdateUserInput,
  User,
  RoleGrant,
  AddGrantInput,
} from "../types";

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

// ---- Users & roles (administration) ----
export function useUsers() {
  return useQuery({
    queryKey: ["users"],
    queryFn: () => api.get<{ items: User[] }>("/v1/users"),
  });
}

export function useRoles() {
  return useQuery({
    queryKey: ["roles"],
    queryFn: () => api.get<{ items: Role[] }>("/v1/roles"),
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateUserInput) => api.post<User>("/v1/users", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: UpdateUserInput & { id: string }) =>
      api.patch<User>(`/v1/users/${id}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/users/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
}

export function useResetUserPassword() {
  return useMutation({
    mutationFn: ({ id, password }: { id: string; password: string }) =>
      api.post<{ status: string }>(`/v1/users/${id}/password`, { password }),
  });
}

// ---- Profile (self-service) ----
export function useProfile() {
  return useQuery({
    queryKey: ["profile"],
    queryFn: () => api.get<Profile>("/v1/profile"),
  });
}

export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateProfileInput) => api.patch<Profile>("/v1/profile", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["profile"] });
      qc.invalidateQueries({ queryKey: ["me"] });
    },
  });
}

export function useChangePassword() {
  return useMutation({
    mutationFn: (input: ChangePasswordInput) =>
      api.post<{ status: string }>("/v1/profile/password", input),
  });
}

// ---- Role management ----
export function usePermissions() {
  return useQuery({
    queryKey: ["permissions"],
    queryFn: () => api.get<{ items: string[] }>("/v1/permissions"),
  });
}

export function useCreateRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: RoleInput) => api.post<Role>("/v1/roles", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["roles"] }),
  });
}

export function useUpdateRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: RoleInput & { id: string }) =>
      api.patch<Role>(`/v1/roles/${id}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["roles"] }),
  });
}

export function useDeleteRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/roles/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["roles"] }),
  });
}

// ---- Scoped role grants (user administration) ----
export function useRoleGrants(userId: string) {
  return useQuery({
    queryKey: ["role-grants", userId],
    queryFn: () => api.get<{ items: RoleGrant[] }>(`/v1/users/${userId}/role-grants`),
    enabled: Boolean(userId),
  });
}

export function useAddGrant() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, ...input }: { userId: string } & AddGrantInput) =>
      api.post<RoleGrant>(`/v1/users/${userId}/role-grants`, input),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["role-grants", v.userId] }),
  });
}

export function useRemoveGrant() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, grantId }: { userId: string; grantId: string }) =>
      api.del<void>(`/v1/users/${userId}/role-grants/${grantId}`),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["role-grants", v.userId] }),
  });
}
