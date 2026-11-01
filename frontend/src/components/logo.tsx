// Fleetdock brand mark — served from /public/logo.svg.
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <img
      src="/logo.svg"
      alt="Fleetdock"
      width={size}
      height={size}
      className="block shrink-0 rounded-[25%]"
    />
  );
}
