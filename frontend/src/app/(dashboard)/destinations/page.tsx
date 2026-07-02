"use client";

import { useEffect, useState, type FormEvent } from "react";

import { Pencil, Plug, Plus, Trash2 } from "lucide-react";
import { EmptyState, ErrorText, Field, Modal, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCreateDestination,
  useDeleteDestination,
  useDestinations,
  useTestDestination,
  useUpdateDestination,
} from "@/lib/hooks";
import type { Destination } from "@/lib/types";

const PROVIDER_LABEL: Record<string, string> = {
  s3: "AWS S3",
  r2: "Cloudflare R2",
  s3_compatible: "S3-compatible",
};

export default function DestinationsPage() {
  const { data, isLoading, error } = useDestinations();
  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Destination | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const del = useDeleteDestination();
  const test = useTestDestination();

  async function onTest(id: string, name: string) {
    setNotice(null);
    try {
      await test.mutateAsync(id);
      setNotice(`"${name}": bucket is reachable.`);
    } catch (err) {
      setNotice(err instanceof ApiError ? `"${name}": ${err.message}` : "Test failed");
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Backup destinations</h1>
          <p className="muted text-sm">S3 / Cloudflare R2 buckets your backups upload to.</p>
        </div>
        <button className="btn btn-primary" onClick={() => setAddOpen(true)}>
          <Plus size={16} /> Add destination
        </button>
      </div>

      {notice ? (
        <div className="card" style={{ padding: ".7rem .9rem", marginBottom: ".9rem" }}>
          <span className="text-sm">{notice}</span>
        </div>
      ) : null}

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load destinations" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No destinations yet" hint="Add an S3 or Cloudflare R2 bucket to enable backups." />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Provider</th>
                <th>Bucket</th>
                <th>Endpoint</th>
                <th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((d) => (
                <tr key={d.id}>
                  <td className="font-medium">{d.name}</td>
                  <td className="muted">{PROVIDER_LABEL[d.provider] ?? d.provider}</td>
                  <td className="muted">{d.bucket}{d.prefix ? `/${d.prefix}` : ""}</td>
                  <td className="muted">{d.endpoint || "AWS default"}</td>
                  <td style={{ textAlign: "right" }}>
                    <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
                      <button className="btn btn-sm" onClick={() => onTest(d.id, d.name)} disabled={test.isPending}>
                        <Plug size={15} /> Test
                      </button>
                      <button
                        className="btn btn-sm"
                        onClick={() => setEditTarget(d)}
                        aria-label={`Edit ${d.name}`}
                      >
                        <Pencil size={15} />
                      </button>
                      <button
                        className="btn btn-sm btn-danger"
                        onClick={() => {
                          if (confirm(`Delete destination "${d.name}"? Existing backups keep their records.`)) {
                            del.mutate(d.id);
                          }
                        }}
                        disabled={del.isPending}
                        aria-label="Delete"
                      >
                        <Trash2 size={15} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <DestinationModal mode="create" open={addOpen} onClose={() => setAddOpen(false)} />
      <DestinationModal
        mode="edit"
        destination={editTarget}
        open={editTarget !== null}
        onClose={() => setEditTarget(null)}
      />
    </div>
  );
}

type DestinationModalProps =
  | { mode: "create"; destination?: undefined; open: boolean; onClose: () => void }
  | { mode: "edit"; destination: Destination | null; open: boolean; onClose: () => void };

function DestinationModal({ mode, destination, open, onClose }: DestinationModalProps) {
  const create = useCreateDestination();
  const update = useUpdateDestination();
  const isEdit = mode === "edit";

  const [name, setName] = useState("");
  const [provider, setProvider] = useState("r2");
  const [bucket, setBucket] = useState("");
  const [region, setRegion] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [prefix, setPrefix] = useState("");
  const [accessKey, setAccessKey] = useState("");
  const [secretKey, setSecretKey] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    if (isEdit && destination) {
      setName(destination.name);
      setProvider(destination.provider);
      setBucket(destination.bucket);
      setRegion(destination.region ?? "");
      setEndpoint(destination.endpoint ?? "");
      setPrefix(destination.prefix ?? "");
      setAccessKey(destination.access_key_id);
      setSecretKey("");
    } else if (!isEdit) {
      setName("");
      setProvider("r2");
      setBucket("");
      setRegion("");
      setEndpoint("");
      setPrefix("");
      setAccessKey("");
      setSecretKey("");
    }
    setError(null);
  }, [open, isEdit, destination]);

  const pending = create.isPending || update.isPending;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const payload = {
      name,
      provider,
      bucket,
      region: region || undefined,
      endpoint: endpoint || undefined,
      prefix: prefix || undefined,
      access_key_id: accessKey,
    };
    try {
      if (isEdit && destination) {
        await update.mutateAsync({
          id: destination.id,
          ...payload,
          ...(secretKey ? { secret_access_key: secretKey } : {}),
        });
      } else {
        await create.mutateAsync({ ...payload, secret_access_key: secretKey });
      }
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : `Failed to ${isEdit ? "update" : "add"} destination`);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={isEdit ? "Edit backup destination" : "Add backup destination"}
    >
      <form onSubmit={onSubmit}>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Name">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="prod-backups" required />
          </Field>
          <Field label="Provider">
            <select className="input" value={provider} onChange={(e) => setProvider(e.target.value)}>
              <option value="r2">Cloudflare R2</option>
              <option value="s3">AWS S3</option>
              <option value="s3_compatible">Other S3-compatible</option>
            </select>
          </Field>
        </div>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Bucket">
            <input className="input" value={bucket} onChange={(e) => setBucket(e.target.value)} placeholder="db-backups" required />
          </Field>
          <Field label={provider === "s3" ? "Region" : "Region (optional)"}>
            <input className="input" value={region} onChange={(e) => setRegion(e.target.value)} placeholder={provider === "s3" ? "eu-central-1" : "auto"} />
          </Field>
        </div>
        {provider !== "s3" ? (
          <Field label="Endpoint">
            <input
              className="input"
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              placeholder={provider === "r2" ? "https://<account-id>.r2.cloudflarestorage.com" : "https://minio.example.com"}
              required
            />
          </Field>
        ) : null}
        <Field label="Key prefix (optional)">
          <input className="input" value={prefix} onChange={(e) => setPrefix(e.target.value)} placeholder="db-manager" />
        </Field>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Access key ID">
            <input className="input" value={accessKey} onChange={(e) => setAccessKey(e.target.value)} autoComplete="off" required />
          </Field>
          <Field label={isEdit ? "Secret access key (leave blank to keep)" : "Secret access key"}>
            <input
              className="input"
              type="password"
              value={secretKey}
              onChange={(e) => setSecretKey(e.target.value)}
              autoComplete="new-password"
              required={!isEdit}
            />
          </Field>
        </div>
        <p className="muted text-sm">
          {isEdit
            ? "Leave the secret key empty to keep the current one. It is encrypted at rest and never returned by the API."
            : "The secret key is encrypted at rest and never returned by the API."}
        </p>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending}>
            {pending ? (isEdit ? "Saving…" : "Adding…") : isEdit ? "Save changes" : "Add destination"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
