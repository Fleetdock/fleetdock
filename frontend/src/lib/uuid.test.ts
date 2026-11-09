import { afterEach, describe, expect, it, vi } from "vitest";

import { randomUUID } from "./uuid";

const V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe("randomUUID", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("uses crypto.randomUUID when available", () => {
    expect(randomUUID()).toMatch(V4);
  });

  it("falls back to getRandomValues outside secure contexts", () => {
    const real = globalThis.crypto;
    vi.stubGlobal("crypto", { getRandomValues: real.getRandomValues.bind(real) });
    const ids = new Set(Array.from({ length: 50 }, () => randomUUID()));
    expect(ids.size).toBe(50);
    for (const id of ids) expect(id).toMatch(V4);
  });
});
