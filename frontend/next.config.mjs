/** @type {import('next').NextConfig} */

// Paths the Go control plane owns. Kept in sync with apiOwnedPath() in
// backend/internal/interfaces/httpapi/ui.go.
const apiPaths = [
  ["/v1/:path*", "/v1/:path*"],
  ["/agent/:path*", "/agent/:path*"],
  ["/healthz", "/healthz"],
  ["/readyz", "/readyz"],
  ["/docs", "/docs"],
  ["/openapi.yaml", "/openapi.yaml"],
  ["/install.sh", "/install.sh"],
];

const nextConfig = {
  reactStrictMode: true,
  // Self-contained server bundle: the runtime image needs only the node binary,
  // no node_modules and no npm.
  output: "standalone",
  poweredByHeader: false,

  // Development only. In production the Go binary is the front door and never
  // forwards an API path here, so these rewrites are unreachable; they exist so
  // `npm run dev` is same-origin too, matching how the app is actually served.
  async rewrites() {
    if (process.env.NODE_ENV !== "development") return [];
    const api = (process.env.FLEETDOCK_DEV_API_URL ?? "http://127.0.0.1:8080").replace(/\/+$/, "");
    return apiPaths.map(([source, target]) => ({
      source,
      destination: `${api}${target}`,
    }));
  },
};

export default nextConfig;
