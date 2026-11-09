// Fleetdock brand assets, copied from /brand (the logo kit) into /public.
// Plain <img>: they are small static SVGs that next/image would not optimise.
/* eslint-disable @next/next/no-img-element */

/** LogoMark is the app icon: the turquoise F on a charcoal tile. For small sizes. */
export function LogoMark({ size = 28 }: { size?: number }) {
  return (
    <img
      src="/logo.svg"
      alt="Fleetdock"
      width={size}
      height={size}
      className="block shrink-0 rounded-[20%]"
    />
  );
}

/**
 * Wordmark is the full logo (mark and name). The light-background version has
 * a dark name, the dark-background one a light name. By default the theme
 * picks one; `on` fixes it for a surface that is always light or dark. The
 * logo kit asks for at least 140 px of width.
 */
export function Wordmark({
  height = 28,
  on = "theme",
  className = "",
}: {
  height?: number;
  on?: "theme" | "light" | "dark";
  className?: string;
}) {
  // The artwork is 1016.43 × 196.
  const width = Math.round((height * 1016.43) / 196);
  const img = (src: string, cls: string) => (
    <img src={src} alt="Fleetdock" width={width} height={height} className={`shrink-0 ${cls} ${className}`} />
  );
  if (on === "light") return img("/logo-light.svg", "block");
  if (on === "dark") return img("/logo-dark.svg", "block");
  return (
    <>
      {img("/logo-light.svg", "block dark:hidden")}
      {img("/logo-dark.svg", "hidden dark:block")}
    </>
  );
}
