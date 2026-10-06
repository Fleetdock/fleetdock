import { ApiError } from "./api";

/**
 * friendlyError turns an API failure into a sentence a person can act on.
 * Messages the server wrote for people (validation, conflicts, engine errors)
 * are kept; transport-level failures get a plain explanation.
 */
export function friendlyError(err: unknown, fallback = "Something went wrong. Please try again."): string {
  if (err instanceof ApiError) {
    switch (err.status) {
      case 0:
        return "Can't reach Fleetdock. Check your connection and try again.";
      case 401:
        return "Your session has ended. Please sign in again.";
      case 403:
        return err.message && err.message !== "insufficient permissions"
          ? err.message
          : "You don't have permission to do this. Ask an administrator for access.";
      case 404:
        return err.message && err.message !== "Not Found" ? err.message : "This item no longer exists — it may have been removed.";
      case 413:
        return "That file is too large.";
      case 429:
        return "Too many attempts. Wait a minute and try again.";
      case 502:
      case 503:
      case 504:
        return "Fleetdock is starting up or temporarily unavailable. Try again in a moment.";
    }
    if (err.status >= 500) return "Fleetdock hit an unexpected error. Try again; if it keeps happening, check the server logs.";
    if (err.message) return capitalize(err.message);
  }
  if (err instanceof TypeError) return "Can't reach Fleetdock. Check your connection and try again.";
  return fallback;
}

/** fieldError returns the error message when it belongs to the given form field. */
export function fieldError(err: unknown, field: string): string | undefined {
  return err instanceof ApiError && err.field === field ? capitalize(err.message) : undefined;
}

function capitalize(s: string): string {
  return s ? s[0].toUpperCase() + s.slice(1) : s;
}
