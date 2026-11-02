"use client";

import { Archive, Database, Server } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";

import { ApiError, api, safeNext } from "@/lib/api";
import { clearLegacyToken } from "@/lib/auth";
import { Button, Callout, Field, Input } from "@/components/ui";
import { Logo } from "@/components/logo";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  // Set when the dashboard sent us here because the session ended: say so,
  // and go back to the page afterwards.
  const [expired, setExpired] = useState(false);
  const [next, setNext] = useState<string | null>(null);
  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    setExpired(q.get("expired") === "1");
    setNext(safeNext(q.get("next")));
  }, []);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      // The API sets the httpOnly session cookie; the token in the response
      // body is for API clients and is deliberately not kept.
      await api.post("/v1/auth/login", { email, password });
      clearLegacyToken();
      router.replace(next ?? "/dashboard");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Login failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="auth-shell">
      <aside className="auth-brand" aria-label="About Fleetdock">
        <div className="flex items-center gap-2">
          <Logo size={28} />
          <span className="font-semibold">Fleetdock</span>
        </div>
        <div>
          <p className="auth-brand-title">Every database you run, in one place.</p>
          <p>
            Connect your servers, watch their health, and keep backups you
            control — without stitching together a dozen tools.
          </p>
          <ul className="auth-points">
            <li>
              <Database size={16} aria-hidden />
              MariaDB, MySQL and PostgreSQL in one dashboard
            </li>
            <li>
              <Archive size={16} aria-hidden />
              Backups to your own S3-compatible storage
            </li>
            <li>
              <Server size={16} aria-hidden />
              Open source and self-hosted
            </li>
          </ul>
        </div>
      </aside>

      <main className="auth-main">
        <div className="card auth-card">
          <div className="mb-5">
            <h1 className="text-xl font-semibold m-0">Sign in</h1>
            <p className="muted text-sm mt-1 mb-0">
              Use the account your Fleetdock admin created for you.
            </p>
          </div>
          {expired ? (
            <Callout tone="info" className="mb-4" role="status">
              Your session ended. Sign in again to continue where you were.
            </Callout>
          ) : null}
          <form onSubmit={onSubmit}>
            <Field label="Email">
              <Input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                autoComplete="username"
                autoFocus
                required
              />
            </Field>
            <Field label="Password">
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                required
              />
            </Field>
            {error ? (
              <Callout tone="danger" className="mb-3">
                {error}
              </Callout>
            ) : null}
            <Button type="submit" variant="primary" block loading={loading} className="mt-2">
              {loading ? "Signing in…" : "Sign in"}
            </Button>
          </form>
        </div>
      </main>
    </div>
  );
}
