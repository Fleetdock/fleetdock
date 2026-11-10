"use client";

import { useCallback, useSyncExternalStore } from "react";

import { BORDERS_KEY, MOTION_KEY, STYLE_KEY, THEME_KEY } from "@/lib/appearance-script";

/**
 * Appearance preferences, kept in localStorage and applied to <html>: theme (.dark),
 * style (data-style="neutral"), borders (data-borders="none") and animations
 * (data-motion="reduced" | "off").
 */

export type ThemePref = "light" | "dark" | "system";
export type StylePref = "classic" | "neutral";
/** full: page changes fade and rise; reduced: a plain fade; off: no animations or transitions. */
export type MotionPref = "full" | "reduced" | "off";
export type Appearance = { theme: ThemePref; style: StylePref; borderless: boolean; motion: MotionPref };

const CHANGE_EVENT = "fleetdock:appearance";
const DEFAULT: Appearance = { theme: "system", style: "classic", borderless: false, motion: "full" };

function read(): Appearance {
  try {
    const t = localStorage.getItem(THEME_KEY);
    const m = localStorage.getItem(MOTION_KEY);
    return {
      theme: t === "light" || t === "dark" ? t : "system",
      style: localStorage.getItem(STYLE_KEY) === "neutral" ? "neutral" : "classic",
      borderless: localStorage.getItem(BORDERS_KEY) === "none",
      motion: m === "reduced" || m === "off" ? m : "full",
    };
  } catch {
    return DEFAULT;
  }
}

function systemDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

/** apply reflects the preferences onto <html>. */
function apply({ theme, style, borderless, motion }: Appearance) {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "system" ? systemDark() : theme === "dark");
  if (style === "neutral") root.setAttribute("data-style", "neutral");
  else root.removeAttribute("data-style");
  if (borderless) root.setAttribute("data-borders", "none");
  else root.removeAttribute("data-borders");
  if (motion === "full") root.removeAttribute("data-motion");
  else root.setAttribute("data-motion", motion);
}

function save(next: Appearance) {
  try {
    localStorage.setItem(THEME_KEY, next.theme);
    if (next.style === "neutral") localStorage.setItem(STYLE_KEY, "neutral");
    else localStorage.removeItem(STYLE_KEY);
    if (next.borderless) localStorage.setItem(BORDERS_KEY, "none");
    else localStorage.removeItem(BORDERS_KEY);
    if (next.motion === "full") localStorage.removeItem(MOTION_KEY);
    else localStorage.setItem(MOTION_KEY, next.motion);
  } catch {
    /* storage unavailable: still apply for this page view */
  }
  apply(next);
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

// useSyncExternalStore needs a stable snapshot; cache it by its serialised form.
let cached: Appearance = DEFAULT;
function snapshot(): Appearance {
  const next = read();
  if (
    next.theme !== cached.theme ||
    next.style !== cached.style ||
    next.borderless !== cached.borderless ||
    next.motion !== cached.motion
  ) {
    cached = next;
  }
  return cached;
}

function subscribe(onChange: () => void) {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  const onSystem = () => {
    if (read().theme === "system") apply(read());
    onChange();
  };
  // Another tab changed the preference: apply it here too.
  const onStorage = (e: StorageEvent) => {
    if (
      e.key === THEME_KEY ||
      e.key === STYLE_KEY ||
      e.key === BORDERS_KEY ||
      e.key === MOTION_KEY ||
      e.key === null
    ) {
      apply(read());
      onChange();
    }
  };
  window.addEventListener(CHANGE_EVENT, onChange);
  window.addEventListener("storage", onStorage);
  media.addEventListener("change", onSystem);
  return () => {
    window.removeEventListener(CHANGE_EVENT, onChange);
    window.removeEventListener("storage", onStorage);
    media.removeEventListener("change", onSystem);
  };
}

/** useAppearance reads and updates the appearance preferences. */
export function useAppearance() {
  const value = useSyncExternalStore(subscribe, snapshot, () => DEFAULT);
  const setTheme = useCallback((theme: ThemePref) => save({ ...read(), theme }), []);
  const setStyle = useCallback((style: StylePref) => save({ ...read(), style }), []);
  const setBorderless = useCallback((borderless: boolean) => save({ ...read(), borderless }), []);
  const setMotion = useCallback((motion: MotionPref) => save({ ...read(), motion }), []);
  return { ...value, setTheme, setStyle, setBorderless, setMotion };
}

/** useIsDark reports whether the dark theme is currently showing (resolves "system"). */
export function useIsDark(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => document.documentElement.classList.contains("dark"),
    () => false,
  );
}
