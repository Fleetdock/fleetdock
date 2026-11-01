"use client";

import { Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";

import { IconButton } from "@/components/ui";

/** useTheme reads and flips the light/dark theme (the .dark class, remembered in localStorage). */
export function useTheme(): [boolean, () => void] {
  const [dark, setDark] = useState(false);
  useEffect(() => {
    setDark(document.documentElement.classList.contains("dark"));
  }, []);
  const toggle = () => {
    const next = !document.documentElement.classList.contains("dark");
    document.documentElement.classList.toggle("dark", next);
    try {
      localStorage.setItem("theme", next ? "dark" : "light");
    } catch {
      /* storage unavailable */
    }
    setDark(next);
  };
  return [dark, toggle];
}

/** ThemeToggle is the light/dark switch, for the top bar and full-screen workspaces. */
export function ThemeToggle({ size = "md" }: { size?: "sm" | "md" }) {
  const [dark, toggle] = useTheme();
  const icon = size === "md" ? 17 : 15;
  return (
    <IconButton label={dark ? "Switch to light theme" : "Switch to dark theme"} size={size} onClick={toggle}>
      {dark ? <Sun size={icon} aria-hidden /> : <Moon size={icon} aria-hidden />}
    </IconButton>
  );
}
