import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Fleetdock",
    short_name: "Fleetdock",
    description: "Manage your databases, backups and servers",
    start_url: "/",
    display: "standalone",
    background_color: "#09090b",
    theme_color: "#101820",
    icons: [
      { src: "/icon-192.png", type: "image/png", sizes: "192x192" },
      { src: "/icon-512.png", type: "image/png", sizes: "512x512" },
      { src: "/icon.svg", type: "image/svg+xml", sizes: "any" },
    ],
  };
}
