import { describe, expect, it } from "vitest";

import type { SSHTunnel } from "./types";

import { sshChanged, sshDraft, sshInput } from "./ssh-tunnel";

const saved: SSHTunnel = {
  host: "bastion.example.com",
  port: 22,
  username: "jump",
  auth_method: "password",
  host_key_fingerprint: "SHA256:abc",
};

describe("sshInput", () => {
  it("sends only the secret of the chosen method", () => {
    const d = { ...sshDraft(), enabled: true, host: " b ", username: " u ", auth: "key" as const, privateKey: "K", password: "P" };
    expect(sshInput(d)).toEqual({ host: "b", port: 22, username: "u", auth_method: "key", private_key: "K", passphrase: undefined });
  });

  it("omits empty secrets so stored ones are kept", () => {
    const d = { ...sshDraft(saved), enabled: true };
    expect(sshInput(d).password).toBeUndefined();
  });
});

describe("sshChanged", () => {
  it("is false for an untouched saved tunnel", () => {
    expect(sshChanged(sshDraft(saved), saved)).toBe(false);
  });

  it("detects removal, addition, edits and new secrets", () => {
    expect(sshChanged({ ...sshDraft(saved), enabled: false }, saved)).toBe(true);
    expect(sshChanged({ ...sshDraft(), enabled: true }, null)).toBe(true);
    expect(sshChanged({ ...sshDraft(saved), port: "2222" }, saved)).toBe(true);
    expect(sshChanged({ ...sshDraft(saved), password: "new" }, saved)).toBe(true);
    expect(sshChanged(sshDraft(), null)).toBe(false);
  });
});
