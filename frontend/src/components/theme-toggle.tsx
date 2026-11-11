"use client";

import { Moon, Sun } from "lucide-react";

import { IconButton } from "@/components/ui";
import { useAppearance, useIsDark } from "@/lib/appearance";

/** ThemeToggle flips between the light and dark themes (top bar, full-screen workspaces). */
export function ThemeToggle({ size = "md" }: { size?: "sm" | "md" }) {
  const dark = useIsDark();
  const { setTheme } = useAppearance();
  const icon = size === "md" ? 17 : 15;
  return (
    <IconButton
      label={dark ? "Switch to light theme" : "Switch to dark theme"}
      size={size}
      onClick={() => setTheme(dark ? "light" : "dark")}
    >
      {dark ? <Sun size={icon} aria-hidden /> : <Moon size={icon} aria-hidden />}
    </IconButton>
  );
}
