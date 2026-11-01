"use client";

import { useEffect, useState, type FormEvent } from "react";

import { FormError } from "@/components/access/form-error";
import { Button, Callout, Field, Input, Modal, Select } from "@/components/ui";
import { fieldError } from "@/lib/errors";
import { useCreateUser, useResetUserPassword, useUpdateUser } from "@/lib/hooks";
import type { Role, User } from "@/lib/types";

function RoleOptions({ roles }: { roles: Role[] }) {
  return (
    <>
      {roles.map((r) => (
        <option key={r.id} value={r.name}>
          {r.name} — {r.description}
        </option>
      ))}
    </>
  );
}

export function CreateUserModal({ open, onClose, roles }: { open: boolean; onClose: () => void; roles: Role[] }) {
  const create = useCreateUser();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("viewer");
  const [error, setError] = useState<unknown>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({ name, email, password, role });
      setName("");
      setEmail("");
      setPassword("");
      onClose();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Add user">
      <form onSubmit={onSubmit}>
        <Field label="Name" error={fieldError(error, "name")}>
          <Input value={name} onChange={(e) => setName(e.target.value)} autoComplete="off" required />
        </Field>
        <Field label="Email" error={fieldError(error, "email")}>
          <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" required />
        </Field>
        <Field label="Password" hint="At least 8 characters. Share it with the user securely." error={fieldError(error, "password")}>
          <Input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            minLength={8}
            autoComplete="new-password"
            required
          />
        </Field>
        <Field label="Role" error={fieldError(error, "role")}>
          <Select value={role} onChange={(e) => setRole(e.target.value)}>
            <RoleOptions roles={roles} />
          </Select>
        </Field>
        <FormError error={error} fields={["name", "email", "password", "role"]} fallback="Failed to create user" />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={create.isPending}>
            {create.isPending ? "Creating…" : "Create user"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

export function EditUserModal({
  user,
  onClose,
  roles,
  isSelf,
}: {
  user: User | null;
  onClose: () => void;
  roles: Role[];
  isSelf: boolean;
}) {
  const update = useUpdateUser();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("");
  const [status, setStatus] = useState("active");
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (user) {
      setName(user.name);
      setEmail(user.email);
      setRole(user.roles[0] ?? "viewer");
      setStatus(user.status);
      setError(null);
    }
  }, [user]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await update.mutateAsync({ id: user!.id, name, email, role, status });
      onClose();
    } catch (err) {
      setError(err);
    }
  }

  if (!user) return null;
  return (
    <Modal open onClose={onClose} title={`Edit ${user.email}`}>
      <form onSubmit={onSubmit}>
        <Field label="Name" error={fieldError(error, "name")}>
          <Input value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label="Email" error={fieldError(error, "email")}>
          <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </Field>
        <Field label="Role" error={fieldError(error, "role")}>
          <Select value={role} onChange={(e) => setRole(e.target.value)}>
            <RoleOptions roles={roles} />
          </Select>
        </Field>
        <Field
          label="Status"
          hint={isSelf ? "You cannot suspend your own account." : "Suspended users can't sign in."}
          error={fieldError(error, "status")}
        >
          <Select value={status} onChange={(e) => setStatus(e.target.value)} disabled={isSelf}>
            <option value="active">active</option>
            <option value="suspended">suspended</option>
          </Select>
        </Field>
        <FormError error={error} fields={["name", "email", "role", "status"]} fallback="Failed to update user" />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={update.isPending}>
            {update.isPending ? "Saving…" : "Save changes"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

export function ResetPasswordModal({ user, onClose }: { user: User | null; onClose: () => void }) {
  const reset = useResetUserPassword();
  const [password, setPassword] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [done, setDone] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await reset.mutateAsync({ id: user!.id, password });
      setDone(true);
    } catch (err) {
      setError(err);
    }
  }

  function close() {
    setPassword("");
    setDone(false);
    setError(null);
    onClose();
  }

  if (!user) return null;
  return (
    <Modal open onClose={close} title={`Reset password for ${user.email}`}>
      {done ? (
        <>
          <Callout tone="success" role="status">
            Password updated. Share it with the user securely.
          </Callout>
          <div className="modal-actions">
            <Button variant="primary" onClick={close}>
              Done
            </Button>
          </div>
        </>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="New password" hint="At least 8 characters." error={fieldError(error, "password")}>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              minLength={8}
              autoComplete="new-password"
              required
            />
          </Field>
          <FormError error={error} fields={["password"]} fallback="Failed to reset password" />
          <div className="modal-actions">
            <Button onClick={close}>Cancel</Button>
            <Button type="submit" variant="primary" loading={reset.isPending}>
              {reset.isPending ? "Saving…" : "Set password"}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
