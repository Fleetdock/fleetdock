import { describe, expect, it } from "vitest";

import { chromeHiddenFor, isFocusShortcut } from "./focus-mode";

describe("focus mode", () => {
  it("hides the chrome on the diagram page and on the data browser when asked", () => {
    expect(chromeHiddenFor("/databases/abc/diagram", false)).toBe(true);
    expect(chromeHiddenFor("/databases/abc", true)).toBe(false);
    expect(chromeHiddenFor("/data", false)).toBe(false);
    expect(chromeHiddenFor("/data", true)).toBe(true);
    expect(chromeHiddenFor("/databases", true)).toBe(false);
  });

  it("matches Alt+Shift+F only", () => {
    const k = { altKey: true, shiftKey: true, ctrlKey: false, metaKey: false, code: "KeyF" };
    expect(isFocusShortcut(k)).toBe(true);
    expect(isFocusShortcut({ ...k, shiftKey: false })).toBe(false);
    expect(isFocusShortcut({ ...k, ctrlKey: true })).toBe(false);
    expect(isFocusShortcut({ ...k, code: "KeyG" })).toBe(false);
  });
});
