import { ImageResponse } from "next/og";

// iOS touch icons must be raster (PNG), so the docked-stack mark is drawn with
// flexbox divs and rendered to PNG at request time.
export const size = { width: 180, height: 180 };
export const contentType = "image/png";

function bar(width: number, opacity = 1) {
  return {
    width,
    height: 15,
    borderRadius: 8,
    background: "#ffffff",
    opacity,
  } as const;
}

export default function AppleIcon() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "#0f766e",
        }}
      >
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            gap: 11,
          }}
        >
          <div style={bar(54)} />
          <div style={bar(88)} />
          <div style={bar(66)} />
          <div style={bar(100, 0.6)} />
        </div>
      </div>
    ),
    { ...size },
  );
}
