"use client";

import { Callout } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { friendlyError } from "@/lib/errors";

/**
 * FormError shows a failure that doesn't belong to one field. Errors the API
 * tied to a field are shown next to that field instead (see fieldError).
 */
export function FormError({ error, fields, fallback }: { error: unknown; fields: string[]; fallback: string }) {
  if (!error) return null;
  if (error instanceof ApiError && error.field && fields.includes(error.field)) return null;
  return (
    <Callout tone="danger" className="mb-3">
      {friendlyError(error, fallback)}
    </Callout>
  );
}
