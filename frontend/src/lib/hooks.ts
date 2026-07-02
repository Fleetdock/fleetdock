"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "./api";
import type {
  AgentToken,
  ApiToken,
  Backup,
  CreateDatabaseInput,
  CreateDestinationInput,
  CreateInstanceInput,
  CreateServerInput,
  CreateTokenInput,
  CreatedAgentToken,
  Database,
  Destination,
  ImportDatabasesResult,
  Instance,
  Me,
  Operation,
  Paginated,
  RestoreBackupInput,
  Server,
  TestConnectionResult,
  TriggerBackupInput,
  UpdateDestinationInput,
  ChangePasswordInput,
  CreateUserInput,
  Profile,
  Role,
  RoleInput,
  UpdateProfileInput,
  UpdateUserInput,
  User,
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
    refetchInterval: 15_000,
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

// ---- Instances ----
export function useInstances(serverId?: string, kind?: string) {
  const q = new URLSearchParams();
  if (serverId) q.set("server_id", serverId);
  if (kind) q.set("kind", kind);
  const qs = q.toString();
  return useQuery({
    queryKey: ["instances", serverId ?? "all", kind ?? "all"],
    queryFn: () => api.get<Paginated<Instance>>(`/v1/instances${qs ? `?${qs}` : ""}`),
  });
}

export function useCreateInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateInstanceInput) => api.post<Instance>("/v1/instances", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["instances"] }),
  });
}

export function useDeleteInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/instances/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["instances"] }),
  });
}

export function useTestConnection() {
  return useMutation({
    mutationFn: (id: string) =>
      api.post<TestConnectionResult>(`/v1/instances/${id}/test-connection`, {}),
  });
}

export function useImportDatabases() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<ImportDatabasesResult>(`/v1/instances/${id}/import-databases`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["databases"] });
      qc.invalidateQueries({ queryKey: ["operations"] });
    },
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
    refetchInterval: 10_000,
  });
}

export function useCreateDatabase() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateDatabaseInput) => api.post<Database>("/v1/databases", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["databases"] });
      qc.invalidateQueries({ queryKey: ["operations"] });
    },
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

// ---- Operations ----
export function useOperations(params?: { status?: string; resource_id?: string }) {
  const q = new URLSearchParams();
  if (params?.status) q.set("status", params.status);
  if (params?.resource_id) q.set("resource_id", params.resource_id);
  q.set("limit", "50");
  return useQuery({
    queryKey: ["operations", q.toString()],
    queryFn: () => api.get<Paginated<Operation>>(`/v1/operations?${q.toString()}`),
    refetchInterval: 4_000,
  });
}

// ---- Backups ----
export function useBackups(databaseId?: string) {
  const qs = databaseId ? `&database_id=${databaseId}` : "";
  return useQuery({
    queryKey: ["backups", databaseId ?? "all"],
    queryFn: () => api.get<Paginated<Backup>>(`/v1/backups?limit=50${qs}`),
    refetchInterval: 5_000,
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

// ---- Backup destinations ----
export function useDestinations() {
  return useQuery({
    queryKey: ["destinations"],
    queryFn: () => api.get<{ items: Destination[] }>("/v1/backup-destinations"),
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

// ---- Permission helper ----
// Returns a checker for the current user's permissions. While /auth/me is
// still loading, every check returns false (UI renders read-only until the
// permissions arrive).
export function useCan() {
  const { data: me } = useMe();
  const perms = me?.permissions;
  return (perm: string) => (perms ? perms.includes(perm) : false);
}
