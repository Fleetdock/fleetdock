"use client";

import Link from "next/link";
import { useState } from "react";

import { CopyButton, Detail, ErrorText, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useConnectivity, useOperation } from "@/lib/hooks";

import { PublicEndpointCard } from "./public-endpoint-card";

const TERMINAL = ["succeeded", "failed", "canceled"];

export function ConnectivitySection({ databaseId, canWrite }: { databaseId: string; canWrite: boolean }) {
  const { data, isLoading, error } = useConnectivity(databaseId);
  const [operationId, setOperationId] = useState<string | null>(null);
  const op = useOperation(operationId ?? "");

  if (isLoading) {
    return (
      <div className="muted text-sm flex items-center gap-2">
        <Spinner /> Loading connectivity…
      </div>
    );
  }

  // Previously any API failure returned null and the whole section vanished
  // with no explanation.
  if (error || !data) {
    return (
      <section style={{ marginBottom: "1.25rem" }}>
        <h2 className="text-lg font-semibold" style={{ marginBottom: ".75rem" }}>
          Connectivity
        </h2>
        <ErrorText message={error instanceof ApiError ? error.message : "Could not load connectivity for this database."} />
      </section>
    );
  }

  const opRunning = op.data && !TERMINAL.includes(op.data.status);
  const privateAddress = `${data.private.host}:${data.private.port}`;

  return (
    <section style={{ marginBottom: "1.25rem" }}>
      <h2 className="text-lg font-semibold" style={{ marginBottom: ".75rem" }}>
        Connectivity
      </h2>

      {operationId && opRunning ? (
        <div className="card" style={{ padding: ".7rem", marginBottom: ".75rem" }}>
          Applying gateway changes ({op.data?.status}).{" "}
          <Link href={`/operations/${operationId}`}>View operation</Link>
        </div>
      ) : null}
      {operationId && op.data?.status === "failed" ? (
        <div style={{ marginBottom: ".75rem" }}>
          <ErrorText message="The gateway update failed." />
          <Link href={`/operations/${operationId}`}>View operation</Link>
        </div>
      ) : null}

      <div className="card" style={{ padding: "1.1rem", marginBottom: ".75rem" }}>
        <h3 className="font-medium" style={{ marginBottom: ".5rem" }}>
          Private access
        </h3>
        <Detail label="Address" value={privateAddress} mono />
        <Detail label="Protocol" value={data.private.protocol} />
        <div style={{ marginTop: ".75rem" }}>
          <CopyButton value={privateAddress} label="Copy address" />
        </div>
      </div>

      <PublicEndpointCard
        databaseId={databaseId}
        data={data}
        canWrite={canWrite}
        onOperation={setOperationId}
      />
    </section>
  );
}
