"use client";

import { KeyRound, UserRound } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";

import { FormError } from "@/components/access/form-error";
import { Badge, Button, Card, CardBody, CardHeader, Field, Input, PageHeader, QueryError, Skeleton, StatusBadge } from "@/components/ui";
import { fieldError } from "@/lib/errors";
import { useChangePassword, useProfile, useUpdateProfile } from "@/lib/hooks";

const header = <PageHeader title="Your profile" description="Your name, email and password." />;

export default function ProfilePage() {
  const { data: profile, isLoading, error, refetch } = useProfile();

  if (isLoading) {
    return (
      <div className="max-w-xl" role="status" aria-label="Loading profile">
        {header}
        <div className="stack" aria-hidden>
          {[0, 1].map((i) => (
            <div key={i} className="card panel stack-sm">
              <Skeleton width="35%" height=".95rem" />
              <Skeleton height="2.25rem" radius="var(--radius-md)" />
              <Skeleton height="2.25rem" radius="var(--radius-md)" />
            </div>
          ))}
        </div>
      </div>
    );
  }
  if (!profile) {
    return (
      <div className="max-w-xl">
        {header}
        <QueryError error={error} what="profile" onRetry={() => void refetch()} />
      </div>
    );
  }

  return (
    <div className="max-w-xl">
      {header}

      <Card className="mb-5">
        <CardHeader
          icon={<UserRound size={15} aria-hidden className="text-muted" />}
          title={profile.name}
          description={profile.email}
          actions={
            <>
              <StatusBadge status={profile.status} />
              <Badge tone="neutral">{profile.roles.join(", ") || "no role"}</Badge>
            </>
          }
        />
        <CardBody>
          <ProfileForm initialName={profile.name} initialEmail={profile.email} />
        </CardBody>
      </Card>

      <Card>
        <CardHeader icon={<KeyRound size={15} aria-hidden className="text-muted" />} title="Change password" />
        <CardBody>
          <PasswordForm />
        </CardBody>
      </Card>
    </div>
  );
}

function ProfileForm({ initialName, initialEmail }: { initialName: string; initialEmail: string }) {
  const update = useUpdateProfile();
  const [name, setName] = useState(initialName);
  const [email, setEmail] = useState(initialEmail);
  const [error, setError] = useState<unknown>(null);
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
      setError(err);
    }
  }

  return (
    <form onSubmit={onSubmit}>
      <Field label="Name" error={fieldError(error, "name")}>
        <Input value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" required />
      </Field>
      <Field label="Email" error={fieldError(error, "email")}>
        <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" required />
      </Field>
      <FormError error={error} fields={["name", "email"]} fallback="Failed to update profile" />
      <div className="form-actions mt-2 justify-end">
        {saved ? (
          <span className="muted text-sm" role="status">
            Saved.
          </span>
        ) : null}
        <Button type="submit" variant="primary" loading={update.isPending}>
          {update.isPending ? "Saving…" : "Save profile"}
        </Button>
      </div>
    </form>
  );
}

function PasswordForm() {
  const change = useChangePassword();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [mismatch, setMismatch] = useState(false);
  const [saved, setSaved] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setMismatch(false);
    setSaved(false);
    if (next !== confirm) {
      setMismatch(true);
      return;
    }
    try {
      await change.mutateAsync({ current_password: current, new_password: next });
      setCurrent("");
      setNext("");
      setConfirm("");
      setSaved(true);
    } catch (err) {
      setError(err);
    }
  }

  return (
    <form onSubmit={onSubmit}>
      <Field label="Current password" error={fieldError(error, "current_password")}>
        <Input
          type="password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          autoComplete="current-password"
          required
        />
      </Field>
      <div className="form-grid">
        <Field label="New password" hint="At least 8 characters." error={fieldError(error, "new_password")}>
          <Input
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            minLength={8}
            autoComplete="new-password"
            required
          />
        </Field>
        <Field label="Confirm new password" error={mismatch ? "New passwords do not match" : undefined}>
          <Input
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            minLength={8}
            autoComplete="new-password"
            required
          />
        </Field>
      </div>
      <FormError error={error} fields={["current_password", "new_password"]} fallback="Failed to change password" />
      <div className="form-actions mt-2 justify-end">
        {saved ? (
          <span className="muted text-sm" role="status">
            Password changed.
          </span>
        ) : null}
        <Button type="submit" variant="primary" loading={change.isPending}>
          {change.isPending ? "Saving…" : "Change password"}
        </Button>
      </div>
    </form>
  );
}
