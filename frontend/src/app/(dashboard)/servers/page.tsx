"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { ChevronRight, Plus, Server } from "lucide-react";
import { useConfirm } from "@/components/confirm";
import { DataTable } from "@/components/data-table";
import { InstallCommand } from "@/components/install-command";
import { useToast } from "@/components/toast";
import { Button, ButtonLink, ErrorText, Field, Modal, PageHeader, SectionHeader, StatusBadge, Time } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { friendlyError } from "@/lib/errors";
import {
  LIST_PAGE_SIZE,
  useAgentTokens,
  useCan,
  useCreateAgentToken,
  useCreateServer,
  useRevokeRegistrationToken,
  useServers,
} from "@/lib/hooks";
import type { AgentToken, Server as ServerType } from "@/lib/types";

export default function ServersPage() {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useServers(search, page);
  const [registerOpen, setRegisterOpen] = useState(false);
  const [connectOpen, setConnectOpen] = useState(false);
  const can = useCan();

  return (
    <div>
      <PageHeader
        title="Servers"
        description="Hosts running the Fleetdock agent."
        actions={
          can("server:write") ? (
            <>
              <Button icon={<Plus size={16} aria-hidden />} onClick={() => setRegisterOpen(true)}>
                Register manually
              </Button>
              <Button variant="primary" icon={<Server size={16} aria-hidden />} onClick={() => setConnectOpen(true)}>
                Connect server
              </Button>
            </>
          ) : undefined
        }
      />

      <DataTable<ServerType>
        columns={[
          {
            id: "name",
            header: "Name",
            className: "font-medium",
            render: (s) => (
              <Link href={`/servers/${s.id}`} className="link-plain">
                {s.name}
              </Link>
            ),
          },
          { id: "hostname", header: "Hostname", className: "muted", render: (s) => s.hostname },
          { id: "status", header: "Status", render: (s) => <StatusBadge status={s.status} /> },
          { id: "agent", header: "Agent", className: "muted", hideOnMobile: true, render: (s) => s.agent_version ?? "—" },
          {
            id: "heartbeat",
            header: "Last heartbeat",
            className: "muted",
            hideOnMobile: true,
            render: (s) => (s.last_heartbeat_at ? new Date(s.last_heartbeat_at).toLocaleString() : "never"),
          },
          {
            id: "actions",
            header: "",
            align: "right",
            render: (s) => (
              <ButtonLink href={`/servers/${s.id}`} variant="ghost" size="sm" aria-label={`Open ${s.name}`}>
                Open <ChevronRight size={15} aria-hidden />
              </ButtonLink>
            ),
          },
        ]}
        rows={data?.items ?? []}
        rowKey={(s) => s.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load servers"
        emptyTitle="No servers yet"
        emptyHint="Get a one-command installer for your host, run it there, and the server shows up here."
        emptyAction={
          can("server:write") ? (
            <Button variant="primary" icon={<Server size={16} aria-hidden />} onClick={() => setConnectOpen(true)}>
              Connect server
            </Button>
          ) : undefined
        }
        emptySearchTitle="No servers match your search"
        search={{
          value: search,
          onChange: (v) => {
            setSearch(v);
            setPage(1);
          },
          placeholder: "Search name or hostname…",
        }}
        pagination={{
          page,
          pageCount: Math.max(1, Math.ceil((data?.pagination.total ?? 0) / LIST_PAGE_SIZE)),
          onPage: setPage,
        }}
      />

      {can("server:write") ? <PendingInstallCommands /> : null}

      <RegisterServerModal open={registerOpen} onClose={() => setRegisterOpen(false)} />
      <ConnectServerModal open={connectOpen} onClose={() => setConnectOpen(false)} />
    </div>
  );
}

/**
 * PendingInstallCommands lists install commands that have not been used yet,
 * so one that was shared by mistake can be revoked before it enrolls a host.
 */
function PendingInstallCommands() {
  const { data, dataUpdatedAt } = useAgentTokens();
  const revoke = useRevokeRegistrationToken();
  const confirm = useConfirm();
  const { push } = useToast();
  // Expiry is judged as of the last fetch, which keeps rendering pure.
  const pending = (data?.items ?? []).filter(
    (t) => !t.used_at && new Date(t.expires_at).getTime() > dataUpdatedAt,
  );
  if (pending.length === 0) return null;

  async function onRevoke(t: AgentToken) {
    const ok = await confirm({
      title: `Revoke install command "${t.name || "install"}"?`,
      message: "Anyone holding this command can no longer use it to enroll a server.",
      confirmLabel: "Revoke",
      danger: true,
    });
    if (!ok) return;
    try {
      await revoke.mutateAsync(t.id);
      push("success", "Install command revoked");
    } catch (err) {
      push("error", friendlyError(err, "Failed to revoke the install command"));
    }
  }

  return (
    <section className="section mt-6">
      <SectionHeader
        title="Unused install commands"
        description="Revoke any command that was shared by mistake before it is used to enroll a host."
      />
      <DataTable<AgentToken>
        columns={[
          { id: "name", header: "Label", className: "font-medium", render: (t) => t.name || "install" },
          {
            id: "purpose",
            header: "Purpose",
            className: "muted",
            render: (t) => (t.server_id ? "Re-enroll existing server" : "Connect new server"),
          },
          { id: "expires", header: "Expires", className: "muted", render: (t) => <Time value={t.expires_at} /> },
          {
            id: "actions",
            header: "",
            align: "right",
            render: (t) => (
              <Button variant="danger" size="sm" disabled={revoke.isPending} onClick={() => onRevoke(t)}>
                Revoke
              </Button>
            ),
          },
        ]}
        rows={pending}
        rowKey={(t) => t.id}
        emptyTitle="No unused install commands"
      />
    </section>
  );
}

function ConnectServerModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const create = useCreateAgentToken();
  const [name, setName] = useState("");
  const [command, setCommand] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const res = await create.mutateAsync({ name: name || "install" });
      setCommand(res.install_command);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to generate install command");
    }
  }

  function close() {
    setName("");
    setCommand(null);
    setError(null);
    onClose();
  }

  return (
    <Modal open={open} onClose={close} title="Connect a server">
      {command ? (
        <div>
          <InstallCommand command={command} />
          <div className="modal-actions">
            <Button variant="primary" onClick={close}>
              Done
            </Button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="Label (optional)" hint="Generates a single-use registration token and install script.">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="prod-01" />
          </Field>
          <ErrorText message={error ?? undefined} />
          <div className="modal-actions">
            <Button onClick={close}>Cancel</Button>
            <Button type="submit" variant="primary" loading={create.isPending}>
              {create.isPending ? "Generating…" : "Generate command"}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}

function RegisterServerModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const create = useCreateServer();
  const [name, setName] = useState("");
  const [hostname, setHostname] = useState("");
  const [tags, setTags] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({
        name,
        hostname,
        tags: tags ? tags.split(",").map((t) => t.trim()).filter(Boolean) : undefined,
      });
      setName("");
      setHostname("");
      setTags("");
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to register server");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Register server manually">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="prod-01" required />
        </Field>
        <Field label="Hostname">
          <input className="input" value={hostname} onChange={(e) => setHostname(e.target.value)} placeholder="db1.example.com" required />
        </Field>
        <Field label="Tags (comma-separated)">
          <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="production,eu" />
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={create.isPending}>
            {create.isPending ? "Registering…" : "Register"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
