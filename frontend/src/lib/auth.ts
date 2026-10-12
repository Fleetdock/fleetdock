// The dashboard's session lives in an httpOnly cookie set by the API on login,
// so script on the page — including injected script — cannot read it. Nothing
// about the session is stored in the browser by this code.

// CSRF_HEADER must accompany every state-changing request: the API refuses
// cookie-authenticated POST/PATCH/PUT/DELETE requests without it.
export const CSRF_HEADER = "X-Fleetdock-CSRF";

// Earlier releases kept the JWT in localStorage under this key.
const LEGACY_TOKEN_KEY = "fleetdock_token";

// clearLegacyToken removes a JWT an earlier release left in localStorage, so a
// token from before the cookie migration does not linger in the browser.
export function clearLegacyToken(): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(LEGACY_TOKEN_KEY);
  } catch {
    /* storage unavailable */
  }
}
