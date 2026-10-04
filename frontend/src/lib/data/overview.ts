"use client";

import { useQuery } from "@tanstack/react-query";

import { api } from "../api";
import type {
  Overview,
} from "../types";

// ---- Overview dashboard ----
export function useOverview() {
  return useQuery({
    queryKey: ["overview"],
    queryFn: () => api.get<Overview>("/v1/overview"),
    refetchInterval: 15_000,
  });
}
