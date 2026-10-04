"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, download } from "../api";
import type {
  HistoryEntry,
  QueryOutput,
  SavedQuery,
  AlterOp,
  DBObject,
  ForeignKey,
  TableSpec,
  BrowseRequest,
  BrowseResult,
  RowValues,
  CreateDatabaseInput,
  Connectivity,
  CredentialCreateResult,
  Database,
  DatabaseCredential,
  EndpointView,
  Instance,
  Paginated,
  StartMoveInput,
  CreateDBUserInput,
  DBUser,
  GrantInput,
  SchemaGrant,
  TableInfo,
  TableSchema,
  RunQueryInput,
} from "../types";
import { pageQS } from "./core";

// ---- Databases ----
export function useDatabases(params?: { instance_id?: string; search?: string; page?: number; enabled?: boolean }) {
  const q = new URLSearchParams();
  if (params?.instance_id) q.set("instance_id", params.instance_id);
  if (params?.search) q.set("search", params.search);
  const paging = params?.page === undefined ? "limit=100" : pageQS(params.page);
  const qs = q.toString();
  return useQuery({
    queryKey: ["databases", qs, params?.page ?? "all"],
    queryFn: () => api.get<Paginated<Database>>(`/v1/databases?${qs ? `${qs}&` : ""}${paging}`),
    enabled: params?.enabled ?? true,
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

export function useSetDBUserPassword() {
  return useMutation({
    mutationFn: ({ instanceId, ...input }: { instanceId: string; username: string; host: string; password: string }) =>
      api.post<{ status: string }>(`/v1/instances/${instanceId}/db-users/password`, input),
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
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: RunQueryInput) =>
      api.post<QueryOutput>(`/v1/databases/${databaseId}/query`, input),
    onSettled: () => qc.invalidateQueries({ queryKey: ["query-history", databaseId] }),
  });
}

export function cancelQuery(databaseId: string, queryId: string) {
  return api.post<void>(`/v1/databases/${databaseId}/query/${queryId}/cancel`, {});
}

export function useQueryHistory(databaseId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["query-history", databaseId],
    queryFn: () =>
      api.get<{ items: HistoryEntry[] }>(`/v1/databases/${databaseId}/query-history?limit=50`).then((r) => r.items),
    enabled: Boolean(databaseId) && enabled,
  });
}

export function useSavedQueries(databaseId: string) {
  return useQuery({
    queryKey: ["saved-queries", databaseId],
    queryFn: () =>
      api.get<{ items: SavedQuery[] }>(`/v1/saved-queries?database_id=${databaseId}`).then((r) => r.items),
    enabled: Boolean(databaseId),
  });
}

export function useSavedQueryMutations(databaseId: string) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["saved-queries", databaseId] });
  const create = useMutation({
    mutationFn: (q: { name: string; sql: string; database_id?: string }) => api.post<SavedQuery>("/v1/saved-queries", q),
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: ({ id, ...q }: { id: string; name: string; sql: string }) => api.patch<SavedQuery>(`/v1/saved-queries/${id}`, q),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del<void>(`/v1/saved-queries/${id}`),
    onSuccess: invalidate,
  });
  return { create, update, remove };
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

export function useConnectivity(databaseId: string) {
  return useQuery({
    queryKey: ["connectivity", databaseId],
    queryFn: () => api.get<Connectivity>(`/v1/databases/${databaseId}/connectivity`),
    enabled: Boolean(databaseId),
    retry: false,
    // Poll quickly while the gateway is converging, then back off. A settled
    // endpoint does not need to be re-fetched every 10s forever.
    refetchInterval: (query) => {
      const status = query.state.data?.public?.status;
      if (!status || status === "disabled") return false;
      return status === "active" ? 30_000 : 4_000;
    },
  });
}

// Enable/disable/update all enqueue a reconcile job, so the operations list
// must refresh alongside connectivity.
export function invalidateConnectivity(qc: ReturnType<typeof useQueryClient>, databaseId: string) {
  void qc.invalidateQueries({ queryKey: ["connectivity", databaseId] });
  void qc.invalidateQueries({ queryKey: ["operations"] });
}

export function useEnablePublicAccess(databaseId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { allowed_cidrs: string[]; tls_mode?: string }) =>
      api.post<{ endpoint: EndpointView; operation_id: string }>(`/v1/databases/${databaseId}/public-access`, body),
    onSuccess: () => invalidateConnectivity(qc, databaseId),
  });
}

export function useDisablePublicAccess(databaseId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.del<{ operation_id: string }>(`/v1/databases/${databaseId}/public-access`),
    onSuccess: () => invalidateConnectivity(qc, databaseId),
  });
}

// Changing the allowlist in place keeps the assigned port. Disabling and
// re-enabling would allocate a different one and break every existing client.
export function useUpdateAllowedCIDRs(databaseId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { allowed_cidrs: string[] }) =>
      api.patch<{ operation_id: string }>(`/v1/databases/${databaseId}/public-access`, body),
    onSuccess: () => invalidateConnectivity(qc, databaseId),
  });
}

