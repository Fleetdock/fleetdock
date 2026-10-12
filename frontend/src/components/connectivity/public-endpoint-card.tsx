"use client";

import { useState } from "react";
import { Globe } from "lucide-react";

import { ConfirmModal, ErrorText, Field, KeyValueList, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useDisablePublicAccess, useEnablePublicAccess } from "@/lib/hooks";
import type { Connectivity, EndpointView, GatewayInfo } from "@/lib/types";

import { ALLOW_ANYWHERE, AllowlistEditor, parseCIDRs } from "./allowlist-editor";

export function PublicEndpointCard({
  databaseId,
  data,
  canWrite,
  onOperation,
}: {
  databaseId: string;
  data: Connectivity;
  canWrite: boolean;
  onOperation: (id: string | null) => void;
}) {
  return (
    <div className="card panel">
      <h3 className="panel-title">
        <span>
          <Globe size={16} aria-hidden /> Public access
        </span>
        {data.public ? <StatusBadge status={data.public.status} /> : <span className="badge">Off</span>}
      </h3>

      {data.public ? (
        <EnabledPublicAccess
          databaseId={databaseId}
          endpoint={data.public}
          gateway={data.gateway}
          canWrite={canWrite}
          onOperation={onOperation}
        />
      ) : (
        <EnableForm databaseId={databaseId} gateway={data.gateway} canWrite={canWrite} onOperation={onOperation} />
      )}
    </div>
  );
}

function EnabledPublicAccess({
  databaseId,
  endpoint,
  gateway,
  canWrite,
  onOperation,
}: {
  databaseId: string;
  endpoint: EndpointView;
  gateway: GatewayInfo;
  canWrite: boolean;
  onOperation: (id: string | null) => void;
}) {
  const disable = useDisablePublicAccess(databaseId);
  const [editing, setEditing] = useState(false);
  const [confirmDisable, setConfirmDisable] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const cidrs = endpoint.allowed_cidrs ?? [];
  const address = `${endpoint.host}:${endpoint.port}`;
  // Rejected connections with no successful sessions is the signature of an
  // allowlist that does not match where clients actually connect from.
  const allowlistLikelyWrong = endpoint.denied_connections > 0 && endpoint.sessions_total === 0;
  // The issued URLs carry sslmode=require, so they cannot connect at all until
  // the database is fixed or the endpoint is recreated with a relaxed mode.
  const tlsUnsatisfiable = endpoint.tls_mode === "required" && endpoint.tls_status === "unsupported";

  async function onDisable() {
    setError(null);
    try {
      const res = await disable.mutateAsync();
      onOperation(res.operation_id || null);
      setConfirmDisable(false);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Failed to disable public access");
      setConfirmDisable(false);
    }
  }

  return (
    <>
      <KeyValueList
        items={[
          { label: "Address", value: address, mono: true, copy: address },
          { label: "TLS", value: tlsSummary(endpoint.tls_mode, endpoint.tls_status) },
          {
            label: "Allowed networks",
            value: cidrs.length ? (
              <span className="flex gap-1" style={{ flexWrap: "wrap" }}>
                {cidrs.map((c) => (
                  <code key={c} className={c === ALLOW_ANYWHERE ? "badge badge-amber" : "badge"}>
                    {c}
                  </code>
                ))}
              </span>
            ) : (
              "None"
            ),
          },
          {
            label: "Rejected",
            value: `${endpoint.denied_connections} connection${endpoint.denied_connections === 1 ? "" : "s"} blocked by the allowlist`,
            hidden: endpoint.denied_connections === 0,
          },
        ]}
      />

      {endpoint.last_error ? <ErrorText message={endpoint.last_error} /> : null}

      {tlsUnsatisfiable ? (
        <div className="callout callout-danger" role="alert">
          <strong>This database does not accept TLS, but the endpoint requires it.</strong>
          <p className="text-sm" style={{ margin: ".25rem 0 0" }}>
            Connection URLs are issued with <code>sslmode=require</code> and will fail until you enable TLS on the
            database. Fleetdock will not quietly downgrade the requirement, because that would drop encryption without
            you asking.
          </p>
        </div>
      ) : null}

      {allowlistLikelyWrong ? (
        <div className="callout callout-warning" role="status">
          <strong>Connections are being rejected by the allowlist.</strong>
          <p className="text-sm" style={{ margin: ".25rem 0 0" }}>
            The gateway saw {endpoint.denied_connections} connection
            {endpoint.denied_connections === 1 ? "" : "s"} from addresses that are not in your allowed networks, and no
            successful sessions.
          </p>
          <SourceIPHint gateway={gateway} />
        </div>
      ) : null}

      {endpoint.status === "pending" ? (
        <div className="callout callout-info" role="status">
          Waiting for the gateway to program this endpoint.
        </div>
      ) : null}

      <ErrorText message={error ?? undefined} />

      {canWrite ? (
        <div className="panel-actions">
          <button className="btn btn-sm" onClick={() => setEditing(true)}>
            Edit allowed networks
          </button>
          <button
            className="btn btn-sm btn-danger"
            disabled={disable.isPending || endpoint.status === "disabling"}
            onClick={() => setConfirmDisable(true)}
          >
            {endpoint.status === "disabling" ? "Disabling…" : "Disable public access"}
          </button>
        </div>
      ) : null}

      {editing ? (
        <AllowlistEditor databaseId={databaseId} current={cidrs} onClose={() => setEditing(false)} />
      ) : null}

      <ConfirmModal
        open={confirmDisable}
        danger
        busy={disable.isPending}
        title="Disable public access"
        confirmLabel="Disable"
        message={
          <>
            Applications using <code>{address}</code> will stop connecting. Re-enabling later assigns a new port, so
            every connection URL will need updating.
          </>
        }
        onConfirm={onDisable}
        onCancel={() => setConfirmDisable(false)}
      />
    </>
  );
}

