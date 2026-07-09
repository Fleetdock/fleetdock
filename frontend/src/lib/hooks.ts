"use client";

import { useEffect, useRef, useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useToast } from "@/components/toast";
import { api, download } from "./api";
import type {
  AgentToken,
  ApiToken,
  Backup,
  CreateDatabaseInput,
  CreateDestinationInput,
  CreateInstanceInput,
  ProvisionInstanceInput,
  CreateServerInput,
  CreateTokenInput,
  CreatedAgentToken,
  Database,
  Destination,
  ImportDatabasesResult,
  Instance,
  Me,
  Operation,
  OperationLog,
  Paginated,
  RestoreBackupInput,
  StartMoveInput,
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
  CreateDBUserInput,
  DBUser,
  GrantInput,
  RowsPage,
  SchemaGrant,
  TableInfo,
  TableSchema,
  QueryResult,
  RunQueryInput,
  Overview,
  Schedule,
  CreateScheduleInput,
  UpdateScheduleInput,
  AuditEntry,
  NotificationChannel,
  ChannelInput,
  AlertRule,
  RuleInput,
  MetricSample,
  RoleGrant,
  AddGrantInput,
} from "./types";

export function useMe() {
  return useQuery({ queryKey: ["me"], queryFn: () => api.get<Me>("/v1/auth/me") });
}

// ---- Servers ----
export const LIST_PAGE_SIZE = 20;

function pageQS(page?: number) {
  const p = Math.max(1, page ?? 1);
  return `limit=${LIST_PAGE_SIZE}&offset=${(p - 1) * LIST_PAGE_SIZE}`;
}

