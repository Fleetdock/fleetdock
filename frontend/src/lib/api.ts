import { clearToken, getToken } from "./auth";

// Same origin by default: one process serves both the dashboard and the API on
// one port and one domain, so a relative path is correct in every supported
// deployment and nothing about the host is baked into the build. In `next dev`
// the API paths are proxied by the rewrites in next.config.mjs, so this holds
// there too.
//
// NEXT_PUBLIC_API_URL remains an escape hatch for split-origin setups, where it
// must be a full origin (https://api.example.com). Setting it still requires
// rebuilding the bundle — Next inlines NEXT_PUBLIC_* at build time.
const configuredOrigin = process.env.NEXT_PUBLIC_API_URL?.trim().replace(/\/+$/, "");
export const API_URL = configuredOrigin || "";

export class ApiError extends Error {
  status: number;
  field?: string;
  constructor(message: string, status: number, field?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.field = field;
  }
}

async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const token = getToken();
  const res = await fetch(`${API_URL}${path}`, {
    ...opts,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(opts.headers ?? {}),
    },
  });

  if (res.status === 401) {
    clearToken();
    if (typeof window !== "undefined" && window.location.pathname !== "/login") {
      window.location.href = "/login";
    }
    throw new ApiError("unauthorized", 401);
  }

  if (res.status === 204) return undefined as T;

  const body = await res.json().catch(() => null);
  if (!res.ok) {
    const err = body?.error;
    throw new ApiError(err?.message ?? res.statusText, res.status, err?.field);
  }
  return body as T;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, data: unknown) =>
    request<T>(path, { method: "POST", body: JSON.stringify(data) }),
  patch: <T>(path: string, data: unknown) =>
    request<T>(path, { method: "PATCH", body: JSON.stringify(data) }),
  del: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};

// download fetches a file endpoint with auth and saves the response as a file.
// It is used for streamed CSV exports, which cannot go through a plain <a href>
// because the request needs the bearer token. On error the JSON body is parsed
// into an ApiError, mirroring request().
export async function download(
  path: string,
  opts: { method?: "GET" | "POST"; body?: unknown; filename?: string } = {},
): Promise<void> {
  const token = getToken();
  const res = await fetch(`${API_URL}${path}`, {
    method: opts.method ?? "GET",
    headers: {
      ...(opts.body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    ...(opts.body !== undefined ? { body: JSON.stringify(opts.body) } : {}),
  });

  if (res.status === 401) {
    clearToken();
    if (typeof window !== "undefined" && window.location.pathname !== "/login") {
      window.location.href = "/login";
    }
    throw new ApiError("unauthorized", 401);
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    const err = body?.error;
    throw new ApiError(err?.message ?? res.statusText, res.status, err?.field);
  }

  const blob = await res.blob();
  const filename =
    opts.filename ?? filenameFromDisposition(res.headers.get("Content-Disposition")) ?? "export.csv";
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

function filenameFromDisposition(header: string | null): string | undefined {
  if (!header) return undefined;
  const match = /filename="?([^";]+)"?/i.exec(header);
  return match?.[1];
}
