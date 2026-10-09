import { describe, expect, it } from "vitest";

import { dataAccessChanged, dataAccessDraft, dataAccessInput } from "./data-access";

describe("dataAccessDraft", () => {
  it("defaults to the admin login", () => {
    expect(dataAccessDraft()).toEqual({ mode: "admin", username: "", password: "" });
  });
});

describe("dataAccessInput", () => {
  it("sends only the mode unless it is a dedicated login", () => {
    expect(dataAccessInput({ mode: "managed", username: "x", password: "y" })).toEqual({ data_access: "managed" });
  });

  it("trims the username and omits an empty password to keep the stored one", () => {
    expect(dataAccessInput({ mode: "login", username: " reader ", password: "" })).toEqual({
      data_access: "login",
      data_username: "reader",
      data_password: undefined,
    });
  });
});

describe("dataAccessChanged", () => {
  const saved = { data_access: "login" as const, data_username: "reader" };

  it("is false for an untouched saved login", () => {
    expect(dataAccessChanged(dataAccessDraft(saved), saved)).toBe(false);
  });

  it("detects a new password, username or mode", () => {
    expect(dataAccessChanged({ ...dataAccessDraft(saved), password: "pw" }, saved)).toBe(true);
    expect(dataAccessChanged({ ...dataAccessDraft(saved), username: "other" }, saved)).toBe(true);
    expect(dataAccessChanged({ ...dataAccessDraft(saved), mode: "admin" }, saved)).toBe(true);
  });

  it("treats a missing setting as the admin login", () => {
    expect(dataAccessChanged(dataAccessDraft(), {})).toBe(false);
  });
});
