import { ImageResponse } from "next/og";

// 1200x630 social card, generated at request time. Next auto-wires this as
// og:image and twitter:image. Uses @vercel/og's bundled default font.
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";
export const alt = "Fleetdock — MariaDB control plane";

function bar(width: number, opacity = 1) {
  return {
    width,
    height: 9,
    borderRadius: 5,
    background: "#ffffff",
    opacity,
  } as const;
}

export default function OpengraphImage() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "center",
          padding: 96,
          background: "#09090b",
          color: "#fafafa",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 32 }}>
          <div
            style={{
              width: 120,
              height: 120,
              borderRadius: 28,
              background: "#0d9488",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                gap: 8,
              }}
            >
              <div style={bar(38)} />
              <div style={bar(62)} />
              <div style={bar(46)} />
              <div style={bar(70, 0.6)} />
            </div>
          </div>
          <div style={{ fontSize: 84, fontWeight: 700, letterSpacing: -2 }}>
            Fleetdock
          </div>
        </div>
        <div style={{ marginTop: 44, fontSize: 40, color: "#a1a1aa" }}>
          MariaDB control plane
        </div>
      </div>
    ),
    { ...size },
  );
}
