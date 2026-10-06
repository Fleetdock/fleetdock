import { describe, expect, it } from "vitest";

import { ApiError } from "./api";
import { fieldError, friendlyError } from "./errors";

describe("friendlyError", () => {
  it("keeps human messages from the server", () => {
    expect(friendlyError(new ApiError("database is locked; writes are not allowed", 409))).toBe(
      "Database is locked; writes are not allowed",
    );
  });
  it("explains transport and permission failures", () => {
    expect(friendlyError(new ApiError("insufficient permissions", 403))).toMatch(/permission/);
    expect(friendlyError(new ApiError("boom", 500))).toMatch(/unexpected error/);
    expect(friendlyError(new ApiError("", 503))).toMatch(/starting up/);
    expect(friendlyError(new TypeError("Failed to fetch"))).toMatch(/Can't reach/);
  });
  it("falls back for unknown errors", () => {
    expect(friendlyError("x", "Nope")).toBe("Nope");
  });
});

describe("fieldError", () => {
  it("matches only its own field", () => {
    const e = new ApiError("name is required", 422, "name");
    expect(fieldError(e, "name")).toBe("Name is required");
    expect(fieldError(e, "host")).toBeUndefined();
  });
});