export function useDatabaseCredentials(databaseId: string) {
  return useQuery({
    queryKey: ["credentials", databaseId],
    queryFn: () => api.get<{ items: DatabaseCredential[] }>(`/v1/databases/${databaseId}/credentials`),
    enabled: Boolean(databaseId),
    retry: false,
  });
}

export function useCreateCredential(databaseId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; access_level: string; use_public?: boolean; username?: string }) =>
      api.post<CredentialCreateResult>(`/v1/databases/${databaseId}/credentials`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["credentials", databaseId] }),
  });
}

export function useRotateCredential(databaseId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (credentialId: string) =>
      api.post<CredentialCreateResult>(`/v1/databases/${databaseId}/credentials/${credentialId}/rotate`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["credentials", databaseId] }),
  });
}

export function useRevokeCredential(databaseId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (credentialId: string) => api.del<void>(`/v1/databases/${databaseId}/credentials/${credentialId}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["credentials", databaseId] }),
  });
}

export function useInstance(id: string) {
  return useQuery({
    queryKey: ["instance", id],
    queryFn: () => api.get<Instance>(`/v1/instances/${id}`),
    enabled: Boolean(id),
  });
}

// ---- Data editing ----
export function useBrowseRows(databaseId: string, table: string, req: BrowseRequest) {
  return useQuery({
    queryKey: ["browse-rows", databaseId, table, req],
    queryFn: () =>
      api.post<BrowseResult>(`/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/browse`, req),
    enabled: Boolean(databaseId && table),
    retry: false,
    placeholderData: (prev) => prev,
  });
}

export function rowsPath(databaseId: string, table: string) {
  return `/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/rows`;
}

export function useRowMutations(databaseId: string, table: string) {
  const qc = useQueryClient();
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["browse-rows", databaseId, table] });
    qc.invalidateQueries({ queryKey: ["tables", databaseId] });
  };
  const insert = useMutation({
    mutationFn: (values: RowValues) => api.post<void>(rowsPath(databaseId, table), { values }),
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: ({ key, values }: { key: RowValues; values: RowValues }) =>
      api.patch<void>(rowsPath(databaseId, table), { key, values }),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: (key: RowValues) => api.post<void>(`${rowsPath(databaseId, table)}/delete`, { key }),
    onSuccess: invalidate,
  });
  const importCSV = useMutation({
    mutationFn: ({ file, emptyAsNull }: { file: File; emptyAsNull: boolean }) =>
      api.upload<{ imported: number }>(
        `/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/import${emptyAsNull ? "?empty_as_null=true" : ""}`,
        file,
        "text/csv",
      ),
    onSuccess: invalidate,
  });
  return { insert, update, remove, importCSV };
}

// ---- Structure ----
export function useForeignKeys(databaseId: string, table: string) {
  return useQuery({
    queryKey: ["foreign-keys", databaseId, table],
    queryFn: () =>
      api
        .get<{ items: ForeignKey[] }>(`/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/foreign-keys`)
        .then((r) => r.items),
    enabled: Boolean(databaseId && table),
    retry: false,
  });
}

export function useDBObjects(databaseId: string) {
  return useQuery({
    queryKey: ["db-objects", databaseId],
    queryFn: () => api.get<{ items: DBObject[] }>(`/v1/databases/${databaseId}/objects`).then((r) => r.items),
    enabled: Boolean(databaseId),
    retry: false,
  });
}

/** useStructureMutations covers DDL on one table (and table creation). */
export function useStructureMutations(databaseId: string, table = "") {
  const qc = useQueryClient();
  const base = `/v1/databases/${databaseId}/tables`;
  const t = `${base}/${encodeURIComponent(table)}`;
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["tables", databaseId] });
    qc.invalidateQueries({ queryKey: ["table-schema", databaseId] });
    qc.invalidateQueries({ queryKey: ["foreign-keys", databaseId] });
    qc.invalidateQueries({ queryKey: ["browse-rows", databaseId] });
    qc.invalidateQueries({ queryKey: ["db-objects", databaseId] });
  };
  const createTable = useMutation({ mutationFn: (spec: TableSpec) => api.post(base, spec), onSuccess: invalidate });
  const alter = useMutation({ mutationFn: (ops: AlterOp[]) => api.patch(t, { ops }), onSuccess: invalidate });
  const drop = useMutation({ mutationFn: () => api.post(`${t}/drop`, {}), onSuccess: invalidate });
  const truncate = useMutation({ mutationFn: () => api.post(`${t}/truncate`, {}), onSuccess: invalidate });
  const rename = useMutation({
    mutationFn: (newName: string) => api.post(`${t}/rename`, { new_name: newName }),
    onSuccess: invalidate,
  });
  const createIndex = useMutation({
    mutationFn: (spec: { name: string; columns: string[]; unique: boolean }) => api.post(`${t}/indexes`, spec),
    onSuccess: invalidate,
  });
  const dropIndex = useMutation({
    mutationFn: (name: string) => api.post(`${t}/indexes/drop`, { name }),
    onSuccess: invalidate,
  });
  return { createTable, alter, drop, truncate, rename, createIndex, dropIndex };
}
