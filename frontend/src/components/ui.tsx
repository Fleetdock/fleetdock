"use client";

import type { ReactNode } from "react";

export function Modal({
  open,
  onClose,
  title,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
}) {
  if (!open) return null;
  return (
    <div className="overlay" onClick={onClose}>
      <div className="card modal" style={{ padding: "1.25rem" }} onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between" style={{ marginBottom: "1rem" }}>
          <h3 className="text-base font-semibold">{title}</h3>
          <button className="btn btn-ghost btn-sm" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="field">
      <label className="label">{label}</label>
      {children}
    </div>
  );
}

const STATUS_CLASS: Record<string, string> = {
  online: "badge-green",
  active: "badge-green",
  running: "badge-green",
  pending: "badge-amber",
  provisioning: "badge-amber",
  locked: "badge-amber",
  draining: "badge-amber",
  migrating: "badge-amber",
  offline: "badge-gray",
  stopped: "badge-gray",
  deleting: "badge-red",
  error: "badge-red",
};

export function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`badge ${STATUS_CLASS[status] ?? "badge-gray"}`}>
      <span className="dot" />
      {status}
    </span>
  );
}

export function Spinner() {
  return <span className="spin" aria-label="loading" />;
}

export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="card" style={{ padding: "2.5rem", textAlign: "center" }}>
      <p className="font-medium">{title}</p>
      {hint ? <p className="muted text-sm" style={{ marginTop: ".35rem" }}>{hint}</p> : null}
    </div>
  );
}

export function ErrorText({ message }: { message?: string }) {
  if (!message) return null;
  return (
    <p className="text-sm" style={{ color: "var(--danger)", marginTop: ".25rem" }}>
      {message}
    </p>
  );
}
