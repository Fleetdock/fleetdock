// Fleetdock brand mark: a fleet of server bars "docked" on a pier line.
// Theme-aware — the tile follows --primary and the glyph --primary-fg, so it
// adapts to light/dark automatically. Reused in the sidebar and login page.
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      role="img"
      aria-label="Fleetdock"
      style={{ display: "block", flexShrink: 0 }}
    >
      <rect width="32" height="32" rx="8" fill="var(--primary)" />
      <g fill="var(--primary-fg)">
        <rect x="11" y="7" width="10" height="3" rx="1.5" />
        <rect x="8" y="12" width="16" height="3" rx="1.5" />
        <rect x="10" y="17" width="12" height="3" rx="1.5" />
        <rect x="6" y="23" width="20" height="2.5" rx="1.25" opacity=".6" />
      </g>
    </svg>
  );
}
