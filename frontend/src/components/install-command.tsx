"use client";

import { CopyButton, buttonClass } from "@/components/ui";

/**
 * InstallCommand shows a one-time agent install command with the guidance
 * operators need to run it correctly.
 */
export function InstallCommand({ command }: { command: string }) {
  return (
    <div className="stack-sm">
      <p className="m-0 text-sm">
        Run this on the target Linux host as a user with <code>sudo</code> (requires <code>curl</code>).
        Do not put <code>sudo</code> before <code>curl</code> — it must come after the pipe, before <code>sh</code>.
      </p>
      <p className="m-0 text-sm text-muted">
        If the host is a VM or another machine, set <code>FLEETDOCK_PUBLIC_URL</code> to a URL that host can reach (not <code>localhost</code>).
      </p>
      <div className="code-block">{command}</div>
      <div className="flex justify-end">
        <CopyButton value={command} label="Copy command" className={buttonClass()} />
      </div>
    </div>
  );
}
