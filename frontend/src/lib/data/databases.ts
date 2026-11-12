"use client";

import { useCallback } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, download } from "../api";
import { randomUUID } from "../uuid";
import type {
  ColumnCatalog,
  ExtensionInfo,
  MaintenanceOp,
  PartitionInfo,
  SequenceInfo,
  TableStat,
  DefaultPrivilegeObject,
  SchemaAccess,
  SearchPaths,
  Diagram,
  RowFilter,
  SortKey,
  RowChange,
  RowEditResult,
  UndoResult,
  IndexInsights,
  ExplainResult,
  SchemaSearchHit,
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
  SchemaInfo,
  SchemaDependents,
  SchemaPatch,
} from "../types";
import type { Mutation, MutationResult } from "../data-browser/pending";
import { writeStatements } from "../sql";
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
    onSettled: (_out, _err, input) => {
      qc.invalidateQueries({ queryKey: ["query-history", databaseId] });
      // A script that wrote may have changed the schema autocompletion uses.
      if (writeStatements(input.sql).length > 0) {
        qc.invalidateQueries({ queryKey: ["schema-columns", databaseId] });
        qc.invalidateQueries({ queryKey: ["diagram", databaseId] });
        qc.invalidateQueries({ queryKey: ["schemas", databaseId] });
      }
    },
  });
}

// useExplain asks for a statement's plan; analyze runs it read-only and
// rolls it back.
export function useExplain(databaseId: string) {
  return useMutation({
    mutationFn: (input: { sql: string; analyze: boolean }) =>
      api.post<ExplainResult>(`/v1/databases/${databaseId}/explain`, input),
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

// exportTableCSV streams a table's rows to a CSV download: those the data
// browser's filters, search and sort select, or the whole table without them.
export function exportTableCSV(
  databaseId: string,
  table: string,
  view?: { filters: RowFilter[]; sort: SortKey[]; search: string },
) {
  const path = `/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/export`;
  return view ? download(path, { method: "POST", body: view }) : download(path);
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

// useApplyRowChanges sends a Data Browser tab's pending edits through the row
// endpoints, one row at a time (each in its own transaction server-side), and
// reports every outcome so failures can stay on screen. The table is
// refreshed once at the end rather than after every row.
export interface AppliedChanges {
  results: MutationResult[];
  /** Groups this save's changes in the journal; undo it with useUndoBatch. */
  batchId: string;
}

export function useApplyRowChanges(databaseId: string, table: string) {
  const qc = useQueryClient();
  return useCallback(
    async (mutations: Mutation[]): Promise<AppliedChanges> => {
      const path = rowsPath(databaseId, table);
      const batchId = randomUUID();
      const results: MutationResult[] = [];
      for (const m of mutations) {
        try {
          let res: RowEditResult;
          if (m.kind === "delete") res = await api.post<RowEditResult>(`${path}/delete`, { key: m.key, batch_id: batchId });
          else if (m.kind === "update") res = await api.patch<RowEditResult>(path, { key: m.key, values: m.values, batch_id: batchId });
          else res = await api.post<RowEditResult>(path, { values: m.values, batch_id: batchId });
          results.push({ kind: m.kind, id: m.id, ok: true, changeId: res?.change_id, undoable: res?.undoable });
        } catch (err) {
          results.push({ kind: m.kind, id: m.id, ok: false, error: err instanceof Error ? err.message : "Failed" });
        }
      }
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["browse-rows", databaseId, table] }),
        qc.invalidateQueries({ queryKey: ["tables", databaseId] }),
        qc.invalidateQueries({ queryKey: ["row-changes", databaseId] }),
      ]);
      return { results, batchId };
    },
    [qc, databaseId, table],
  );
}

// useRowChanges lists the change journal of a table, newest first.
export function useRowChanges(databaseId: string, table: string, enabled = true) {
  return useQuery({
    queryKey: ["row-changes", databaseId, table],
    queryFn: () =>
      api
        .get<{ items: RowChange[] }>(`/v1/databases/${databaseId}/changes?table=${encodeURIComponent(table)}&limit=100`)
        .then((r) => r.items),
    enabled: Boolean(databaseId && table) && enabled,
    retry: false,
  });
}

