"use client";

import { useEffect, useState, type FormEvent } from "react";

import { ErrorText, Field, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useChangePassword, useProfile, useUpdateProfile } from "@/lib/hooks";

export default function ProfilePage() {
  const { data: profile, isLoading } = useProfile();

  if (isLoading || !profile) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  }

  return (
    <div style={{ maxWidth: 560 }}>
      <div style={{ marginBottom: "1.1rem" }}>
        <h1 className="text-xl font-semibold">Profile</h1>
        <p className="muted text-sm">Your account details and password.</p>
      </div>

      <div className="card" style={{ padding: "1.1rem", marginBottom: "1.25rem" }}>
        <div className="flex items-center justify-between" style={{ marginBottom: ".9rem" }}>
          <div>
            <div className="font-semibold">{profile.name}</div>
            <div className="muted text-sm">{profile.email}</div>
          </div>
          <div className="flex items-center gap-2">
            <StatusBadge status={profile.status} />
            <span className="badge badge-gray">{profile.roles.join(", ") || "no role"}</span>
          </div>
        </div>
        <ProfileForm initialName={profile.name} initialEmail={profile.email} />
      </div>

      <div className="card" style={{ padding: "1.1rem" }}>
        <h2 className="font-semibold" style={{ marginBottom: ".8rem" }}>Change password</h2>
        <PasswordForm />
      </div>
    </div>
  );
}

function ProfileForm({ initialName, initialEmail }: { initialName: string; initialEmail: string }) {
  const update = useUpdateProfile();
  const [name, setName] = useState(initialName);
  const [email, setEmail] = useState(initialEmail);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    setName(initialName);
    setEmail(initialEmail);
  }, [initialName, initialEmail]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSaved(false);
    try {
      await update.mutateAsync({ name, email });
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update profile");
    }
  }

  return (
    <form onSubmit={onSubmit}>
      <Field label="Name">
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
      </Field>
      <Field label="Email">
        <input className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      </Field>
      <ErrorText message={error ?? undefined} />
      <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
        {saved ? <span className="muted text-sm">Saved.</span> : null}
        <button type="submit" className="btn btn-primary" disabled={update.isPending}>
          {update.isPending ? "Saving…" : "Save profile"}
        </button>
      </div>
    </form>
  );
}

function PasswordForm() {
  const change = useChangePassword();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSaved(false);
    if (next !== confirm) {
      setError("New passwords do not match");
      return;
    }
    try {
      await change.mutateAsync({ current_password: current, new_password: next });
      setCurrent("");
      setNext("");
      setConfirm("");
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to change password");
    }
  }

  return (
    <form onSubmit={onSubmit}>
      <Field label="Current password">
        <input className="input" type="password" value={current} onChange={(e) => setCurrent(e.target.value)} autoComplete="current-password" required />
      </Field>
      <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
        <Field label="New password (min 8 chars)">
          <input className="input" type="password" value={next} onChange={(e) => setNext(e.target.value)} minLength={8} autoComplete="new-password" required />
        </Field>
        <Field label="Confirm new password">
          <input className="input" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} minLength={8} autoComplete="new-password" required />
        </Field>
      </div>
      <ErrorText message={error ?? undefined} />
      <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
        {saved ? <span className="muted text-sm">Password changed.</span> : null}
        <button type="submit" className="btn btn-primary" disabled={change.isPending}>
          {change.isPending ? "Saving…" : "Change password"}
        </button>
      </div>
    </form>
  );
}
