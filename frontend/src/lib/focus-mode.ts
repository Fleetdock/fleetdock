"use client";

// Focus mode hides Fleetdock's own chrome (the sidebar and the top bar) so a
// workspace can use the whole window: always on the ER diagram page, and on
// the data browser when the user turns it on (remembered in this browser).
// The browser's real full screen is a separate, optional step on top.

import { useCallback, useSyncExternalStore } from "react";

const DATA_KEY = "fd.focus.data";
const listeners = new Set<() => void>();
let dataFocus: boolean | null = null;

function readDataFocus(): boolean {
  if (dataFocus === null) {
    try {
      dataFocus = localStorage.getItem(DATA_KEY) === "1";
    } catch {
      dataFocus = false;
    }
  }
  return dataFocus;
}

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

export function setDataBrowserFocus(on: boolean) {
  dataFocus = on;
  try {
    localStorage.setItem(DATA_KEY, on ? "1" : "0");
  } catch {
    /* storage unavailable: it still works until the next reload */
  }
  for (const fn of listeners) fn();
}

/** useDataBrowserFocus is the data browser's full-screen switch. */
export function useDataBrowserFocus(): [boolean, () => void] {
  const on = useSyncExternalStore(subscribe, readDataFocus, () => false);
  const toggle = useCallback(() => setDataBrowserFocus(!readDataFocus()), []);
  return [on, toggle];
}

/** chromeHiddenFor reports whether a route renders without the sidebar and top bar. */
export function chromeHiddenFor(pathname: string, dataFocus: boolean): boolean {
  if (/^\/databases\/[^/]+\/diagram\/?$/.test(pathname)) return true;
  return pathname === "/data" && dataFocus;
}

/** useChromeHidden is what the dashboard layout asks. */
export function useChromeHidden(pathname: string): boolean {
  const [dataFocus] = useDataBrowserFocus();
  return chromeHiddenFor(pathname, dataFocus);
}

/** Browser full screen (the Fullscreen API), where the browser allows it. */
export function browserFullscreenSupported(): boolean {
  return typeof document !== "undefined" && Boolean(document.documentElement.requestFullscreen);
}

const noSubscribe = () => () => undefined;

/** useFullscreenSupported is false during server rendering and where the API is missing (iPhone Safari). */
export function useFullscreenSupported(): boolean {
  return useSyncExternalStore(noSubscribe, browserFullscreenSupported, () => false);
}

export function useBrowserFullscreen(): [boolean, () => void] {
  const on = useSyncExternalStore(
    (fn) => {
      document.addEventListener("fullscreenchange", fn);
      return () => document.removeEventListener("fullscreenchange", fn);
    },
    () => Boolean(document.fullscreenElement),
    () => false,
  );
  const toggle = useCallback(() => {
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => undefined);
    else void document.documentElement.requestFullscreen?.().catch(() => undefined);
  }, []);
  return [on, toggle];
}

/** isFocusShortcut matches Alt+Shift+F (by key position, so Option on a Mac works too). */
export function isFocusShortcut(e: Pick<KeyboardEvent, "altKey" | "shiftKey" | "ctrlKey" | "metaKey" | "code">): boolean {
  return e.altKey && e.shiftKey && !e.ctrlKey && !e.metaKey && e.code === "KeyF";
}
