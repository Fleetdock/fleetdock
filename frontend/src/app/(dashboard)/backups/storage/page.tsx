"use client";

import { useErrorToast, useToast } from "@/components/toast";

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";

import { DataTable, type DataTableColumn } from "@/components/data-table";
import {
  Button,
  ErrorText,
  Field,
  IconButton,
  Input,
  Modal,
  PageHeader,
  Select,
} from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCan,
  useCreateDestination,
  useDeleteDestination,
  useDestinations,
  useTestDestination,
  useUpdateDestination,
} from "@/lib/hooks";
import type { Destination } from "@/lib/types";
import { useDataTable } from "@/lib/use-data-table";
import { HardDrive, Pencil, Plug, Plus, Trash2 } from "lucide-react";
import { useConfirm } from "@/components/confirm";
import { friendlyError } from "@/lib/errors";

const PROVIDER_LABEL: Record<string, string> = {
  s3: "AWS S3",
  r2: "Cloudflare R2",
  s3_compatible: "S3-compatible",
};

export default function DestinationsPage() {
  const { data, isLoading, error } = useDestinations();
  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Destination | null>(null);
  const confirm = useConfirm();
  const { push } = useToast();

  const del = useDeleteDestination();
  const toastError = useErrorToast();
  const test = useTestDestination();
  const can = useCan();
  const canWrite = can("destination:write");
  const [search, setSearch] = useState("");
  const table = useDataTable({
    items: data?.items,
    search: {
      query: search,
      match: (d, q) =>
        d.name.toLowerCase().includes(q) ||
        d.bucket.toLowerCase().includes(q) ||
        (d.endpoint ?? "").toLowerCase().includes(q),
    },
    sort: {
      key: "name",
      dir: "asc",
      compare: (a, b, key) =>
        String(a[key as keyof Destination] ?? "").localeCompare(
          String(b[key as keyof Destination] ?? ""),
        ),
    },
  });

  const onTest = useCallback(
    async (id: string, name: string) => {
      try {
        await test.mutateAsync(id);
        push("success", `${name}: the bucket is reachable and writable`);
      } catch (err) {
        push("error", `${name}: ${friendlyError(err, "the test failed")}`);
      }
    },
    [push, test],
  );

  const columns = useMemo(() => {
    const cols: DataTableColumn<Destination>[] = [
      {
        id: "name",
        header: "Name",
        sortable: true,
        sortKey: "name",
        className: "font-medium",
        render: (d) => d.name,
      },
      {
        id: "provider",
        header: "Provider",
        className: "muted",
        render: (d) => PROVIDER_LABEL[d.provider] ?? d.provider,
      },
      {
        id: "bucket",
        header: "Bucket",
        className: "muted",
        render: (d) => `${d.bucket}${d.prefix ? `/${d.prefix}` : ""}`,
      },
      {
        id: "endpoint",
        header: "Endpoint",
        className: "muted",
        render: (d) => d.endpoint || "AWS default",
      },
    ];
    if (canWrite) {
      cols.push({
        id: "actions",
        header: "Actions",
        align: "right",
        render: (d) => (
          <div className="flex items-center justify-end gap-2">
            <Button
              size="sm"
              icon={<Plug size={15} aria-hidden />}
              onClick={() => onTest(d.id, d.name)}
              disabled={test.isPending}
            >
              Test
            </Button>
            <IconButton
              variant="secondary"
              label={`Edit ${d.name}`}
              onClick={() => setEditTarget(d)}
            >
              <Pencil size={15} aria-hidden />
            </IconButton>
            <IconButton
              variant="danger"
              label={`Remove ${d.name}`}
              onClick={async () => {
                const ok = await confirm({
                  title: `Remove "${d.name}"?`,
                  message:
                    "Fleetdock stops using this bucket. Files already in it are not deleted, and existing backups keep their records.",
                  confirmLabel: "Remove storage",
                  danger: true,
                });
                if (ok) del.mutate(d.id, { onError: toastError("Failed to remove the storage") });
              }}
              disabled={del.isPending}
            >
              <Trash2 size={15} aria-hidden />
            </IconButton>
          </div>
        ),
      });
    }
    return cols;
  }, [toastError, canWrite, confirm, del, onTest, test.isPending]);

  return (
    <div>
      <PageHeader
        title="Backup storage"
        description="The S3-compatible buckets (AWS S3, Cloudflare R2, MinIO…) your backups are saved to."
        actions={
          canWrite ? (
            <Button
              variant="primary"
              icon={<Plus size={16} aria-hidden />}
              onClick={() => setAddOpen(true)}
            >
              Add storage
            </Button>
          ) : null
        }
      />

      <DataTable<Destination>
        columns={columns}
        rows={table.rows}
        rowKey={(d) => d.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load backup storage"
        emptyTitle="No backup storage yet"
        emptyHint="Add an S3 or Cloudflare R2 bucket to enable backups."
        emptyAction={
          canWrite ? (
            <Button
              variant="primary"
              icon={<HardDrive size={15} aria-hidden />}
              onClick={() => setAddOpen(true)}
            >
              Add storage
            </Button>
          ) : undefined
        }
        emptySearchTitle="Nothing matches your search"
        search={{
          value: search,
          onChange: (v) => {
            setSearch(v);
            table.setPage(1);
          },
          placeholder: "Search storage…",
        }}
        sort={{ key: table.sortKey, dir: table.sortDir, onSort: table.setSort }}
        pagination={{
          page: table.page,
          pageCount: table.pageCount,
          onPage: table.setPage,
        }}
      />

      <DestinationModal
        mode="create"
        open={addOpen}
        onClose={() => setAddOpen(false)}
      />
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
  | {
      mode: "create";
      destination?: undefined;
      open: boolean;
      onClose: () => void;
    }
  | {
      mode: "edit";
      destination: Destination | null;
      open: boolean;
      onClose: () => void;
    };

function DestinationModal({
  mode,
  destination,
  open,
  onClose,
}: DestinationModalProps) {
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
      setError(
        err instanceof ApiError
          ? err.message
          : `Failed to ${isEdit ? "save" : "add"} the storage`,
      );
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={isEdit ? "Edit backup storage" : "Add backup storage"}
    >
      <form onSubmit={onSubmit}>
        <div className="form-grid">
          <Field label="Name">
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="prod-backups"
              required
            />
          </Field>
          <Field label="Provider">
            <Select
              value={provider}
              onChange={(e) => setProvider(e.target.value)}
            >
              <option value="r2">Cloudflare R2</option>
              <option value="s3">AWS S3</option>
              <option value="s3_compatible">Other S3-compatible</option>
            </Select>
          </Field>
        </div>
        <div className="form-grid">
          <Field label="Bucket" help="The bucket must already exist. Fleetdock writes backups into it and never deletes the bucket itself.">
            <Input
              value={bucket}
              onChange={(e) => setBucket(e.target.value)}
              placeholder="db-backups"
              required
            />
          </Field>
          <Field label={provider === "s3" ? "Region" : "Region (optional)"}>
            <Input
              value={region}
              onChange={(e) => setRegion(e.target.value)}
              placeholder={provider === "s3" ? "eu-central-1" : "auto"}
            />
          </Field>
        </div>
        {provider !== "s3" ? (
          <Field label="Endpoint" help="The S3 API address. Leave empty for AWS; for R2 it looks like https://<account>.r2.cloudflarestorage.com, for MinIO your server’s URL.">
            <Input
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              placeholder={
                provider === "r2"
                  ? "https://<account-id>.r2.cloudflarestorage.com"
                  : "https://minio.example.com"
              }
              required
            />
          </Field>
        ) : null}
        <Field label="Folder (optional)" help="A path inside the bucket to keep backups in, e.g. fleetdock/prod. Useful when the bucket is shared.">
          <Input
            value={prefix}
            onChange={(e) => setPrefix(e.target.value)}
            placeholder="fleetdock"
          />
        </Field>
        <div className="form-grid">
          <Field label="Access key ID">
            <Input
              value={accessKey}
              onChange={(e) => setAccessKey(e.target.value)}
              autoComplete="off"
              required
            />
          </Field>
          <Field
            label={
              isEdit
                ? "Secret access key (leave blank to keep)"
                : "Secret access key"
            }
          >
            <Input
              type="password"
              value={secretKey}
              onChange={(e) => setSecretKey(e.target.value)}
              autoComplete="new-password"
              required={!isEdit}
            />
          </Field>
        </div>
        <p className="text-sm muted">
          {isEdit
            ? "Leave the secret key empty to keep the current one. It is encrypted at rest and never returned by the API."
            : "The secret key is encrypted at rest and never returned by the API."}
        </p>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={pending}>
            {pending
              ? isEdit
                ? "Saving…"
                : "Adding…"
              : isEdit
                ? "Save changes"
                : "Add storage"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