function EnableForm({
  databaseId,
  gateway,
  canWrite,
  onOperation,
}: {
  databaseId: string;
  gateway: GatewayInfo;
  canWrite: boolean;
  onOperation: (id: string | null) => void;
}) {
  const enable = useEnablePublicAccess(databaseId);
  const [cidrInput, setCidrInput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [confirmAnywhere, setConfirmAnywhere] = useState(false);

  if (!gateway.enabled) {
    return (
      <p className="muted text-sm">
        This deployment has no gateway configured, so public access is unavailable. Set{" "}
        <code>FLEETDOCK_GATEWAY_ENABLED</code> and run the gateway service to enable it.
      </p>
    );
  }
  if (gateway.unavailable_reason) {
    return <p className="muted text-sm">{capitalize(gateway.unavailable_reason)}.</p>;
  }
  if (!canWrite) {
    return <p className="muted text-sm">Public access is disabled.</p>;
  }

  async function onEnable() {
    setError(null);
    const cidrs = parseCIDRs(cidrInput);

    // An empty allowlist previously defaulted to 0.0.0.0/0 without any
    // confirmation, exposing the database to the internet by accident.
    if (cidrs.length === 0) {
      setError("Enter at least one network. To allow any address, enter 0.0.0.0/0 explicitly.");
      return;
    }
    if (cidrs.includes(ALLOW_ANYWHERE) && !confirmAnywhere) {
      setConfirmAnywhere(true);
      return;
    }

    try {
      const res = await enable.mutateAsync({ allowed_cidrs: cidrs, tls_mode: "required" });
      onOperation(res.operation_id);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Failed to enable public access");
    }
  }

  return (
    <>
      <Field label="Allowed networks (CIDR or IP, comma-separated)">
        <input
          className="input"
          value={cidrInput}
          onChange={(e) => {
            setCidrInput(e.target.value);
            setConfirmAnywhere(false);
          }}
          placeholder="203.0.113.7, 10.0.0.0/8"
        />
      </Field>
      <SourceIPHint gateway={gateway} />

      {confirmAnywhere ? (
        <div className="callout callout-warning" role="alert">
          <strong>0.0.0.0/0 allows connections from any address on the internet.</strong> Press Enable again to confirm.
        </div>
      ) : null}

      <ErrorText message={error ?? undefined} />

      <button className="btn btn-primary btn-sm" style={{ marginTop: ".5rem" }} disabled={enable.isPending} onClick={onEnable}>
        {enable.isPending ? "Enabling…" : "Enable public access"}
      </button>
    </>
  );
}

/**
 * SourceIPHint explains that the allowlist matches the address the gateway
 * observes, which is not the client's address behind NAT or a load balancer.
 * Getting this wrong silently rejects every connection.
 */
function SourceIPHint({ gateway }: { gateway: GatewayInfo }) {
  const host = gateway.public_host;
  return (
    <p className="muted text-sm" style={{ marginTop: ".5rem" }}>
      The gateway matches the source address it observes. Behind NAT (including Docker Desktop) or a load balancer
      without PROXY protocol, that is the proxy&apos;s address, not your client&apos;s.
      {gateway.diag_port && host ? (
        <>
          {" "}
          Check it from the machine that will connect:{" "}
          <code>
            curl http://{host}:{gateway.diag_port}
          </code>
        </>
      ) : null}
    </p>
  );
}

const TLS_MODES: Record<string, string> = {
  required: "Required",
  preferred: "Preferred",
  disabled: "Off",
};

const TLS_OBSERVED: Record<string, string> = {
  required: "database enforces TLS",
  preferred: "database supports TLS",
  disabled: "TLS is turned off on the database",
  unsupported: "database does not support TLS",
  misconfigured: "database TLS is misconfigured",
  unknown: "not checked yet",
};

/** tlsSummary turns the endpoint's TLS mode and observed status into a sentence. */
function tlsSummary(mode: string, observed: string): string {
  return `${TLS_MODES[mode] ?? mode} · ${TLS_OBSERVED[observed] ?? observed}`;
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