// useUndo reverts one change (changeId) or a whole save (batchId).
export function useUndo(databaseId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { changeId?: string; batchId?: string; force?: boolean }): Promise<UndoResult[]> => {
      if (input.changeId) {
        const r = await api.post<UndoResult>(`/v1/databases/${databaseId}/changes/${input.changeId}/undo`, { force: input.force ?? false });
        return [r];
      }
      const r = await api.post<{ items: UndoResult[] }>(`/v1/databases/${databaseId}/changes/undo`, {
        batch_id: input.batchId,
        force: input.force ?? false,
      });
      return r.items;
    },
    onSettled: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ["row-changes", databaseId] }),
        qc.invalidateQueries({ queryKey: ["browse-rows", databaseId] }),
        qc.invalidateQueries({ queryKey: ["tables", databaseId] }),
      ]),
  });
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

export function useReferencedBy(databaseId: string, table: string) {
  return useQuery({
    queryKey: ["foreign-keys", databaseId, table, "incoming"],
    queryFn: () =>
      api
        .get<{ items: ForeignKey[] }>(`/v1/databases/${databaseId}/tables/${encodeURIComponent(table)}/referenced-by`)
        .then((r) => r.items),
    enabled: Boolean(databaseId && table),
    retry: false,
  });
}

// useSchemaSearch finds tables and columns by name across every database the
// caller may open (GET /v1/search/schema). Queries under two characters skip
// the request.
export function useSchemaSearch(q: string, enabled = true) {
  const term = q.trim();
  return useQuery({
    queryKey: ["schema-search", term],
    queryFn: () =>
      api.get<{ items: SchemaSearchHit[] }>(`/v1/search/schema?q=${encodeURIComponent(term)}&limit=20`).then((r) => r.items),
    enabled: enabled && term.length >= 2,
    staleTime: 30_000,
    retry: false,
  });
}

export function useIndexInsights(databaseId: string, enabled = true) {
  return useQuery({
    queryKey: ["index-insights", databaseId],
    queryFn: () => api.get<IndexInsights>(`/v1/databases/${databaseId}/index-insights`),
    enabled: Boolean(databaseId) && enabled,
    staleTime: 60_000,
    retry: false,
  });
}

// useSchemaColumns loads every table's columns for SQL autocompletion. The
// catalog changes rarely, so it is cached for a few minutes and refreshed after
// DDL from the structure editor or the console.
export function useSchemaColumns(databaseId: string) {
  return useQuery({
    queryKey: ["schema-columns", databaseId],
    queryFn: () => api.get<ColumnCatalog>(`/v1/databases/${databaseId}/columns`),
    enabled: Boolean(databaseId),
    staleTime: 5 * 60_000,
    retry: false,
  });
}

// useDiagram loads a database's tables, columns and foreign keys for its ER
// diagram (GET /v1/databases/{id}/diagram).
// useDiagram loads a database's diagram, limited to some PostgreSQL schemas
// when schemas is given. The previous diagram stays on screen while another
// selection loads.
export function useDiagram(databaseId: string, schemas?: string[], enabled = true) {
  const key = schemas?.join(",") ?? "";
  return useQuery({
    queryKey: ["diagram", databaseId, key],
    queryFn: () =>
      api.get<Diagram>(`/v1/databases/${databaseId}/diagram${key ? `?schemas=${encodeURIComponent(key)}` : ""}`),
    enabled: Boolean(databaseId) && enabled,
    staleTime: 60_000,
    retry: false,
    placeholderData: (prev) => prev,
  });
}

// useSchemas lists a PostgreSQL database's schemas, including empty ones
// (GET /v1/databases/{id}/schemas). Off for other engines.
export function useSchemas(databaseId: string, enabled = true) {
  return useQuery({
    queryKey: ["schemas", databaseId],
    queryFn: () => api.get<{ items: SchemaInfo[] }>(`/v1/databases/${databaseId}/schemas`).then((r) => r.items),
    enabled: Boolean(databaseId) && enabled,
    staleTime: 30_000,
    retry: false,
  });
}

