"use client";

import Link from "next/link";
import { useState } from "react";
import { Network } from "lucide-react";

import { Badge, Button, Callout, KeyValueList, SectionHeader, Skeleton } from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useConnectivity, useOperation } from "@/lib/hooks";

import { PublicEndpointCard } from "./public-endpoint-card";

const TERMINAL = ["succeeded", "failed", "canceled"];

/** connectionURL builds a credential-free URL apps can start from. */
export function connectionURL(protocol: string, host: string, port: number, database?: string): string {
  const scheme = protocol === "postgresql" ? "postgresql" : "mysql";
  const db = database ? `/${encodeURIComponent(database)}` : "";
  return `${scheme}://${host}:${port}${db}`;
}

const PROTOCOL_LABELS: Record<string, string> = {
  postgresql: "PostgreSQL",
  mysql: "MySQL",
  mariadb: "MariaDB",
};

export function ConnectivitySection({
  databaseId,
  databaseName,
  canWrite,
}: {
  databaseId: string;
  databaseName?: string;
  canWrite: boolean;
}) {
  const { data, isLoading, error, refetch } = useConnectivity(databaseId);
  const [operationId, setOperationId] = useState<string | null>(null);
  const op = useOperation(operationId ?? "");

  const header = (
    <SectionHeader
      title="Connection"
      icon={<Network size={17} />}
      description="How applications reach this database. Passwords come from application credentials below."
    />
  );

  if (isLoading) {
    return (
      <section className="section" aria-busy="true">
        {header}
        <div className="card panel" role="status" aria-label="Loading connection details">
          <div className="stack-sm">
            <Skeleton width="40%" />
            <Skeleton width="70%" />
            <Skeleton width="55%" />
          </div>
        </div>
      </section>
    );
  }

  // Previously any API failure returned null and the whole section vanished
  // with no explanation.
  if (error || !data) {
    return (
      <section className="section">
        {header}
        <Callout
          tone="danger"
          title="Couldn't load connection details"
          actions={
            <Button size="sm" onClick={() => void refetch()}>
              Try again
            </Button>
          }
        >
          {error ? friendlyError(error) : "Could not load connectivity for this database."}
        </Callout>
      </section>
    );
  }

  const opRunning = op.data && !TERMINAL.includes(op.data.status);
  const p = data.private;
  const privateAddress = `${p.host}:${p.port}`;
  const url = connectionURL(p.protocol, p.host, p.port, databaseName);

  return (
    <section className="section">
      {header}

      {operationId && opRunning ? (
        <Callout tone="info" role="status">
          Applying gateway changes ({op.data?.status}).{" "}
          <Link href={`/activity/${operationId}`} className="link">
            View operation
          </Link>
        </Callout>
      ) : null}
      {operationId && op.data?.status === "failed" ? (
        <Callout tone="danger">
          The gateway update failed.{" "}
          <Link href={`/activity/${operationId}`} className="link">
            View operation
          </Link>
        </Callout>
      ) : null}

      <div className="card panel">
        <h3 className="panel-title">
          <span>Private network</span>
          <Badge>{PROTOCOL_LABELS[p.protocol] ?? p.protocol}</Badge>
        </h3>
        <KeyValueList
          items={[
            { label: "Host", value: p.host, mono: true },
            { label: "Port", value: p.port, mono: true },
            { label: "Database", value: databaseName ?? "—", mono: true, hidden: !databaseName },
            { label: "Address", value: privateAddress, mono: true, copy: privateAddress },
            { label: "Connection URL", value: url, mono: true, copy: url },
          ]}
        />
      </div>

      <PublicEndpointCard databaseId={databaseId} data={data} canWrite={canWrite} onOperation={setOperationId} />
    </section>
  );
}
