"use client";

import { Callout, KeyValueList, Time } from "@/components/ui";
import { DATA_ACCESS_LABELS } from "@/lib/data-access";
import { engineLabel } from "@/lib/engines";
import type { Instance } from "@/lib/types";

import { SSHTunnelDetail } from "./ssh-tunnel-detail";

/** kindLabel describes, in plain words, how Fleetdock reaches a database server. */
export function kindLabel(i: Instance): string {
  if (i.provisioned) return "Created by Fleetdock";
  return i.kind === "external" ? "Connected remotely" : "On your server";
}

/** instanceAddress is where the database server listens, in plain words. */
export function instanceAddress(i: Instance, serverName?: string): string {
  return i.kind === "external"
    ? `${i.host}:${i.port}${i.ssh_tunnel ? ` via ${i.ssh_tunnel.host}` : ""}`
    : `${serverName ?? "server"}, port ${i.port}`;
}

export const TLS_LABEL: Record<string, string> = {
  disable: "Off",
  prefer: "When available",
  require: "Always",
  "verify-full": "Always, verified",
};

/**
 * InstanceOverview is the details card of a database server: how it is
 * reached, which logins Fleetdock uses, and the result of the last check.
 */
export function InstanceOverview({
  instance,
  address,
  canWrite,
}: {
  instance: Instance;
  address: string;
  canWrite: boolean;
}) {
  const failed = instance.health?.error && instance.health.status !== "healthy";
  return (
    <section className="card panel section" aria-labelledby="instance-details-title">
      <h2 id="instance-details-title" className="panel-title">
        Details
      </h2>
      <KeyValueList
        items={[
          { label: "Engine", value: `${engineLabel(instance.engine)} ${instance.engine_version}` },
          { label: "Address", value: address, mono: true },
          { label: "Admin user", value: instance.username ?? "—", mono: !!instance.username },
          { label: "Encryption", value: TLS_LABEL[instance.tls_mode ?? "prefer"] },
          {
            label: "Data browsing as",
            value:
              instance.data_access === "login" && instance.data_username
                ? instance.data_username
                : DATA_ACCESS_LABELS[instance.data_access ?? "admin"],
          },
          {
            label: "SSH tunnel",
            value: instance.ssh_tunnel ? <SSHTunnelDetail instance={instance} canWrite={canWrite} /> : null,
            hidden: !instance.ssh_tunnel,
          },
          { label: "Added", value: <Time value={instance.created_at} /> },
          {
            label: "Last checked",
            value: <LastChecked instance={instance} />,
            hidden: !instance.has_credentials,
          },
        ]}
      />
      {failed ? (
        <Callout tone="danger" role="status" title="Last check failed">
          {instance.health!.error}
        </Callout>
      ) : null}
    </section>
  );
}

/**
 * LastChecked shows when Fleetdock last reached the server (it checks every
 * minute on its own; "Check now" in the header checks immediately).
 */
function LastChecked({ instance }: { instance: Instance }) {
  return <>{instance.health ? <Time value={instance.health.checked_at} /> : "not yet"}</>;
}
