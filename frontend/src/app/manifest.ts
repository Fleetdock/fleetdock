import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Fleetdock",
    short_name: "Fleetdock",
    description: "Manage your databases, backups and servers",
    start_url: "/",
    display: "standalone",
    background_color: "#09090b",
    theme_color: "#0f766e",
    icons: [
      { src: "/icon.svg", type: "image/svg+xml", sizes: "any" },
      { src: "/apple-icon", type: "image/png", sizes: "180x180" },
    ],
  };
}
