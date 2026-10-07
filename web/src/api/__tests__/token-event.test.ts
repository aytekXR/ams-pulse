/**
 * D14: signing in must tell the rest of the app. LicenseProvider mounts before the
 * sign-in form, so it re-fetches the license on the pulse:auth:token event that
 * setToken emits — without it the sidebar tier label stayed empty until a reload.
 */
import { describe, it, expect, afterEach } from "vitest";
import { setToken, clearToken, getToken } from "../client";

describe("setToken", () => {
  afterEach(() => clearToken());

  it("stores the token and emits pulse:auth:token", () => {
    let events = 0;
    const onToken = () => {
      events += 1;
    };
    window.addEventListener("pulse:auth:token", onToken);
    try {
      setToken("plt_example");
      expect(getToken()).toBe("plt_example");
      expect(events).toBe(1);
    } finally {
      window.removeEventListener("pulse:auth:token", onToken);
    }
  });
});
