"use client";

import { FileQuestion, RefreshCw, WifiOff } from "lucide-react";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api";
import { friendlyError } from "@/lib/errors";

import { EmptyState } from "./base";
import { Button, ButtonLink } from "./controls";

/** isNotFound reports whether a query failed because the item does not exist. */
export function isNotFound(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404;
}

/**
 * QueryError renders a failed query: "not found" when the item is gone (with
 * a way back), otherwise the actual problem with a Retry button. Detail
 * pages used to show "not found" for every failure, including outages.
 */
export function QueryError({
  error,
  onRetry,
  what = "item",
  backHref,
  backLabel,
}: {
  error: unknown;
  onRetry?: () => void;
  /** What failed to load, e.g. "server". */
  what?: string;
  backHref?: string;
  backLabel?: string;
}) {
  if (isNotFound(error)) {
    return (
      <EmptyState
        icon={<FileQuestion size={22} />}
        title={`This ${what} doesn't exist`}
        hint="It may have been removed, or the link is wrong."
        action={
          backHref ? (
            <ButtonLink href={backHref} variant="primary">
              {backLabel ?? "Go back"}
            </ButtonLink>
          ) : null
        }
      />
    );
  }
  return (
    <EmptyState
      icon={<WifiOff size={22} />}
      title={`Couldn't load this ${what}`}
      hint={friendlyError(error)}
      action={
        <>
          {onRetry ? (
            <Button
              variant="primary"
              icon={<RefreshCw size={14} />}
              onClick={onRetry}
            >
              Try again
            </Button>
          ) : null}
          {backHref ? (
            <ButtonLink href={backHref}>{backLabel ?? "Go back"}</ButtonLink>
          ) : null}
        </>
      }
    />
  );
}

/**
 * QueryState picks loading / error / content for one query so every page
 * handles the three the same way.
 */
export function QueryState<T>({
  query,
  loading,
  what,
  backHref,
  backLabel,
  children,
}: {
  query: {
    data: T | undefined;
    isLoading?: boolean;
    isPending?: boolean;
    isError: boolean;
    error: unknown;
    refetch: () => unknown;
  };
  loading: ReactNode;
  what?: string;
  backHref?: string;
  backLabel?: string;
  children: (data: T) => ReactNode;
}) {
  if (query.data !== undefined) return <>{children(query.data)}</>;
  if (query.isError) {
    return (
      <QueryError
        error={query.error}
        onRetry={() => void query.refetch()}
        what={what}
        backHref={backHref}
        backLabel={backLabel}
      />
    );
  }
  return <>{loading}</>;
}
