"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { ApiError, api } from "@/lib/api";
import { setToken } from "@/lib/auth";
import { ErrorText, Field } from "@/components/ui";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      const res = await api.post<{ token: string }>("/v1/auth/login", { email, password });
      setToken(res.token);
      router.replace("/dashboard");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Login failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div
      className="flex items-center justify-center"
      style={{ minHeight: "100vh", padding: "1rem" }}
    >
      <div className="card" style={{ width: "100%", maxWidth: "22rem", padding: "1.75rem" }}>
        <div className="flex items-center gap-2" style={{ marginBottom: "1.25rem" }}>
          <div
            className="flex items-center justify-center"
            style={{ width: 28, height: 28, borderRadius: 8, background: "var(--primary)", color: "var(--primary-fg)", fontWeight: 700, fontSize: 13 }}
          >
            db
          </div>
          <div>
            <div className="font-semibold">db-manager</div>
            <div className="muted text-sm">Sign in to continue</div>
          </div>
        </div>
        <form onSubmit={onSubmit}>
          <Field label="Email">
            <input
              className="input"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="username"
              required
            />
          </Field>
          <Field label="Password">
            <input
              className="input"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
          </Field>
          <ErrorText message={error ?? undefined} />
          <button className="btn btn-primary" type="submit" disabled={loading} style={{ width: "100%", marginTop: ".5rem" }}>
            {loading ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </div>
    </div>
  );
}
