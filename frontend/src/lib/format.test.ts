import { describe, expect, it } from "vitest";

import { formatBytes, formatDuration, formatRelative } from "./format";

describe("formatBytes", () => {
  it("uses binary units", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(5 * 1024 ** 3)).toBe("5.0 GB");
  });
  it("renders zero and unknown as the placeholder", () => {
    expect(formatBytes(0)).toBe("—");
    expect(formatBytes(null)).toBe("—");
    expect(formatBytes(0, "0 B")).toBe("0 B");
  });
});

describe("formatRelative", () => {
  const now = new Date("2026-10-03T12:00:00Z").getTime();
  const ago = (s: number) => now - s * 1000;
  it("reads naturally", () => {
    expect(formatRelative(ago(10), now)).toBe("just now");
    expect(formatRelative(ago(70), now)).toBe("1 min ago");
    expect(formatRelative(ago(5 * 60), now)).toBe("5 min ago");
    expect(formatRelative(ago(3 * 3600), now)).toBe("3 h ago");
    expect(formatRelative(ago(30 * 3600), now)).toBe("yesterday");
    expect(formatRelative(ago(4 * 86400), now)).toBe("4 days ago");
    expect(formatRelative(now + 5 * 60 * 1000, now)).toBe("in 5 min");
  });
  it("handles bad input", () => {
    expect(formatRelative("nope", now)).toBe("—");
  });
});

describe("formatDuration", () => {
  it("scales units", () => {
    expect(formatDuration(850)).toBe("850 ms");
    expect(formatDuration(1234)).toBe("1.2 s");
    expect(formatDuration(245_000)).toBe("4 min 5 s");
    expect(formatDuration(7_380_000)).toBe("2 h 3 min");
  });
});