// useSchemaMutations creates, alters and drops PostgreSQL schemas. A schema
// change can move or remove tables, so everything listing them is refreshed.
export function useSchemaMutations(databaseId: string) {
  const qc = useQueryClient();
  const base = `/v1/databases/${databaseId}/schemas`;
  const invalidate = () => {
    for (const key of ["schemas", "tables", "db-objects", "schema-columns", "diagram", "table-schema", "foreign-keys"]) {
      qc.invalidateQueries({ queryKey: [key, databaseId] });
    }
  };
  const create = useMutation({
    mutationFn: (v: { name: string; owner?: string }) => api.post(base, v),
    onSuccess: invalidate,
  });
  const alter = useMutation({
    mutationFn: (v: { schema: string; change: SchemaPatch }) =>
      api.patch(`${base}/${encodeURIComponent(v.schema)}`, v.change),
    onSuccess: invalidate,
  });
  const drop = useMutation({
    mutationFn: (v: { schema: string; cascade: boolean }) =>
      api.post(`${base}/${encodeURIComponent(v.schema)}/drop`, { cascade: v.cascade }),
    onSuccess: invalidate,
  });
  const move = useMutation({
    mutationFn: (v: { kind: "relation" | "function" | "procedure" | "type"; schema: string; name: string; target: string }) =>
      api.post(`/v1/databases/${databaseId}/objects/move`, v),
    onSuccess: invalidate,
  });
  // A copy runs as an operation: the new schema shows up once it finishes.
  const copy = useMutation({
    mutationFn: (v: { schema: string; target_schema: string; target_database_id?: string; structure_only: boolean }) =>
      api.post<{ operation_id: string }>(`${base}/${encodeURIComponent(v.schema)}/copy`, {
        target_schema: v.target_schema,
        target_database_id: v.target_database_id,
        structure_only: v.structure_only,
      }),
  });
  return { create, alter, drop, move, copy };
}

// useSchemaDependents loads what dropping a schema would remove.
export function useSchemaDependents(databaseId: string, schema: string | null) {
  return useQuery({
    queryKey: ["schema-dependents", databaseId, schema],
    queryFn: () =>
      api.get<SchemaDependents>(`/v1/databases/${databaseId}/schemas/${encodeURIComponent(schema ?? "")}/dependents`),
    enabled: Boolean(databaseId && schema),
    retry: false,
    staleTime: 0,
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
    qc.invalidateQueries({ queryKey: ["schema-columns", databaseId] });
    qc.invalidateQueries({ queryKey: ["index-insights", databaseId] });
    qc.invalidateQueries({ queryKey: ["diagram", databaseId] });
    qc.invalidateQueries({ queryKey: ["schemas", databaseId] });
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

// useSchemaAccess loads who may use a PostgreSQL schema and its default
// privileges.
export function useSchemaAccess(databaseId: string, schema: string | null) {
  return useQuery({
    queryKey: ["schema-access", databaseId, schema],
    queryFn: () =>
      api.get<SchemaAccess>(`/v1/databases/${databaseId}/schemas/${encodeURIComponent(schema ?? "")}/privileges`),
    enabled: Boolean(databaseId && schema),
    retry: false,
  });
}

// useSchemaPrivilegeMutations sets a role's USAGE/CREATE on a schema and
// changes its default privileges.
export function useSchemaPrivilegeMutations(databaseId: string) {
  const qc = useQueryClient();
  const path = (schema: string) => `/v1/databases/${databaseId}/schemas/${encodeURIComponent(schema)}`;
  const invalidate = (_d: unknown, v: { schema: string }) => {
    qc.invalidateQueries({ queryKey: ["schema-access", databaseId, v.schema] });
    qc.invalidateQueries({ queryKey: ["schema-grants", databaseId] });
  };
  const setGrant = useMutation({
    mutationFn: (v: { schema: string; role: string; usage: boolean; create: boolean }) =>
      api.put(`${path(v.schema)}/privileges`, { role: v.role, usage: v.usage, create: v.create }),
    onSuccess: invalidate,
  });
  const alterDefaults = useMutation({
    mutationFn: (v: {
      schema: string;
      creator?: string;
      object_type: DefaultPrivilegeObject;
      grantee: string;
      privileges: string[];
      revoke: boolean;
    }) => {
      const { schema, ...body } = v;
      return api.post(`${path(schema)}/default-privileges`, body);
    },
    onSuccess: invalidate,
  });
  return { setGrant, alterDefaults };
}

// useSearchPaths loads the search_path of a PostgreSQL database and its roles.
export function useSearchPaths(databaseId: string, enabled = true) {
  return useQuery({
    queryKey: ["search-path", databaseId],
    queryFn: () => api.get<SearchPaths>(`/v1/databases/${databaseId}/search-path`),
    enabled: Boolean(databaseId) && enabled,
    retry: false,
  });
}

// useSetSearchPath sets or (with no schemas) resets a search_path.
export function useSetSearchPath(databaseId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { role?: string; schemas: string[] }) => api.put(`/v1/databases/${databaseId}/search-path`, v),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["search-path", databaseId] }),
  });
}

