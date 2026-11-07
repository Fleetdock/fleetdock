"use client";

import { useState } from "react";

import { Button, Callout, CopyButton, buttonClass } from "@/components/ui";
import { useVersion } from "@/lib/hooks";
import type { Server } from "@/lib/types";

/** agentUpdateCommand updates an enrolled agent in place, keeping its identity. */
export function agentUpdateCommand(origin: string): string {
  return `curl -sSL ${origin}/install.sh | sudo sh`;
}

/**
 * AgentUpdateNotice tells that a server's agent is older than this Fleetdock,
 * and gives the command that updates it on the machine.
 */
export function AgentUpdateNotice({ server }: { server: Server }) {
  const { data: version } = useVersion();
  const [show, setShow] = useState(false);
  if (server.agent_status !== "outdated" && server.agent_status !== "unsupported") return null;
  const unsupported = server.agent_status === "unsupported";
  const command = agentUpdateCommand(typeof window === "undefined" ? "" : window.location.origin);
  const fleetdock = version && version.version !== "dev" ? ` Fleetdock is ${version.version}.` : "";

  return (
    <Callout
      tone={unsupported ? "danger" : "warning"}
      title={unsupported ? "This agent is too old for this Fleetdock" : "Agent update available"}
      actions={
        show ? null : (
          <Button size="sm" onClick={() => setShow(true)}>
            How to update
          </Button>
        )
      }
    >
      <p className="m-0">
        The agent on this machine is version {server.agent_version}.{fleetdock}{" "}
        {unsupported
          ? `Agents older than ${version?.min_agent_version ?? "the supported version"} may fail to run tasks. Update it now.`
          : "Update it to get the latest fixes; it keeps working meanwhile."}
      </p>
      {show ? (
        <div className="stack-sm mt-2">
          <p className="m-0 text-sm">
            Run this on the machine. It downloads the agent from this Fleetdock, checks it and restarts the service; the
            server keeps its identity and nothing else changes.
          </p>
          <div className="code-block">{command}</div>
          <div className="flex justify-end">
            <CopyButton value={command} label="Copy command" className={buttonClass()} />
          </div>
        </div>
      ) : null}
    </Callout>
  );
}