export function useServers(search?: string, page?: number) {
  const q = new URLSearchParams();
  if (search) q.set("search", search);
  // page undefined = fetch a large first page (dropdown consumers)
  const paging = page === undefined ? "limit=100" : pageQS(page);
  return useQuery({
    queryKey: ["servers", search ?? "", page ?? "all"],
    queryFn: () => api.get<Paginated<Server>>(`/v1/servers?${q.toString()}&${paging}`),
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
export function useInstances(serverId?: string, kind?: string, page?: number) {
  const q = new URLSearchParams();
  if (serverId) q.set("server_id", serverId);
  if (kind) q.set("kind", kind);
  // page undefined = fetch a large first page (dropdown consumers)
  const paging = page === undefined ? "limit=100" : pageQS(page);
  const qs = q.toString();
  return useQuery({
    queryKey: ["instances", serverId ?? "all", kind ?? "all", page ?? "all"],
    queryFn: () => api.get<Paginated<Instance>>(`/v1/instances?${qs ? `${qs}&` : ""}${paging}`),
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
export function useDatabases(params?: { instance_id?: string; search?: string; page?: number }) {
  const q = new URLSearchParams();
  if (params?.instance_id) q.set("instance_id", params.instance_id);
  if (params?.search) q.set("search", params.search);
  const paging = params?.page === undefined ? "limit=100" : pageQS(params.page);
  const qs = q.toString();
  return useQuery({
    queryKey: ["databases", qs, params?.page ?? "all"],
    queryFn: () => api.get<Paginated<Database>>(`/v1/databases?${qs ? `${qs}&` : ""}${paging}`),
    refetchInterval: 10_000,
    placeholderData: (prev) => prev,
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
    mutationFn: ({ id, drop }: { id: string; drop?: boolean }) =>
      api.del<void>(`/v1/databases/${id}${drop ? "?drop=true" : ""}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["databases"] }),
  });
}

// ---- Operations ----
const TERMINAL_OP_STATUS = new Set(["succeeded", "failed", "canceled"]);

export function useOperations(params?: { status?: string; resource_id?: string; page?: number }) {
  const q = new URLSearchParams();
  if (params?.status) q.set("status", params.status);
  if (params?.resource_id) q.set("resource_id", params.resource_id);
  return useQuery({
    queryKey: ["operations", q.toString(), params?.page ?? 1],
    queryFn: () => api.get<Paginated<Operation>>(`/v1/operations?${q.toString()}&${pageQS(params?.page)}`),
    refetchInterval: 4_000,
    placeholderData: (prev) => prev,
  });
}

export function useOperation(id: string) {
  return useQuery({
    queryKey: ["operation", id],
    queryFn: () => api.get<Operation>(`/v1/operations/${id}`),
    enabled: Boolean(id),
    // Poll while the operation is still in flight; stop once it's terminal.
    refetchInterval: (query) =>
      query.state.data && TERMINAL_OP_STATUS.has(query.state.data.status) ? false : 4_000,
  });
}

export function useOperationLogs(id: string, running: boolean) {
  return useQuery({
    queryKey: ["operation-logs", id],
    queryFn: () => api.get<{ items: OperationLog[] }>(`/v1/operations/${id}/logs?limit=2000`),
    enabled: Boolean(id),
    // Tail while running; when the op turns terminal `running` flips false and
    // React Query does one final fetch, then stops.
    refetchInterval: running ? 2_000 : false,
  });
}

// ---- Backups ----
export function useBackups(databaseId?: string, page?: number) {
  const qs = databaseId ? `&database_id=${databaseId}` : "";
  return useQuery({
    queryKey: ["backups", databaseId ?? "all", page ?? 1],
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

// ---- Move database ----
// A move has no resource of its own: it kicks off a backup and (on completion) a
// restore, both tracked as operations. Starting one returns the backup operation.
export function useStartMove() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: StartMoveInput) =>
      api.post<{ operation_id: string; status: string }>("/v1/moves", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["operations"] });
      qc.invalidateQueries({ queryKey: ["backups"] });
    },
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

// useCanAny returns a checker that is true if the current user holds a
// permission at ANY scope (global or scoped). Used to reveal nav sections and
// list pages for scoped users.
export function useCanAny() {
  const { data: me } = useMe();
  const grants = me?.grants;
  return (perm: string) => (grants ? grants.some((g) => g.permission === perm) : false);
}

// useCanOn returns a resource-scoped checker (defense-in-depth; the server is
// authoritative). Pass the ids the caller knows for the resource — a global
// grant always allows, a server grant matches serverId, a database grant
// matches databaseId.
export function useCanOn() {
  const { data: me } = useMe();
  const grants = me?.grants;
  return (perm: string, res: { serverId?: string | null; databaseId?: string | null }) => {
    if (!grants) return false;
    return grants.some((g) => {
      if (g.permission !== perm) return false;
      switch (g.scope_type) {
        case "global":
          return true;
        case "server":
          return Boolean(res.serverId) && g.scope_id === res.serverId;
        case "database":
          return Boolean(res.databaseId) && g.scope_id === res.databaseId;
        default:
          return false;
      }
    });
  };
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

// ---- Operation-completion toasts ----
// useOperationToasts watches the polled operations list and fires a toast when
// an operation transitions to a terminal state, so users don't have to refresh.
export function useOperationToasts() {
  const { push } = useToast();
  const { data } = useOperations({ page: 1 });
  const seen = useRef<Map<string, string>>(new Map());
  const bootstrapped = useRef(false);

  useEffect(() => {
    const items = data?.items ?? [];
    if (!bootstrapped.current) {
      for (const op of items) seen.current.set(op.id, op.status);
      bootstrapped.current = true;
      return;
    }
    for (const op of items) {
      const prev = seen.current.get(op.id);
      if (prev && prev !== op.status && TERMINAL_OP_STATUS.has(op.status)) {
        const label = op.type.replace(/_/g, " ");
        if (op.status === "succeeded") push("success", `${label} completed`);
        else if (op.status === "failed") push("error", `${label} failed`);
        else push("info", `${label} ${op.status}`);
      }
      seen.current.set(op.id, op.status);
    }
  }, [data, push]);
}

// ---- Live DB administration ----
export function useDBUsers(instanceId: string) {
  return useQuery({
    queryKey: ["db-users", instanceId],
    queryFn: () => api.get<{ items: DBUser[] }>(`/v1/instances/${instanceId}/db-users`),
    enabled: Boolean(instanceId),
    retry: false,
  });
}

export function useCreateDBUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ instance_id, ...input }: CreateDBUserInput) =>
      api.post<{ status: string }>(`/v1/instances/${instance_id}/db-users`, input),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["db-users", v.instance_id] }),
  });
}

export function useDropDBUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ instanceId, ...input }: { instanceId: string } & GrantInput) =>
      api.post<{ status: string }>(`/v1/instances/${instanceId}/db-users/drop`, {
        username: input.username,
        host: input.host,
      }),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["db-users", v.instanceId] }),
  });
}

export function useUserGrants(instanceId: string, username: string, host: string) {
  const q = new URLSearchParams({ username, host });
  return useQuery({
    queryKey: ["user-grants", instanceId, username, host],
    queryFn: () =>
      api.get<{ items: string[] }>(`/v1/instances/${instanceId}/db-users/grants?${q.toString()}`),
    enabled: Boolean(instanceId && username),
    retry: false,
  });
}

export function useGrantOnInstance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ instanceId, ...input }: { instanceId: string } & GrantInput) =>
      api.post<{ status: string }>(`/v1/instances/${instanceId}/grants`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["user-grants"] });
      qc.invalidateQueries({ queryKey: ["schema-grants"] });
    },
  });
}

export function useDBPrivileges() {
  return useQuery({
    queryKey: ["db-privileges"],
    queryFn: () => api.get<{ items: string[] }>("/v1/db-privileges"),
    staleTime: Infinity,
  });
}

export function useSchemaGrants(databaseId: string) {
  return useQuery({
    queryKey: ["schema-grants", databaseId],
    queryFn: () => api.get<{ items: SchemaGrant[] }>(`/v1/databases/${databaseId}/grants`),
    enabled: Boolean(databaseId),
    retry: false,
  });
}

export function useGrantOnDatabase() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ databaseId, ...input }: { databaseId: string } & GrantInput) =>
      api.post<{ status: string }>(`/v1/databases/${databaseId}/grants`, input),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["schema-grants", v.databaseId] }),
  });
}

export function useRevokeOnDatabase() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ databaseId, ...input }: { databaseId: string } & GrantInput) =>
      api.post<{ status: string }>(`/v1/databases/${databaseId}/grants/revoke`, {
        username: input.username,
        host: input.host,
      }),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["schema-grants", v.databaseId] }),
  });
}

export function useDatabaseDBUsers(databaseId: string) {
  return useQuery({
    queryKey: ["database-db-users", databaseId],
    queryFn: () => api.get<{ items: DBUser[] }>(`/v1/databases/${databaseId}/db-users`),
    enabled: Boolean(databaseId),
    retry: false,
  });
}

export function useTables(databaseId: string) {
  return useQuery({
    queryKey: ["tables", databaseId],
    queryFn: () => api.get<{ items: TableInfo[] }>(`/v1/databases/${databaseId}/tables`),
    enabled: Boolean(databaseId),
    retry: false,
  });
}

export function useTableRows(databaseId: string, table: string, limit: number, offset: number) {
  return useQuery({
    queryKey: ["table-rows", databaseId, table, limit, offset],
    queryFn: () =>
      api.get<RowsPage>(`/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/rows?limit=${limit}&offset=${offset}`),
    enabled: Boolean(databaseId && table),
    retry: false,
    placeholderData: (prev) => prev,
  });
}

export function useTableSchema(databaseId: string, table: string) {
  return useQuery({
    queryKey: ["table-schema", databaseId, table],
    queryFn: () =>
      api.get<TableSchema>(
        `/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/schema`,
      ),
    enabled: Boolean(databaseId && table),
    retry: false,
  });
}

// useRunQuery executes an ad-hoc SQL console statement. Whether writes are
// allowed is decided server-side from the caller's database:write permission.
export function useRunQuery(databaseId: string) {
  return useMutation({
    mutationFn: (input: RunQueryInput) =>
      api.post<QueryResult>(`/v1/databases/${databaseId}/query`, input),
  });
}

// exportTableCSV streams a whole table to a CSV download.
export function exportTableCSV(databaseId: string, table: string) {
  return download(
    `/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/export`,
  );
}

// exportQueryCSV streams a read-only query's result set to a CSV download.
export function exportQueryCSV(databaseId: string, sql: string) {
  return download(`/v1/databases/${databaseId}/export`, {
    method: "POST",
    body: { sql },
    filename: "query.csv",
  });
}

export function useDatabase(id: string) {
  return useQuery({
    queryKey: ["database", id],
    queryFn: () => api.get<Database>(`/v1/databases/${id}`),
    enabled: Boolean(id),
  });
}

export function useInstance(id: string) {
  return useQuery({
    queryKey: ["instance", id],
    queryFn: () => api.get<Instance>(`/v1/instances/${id}`),
    enabled: Boolean(id),
  });
}

// ---- Overview dashboard ----
export function useOverview() {
  return useQuery({
    queryKey: ["overview"],
    queryFn: () => api.get<Overview>("/v1/overview"),
    refetchInterval: 15_000,
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

// ---- Audit log ----
export function useAudit(params?: { resource_type?: string; resource_id?: string; page?: number }) {
  const q = new URLSearchParams();
  if (params?.resource_type) q.set("resource_type", params.resource_type);
  if (params?.resource_id) q.set("resource_id", params.resource_id);
  return useQuery({
    queryKey: ["audit", q.toString(), params?.page ?? 1],
    queryFn: () => api.get<Paginated<AuditEntry>>(`/v1/audit?${q.toString()}&${pageQS(params?.page)}`),
    refetchInterval: 10_000,
    placeholderData: (prev) => prev,
  });
}

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

// ---- Client-side pagination helper (for endpoints returning full lists) ----
export function useClientPage<T>(items: T[] | undefined, pageSize = LIST_PAGE_SIZE) {
  const [page, setPage] = useState(1);
  const all = items ?? [];
  const pageCount = Math.max(1, Math.ceil(all.length / pageSize));
  const current = Math.min(page, pageCount);
  return {
    page: current,
    setPage,
    pageCount,
    items: all.slice((current - 1) * pageSize, current * pageSize),
    total: all.length,
  };
}
