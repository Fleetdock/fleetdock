// Fleetdock brand mark — served from /public/logo.svg.
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <img
      src="/logo.svg"
      alt="Fleetdock"
      width={size}
      height={size}
      style={{ display: "block", flexShrink: 0, borderRadius: "25%" }}
    />
  );
}