const enc = encodeURIComponent;

// useSequence loads a PostgreSQL sequence and its current value.
export function useSequence(databaseId: string, schema: string, name: string) {
  return useQuery({
    queryKey: ["sequence", databaseId, schema, name],
    queryFn: () => api.get<SequenceInfo>(`/v1/databases/${databaseId}/sequences/${enc(schema)}/${enc(name)}`),
    enabled: Boolean(databaseId && schema && name),
    retry: false,
  });
}

// useTableStats loads tables' sizes, dead rows and last vacuum and analyze.
export function useTableStats(databaseId: string, schema: string) {
  return useQuery({
    queryKey: ["table-stats", databaseId, schema],
    queryFn: () => api.get<{ items: TableStat[] }>(`/v1/databases/${databaseId}/table-stats?schema=${enc(schema)}`),
    enabled: Boolean(databaseId),
    retry: false,
  });
}

// usePartitions loads a partitioned table's partitions with their bounds.
export function usePartitions(databaseId: string, schema: string, table: string) {
  return useQuery({
    queryKey: ["partitions", databaseId, schema, table],
    queryFn: () => api.get<{ items: PartitionInfo[] }>(`/v1/databases/${databaseId}/partitions/${enc(schema)}/${enc(table)}`),
    enabled: Boolean(databaseId && schema && table),
    retry: false,
  });
}

// useMaintenanceMutations sets sequences, refreshes materialized views and
// runs VACUUM / ANALYZE / REINDEX.
export function useMaintenanceMutations(databaseId: string) {
  const qc = useQueryClient();
  const base = `/v1/databases/${databaseId}`;
  const setval = useMutation({
    mutationFn: (v: { schema: string; name: string; value: number; called: boolean }) =>
      api.post(`${base}/sequences/${enc(v.schema)}/${enc(v.name)}/setval`, { value: v.value, called: v.called }),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["sequence", databaseId, v.schema, v.name] }),
  });
  const refresh = useMutation({
    mutationFn: (v: { schema: string; name: string; concurrently: boolean }) =>
      api.post(`${base}/matviews/${enc(v.schema)}/${enc(v.name)}/refresh`, { concurrently: v.concurrently }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["table-stats", databaseId] }),
  });
  const maintain = useMutation({
    mutationFn: (v: { op: MaintenanceOp; schema: string; table?: string }) =>
      api.post<{ ran: string[] }>(`${base}/maintenance`, v),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["table-stats", databaseId] });
      qc.invalidateQueries({ queryKey: ["tables", databaseId] });
    },
  });
  return { setval, refresh, maintain };
}

// useExtensions loads a PostgreSQL database's available and installed
// extensions.
export function useExtensions(databaseId: string, enabled = true) {
  return useQuery({
    queryKey: ["extensions", databaseId],
    queryFn: () => api.get<{ items: ExtensionInfo[] }>(`/v1/databases/${databaseId}/extensions`),
    enabled: Boolean(databaseId) && enabled,
    retry: false,
  });
}

// useExtensionMutations installs, updates and drops extensions. They bring
// types and functions, so the schema views are refreshed too.
export function useExtensionMutations(databaseId: string) {
  const qc = useQueryClient();
  const base = `/v1/databases/${databaseId}/extensions`;
  const invalidate = () => {
    for (const key of ["extensions", "schemas", "db-objects", "schema-columns", "tables"]) {
      qc.invalidateQueries({ queryKey: [key, databaseId] });
    }
  };
  const install = useMutation({
    mutationFn: (v: { name: string; schema?: string; version?: string }) => api.post(base, v),
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: (v: { name: string; version?: string }) =>
      api.post(`${base}/${encodeURIComponent(v.name)}/update`, { version: v.version }),
    onSuccess: invalidate,
  });
  const drop = useMutation({
    mutationFn: (v: { name: string; cascade: boolean }) =>
      api.post(`${base}/${encodeURIComponent(v.name)}/drop`, { cascade: v.cascade }),
    onSuccess: invalidate,
  });
  return { install, update, drop };
}
