"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, download } from "../api";
import type { CreateFleetRunInput, Database, FleetRun, FleetTarget, Paginated } from "../types";

// Fleet queries: one script run on many databases in the background.

const ACTIVE = new Set(["running", "canceling"]);

export function useFleetRuns() {
  return useQuery({
    queryKey: ["fleet-runs"],
    queryFn: () => api.get<{ items: FleetRun[] }>("/v1/fleet-runs").then((r) => r.items),
    refetchInterval: (q) => ((q.state.data ?? []).some((r) => ACTIVE.has(r.status)) ? 3000 : false),
  });
}

export function useFleetRun(id: string) {
  return useQuery({
    queryKey: ["fleet-run", id],
    queryFn: () => api.get<FleetRun>(`/v1/fleet-runs/${id}`),
    enabled: Boolean(id),
    refetchInterval: (q) => (q.state.data && ACTIVE.has(q.state.data.status) ? 1500 : false),
  });
}

export function useFleetTargets(id: string, active: boolean) {
  return useQuery({
    queryKey: ["fleet-targets", id],
    queryFn: () => api.get<{ items: FleetTarget[] }>(`/v1/fleet-runs/${id}/targets`).then((r) => r.items),
    enabled: Boolean(id),
    // Until every target has settled: the run can report finished a moment
    // before the last target's results were fetched.
    refetchInterval: (q) =>
      active || (q.state.data ?? []).some((t) => t.status === "pending" || t.status === "running") ? 1500 : false,
  });
}

export function useCreateFleetRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateFleetRunInput) => api.post<FleetRun>("/v1/fleet-runs", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["fleet-runs"] }),
  });
}

export function useFleetRunActions(id: string) {
  const qc = useQueryClient();
  const settle = () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: ["fleet-run", id] }),
      qc.invalidateQueries({ queryKey: ["fleet-targets", id] }),
      qc.invalidateQueries({ queryKey: ["fleet-runs"] }),
    ]);
  const post = (name: "resume" | "cancel" | "retry") => () => api.post<FleetRun>(`/v1/fleet-runs/${id}/${name}`, {});
  return {
    resume: useMutation({ mutationFn: post("resume"), onSettled: settle }),
    cancel: useMutation({ mutationFn: post("cancel"), onSettled: settle }),
    retry: useMutation({ mutationFn: post("retry"), onSettled: settle }),
  };
}

export function downloadFleetCSV(id: string) {
  return download(`/v1/fleet-runs/${id}/results.csv`, { filename: `fleet-run-${id.slice(0, 8)}.csv` });
}

/** useAllDatabases lists every database the user can see (up to 1000), for picking fleet targets. */
export function useAllDatabases() {
  return useQuery({
    queryKey: ["databases", "all-pages"],
    queryFn: async () => {
      const out: Database[] = [];
      for (let offset = 0; offset < 1000; offset += 100) {
        const page = await api.get<Paginated<Database>>(`/v1/databases?limit=100&offset=${offset}`);
        out.push(...page.items);
        if (page.items.length < 100 || out.length >= page.pagination.total) break;
      }
      return out;
    },
    staleTime: 30_000,
  });
}
