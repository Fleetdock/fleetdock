"use client";

import { useState } from "react";

import { Button } from "@/components/ui";

import { ApiError } from "@/lib/api";

import { Download } from "lucide-react";

// ExportButton triggers a CSV download, showing a spinner while streaming and
// surfacing any error inline (downloads have no natural error surface otherwise).
export function ExportButton({
  label,
  run,
}: {
  label: string;
  run: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onClick() {
    setBusy(true);
    setError(null);
    try {
      await run();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Export failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <span className="flex items-center gap-2">
      {error ? (
        <span className="text-sm text-danger" title={error} role="alert">
          Export failed
        </span>
      ) : null}
      <Button
        size="sm"
        icon={<Download size={15} aria-hidden />}
        loading={busy}
        onClick={onClick}
      >
        {label}
      </Button>
    </span>
  );
}
