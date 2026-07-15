/**
 * Audit Log page tests (D-102 Phase 2):
 *  - loading spinner, table rows, empty state, error banner
 *  - cursor "Load more" appends the next page (and is absent when next_cursor is null)
 *  - actor fallback to short token id when no actor_name
 *  - design-token source-read pins (no bare hex, no --color-muted, no var() fallbacks)
 */
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import type { AuditEntry } from "@/lib/api/types";

vi.mock("@/api/client", () => ({
  adminApi: {
    listAuditLog: vi.fn(),
  },
  ApiError: class ApiError extends Error {
    status: number;
    body: unknown;
    constructor(status: number, body: { message?: string }) {
      super(body.message ?? `HTTP ${status}`);
      this.status = status;
      this.body = body;
      this.name = "ApiError";
    }
  },
}));

import { adminApi } from "@/api/client";
import { AuditLogPage } from "../AuditLogPage";

const entryA: AuditEntry = {
  id: "a1",
  ts: Date.now() - 60_000,
  actor_name: "admin-token",
  actor_token_id: "tok-abc12345",
  actor_user_id: "",
  action: "alert_rule.create",
  object_type: "alert_rule",
  object_id: "rule-1",
  remote_addr: "10.0.0.5",
  detail: { name: "cpu", metric: "cert_expiry" },
};

const entryB: AuditEntry = {
  id: "a2",
  ts: Date.now() - 120_000,
  actor_name: "",
  actor_token_id: "tok-def67890",
  action: "user.delete",
  object_type: "user",
  object_id: "user-9",
  remote_addr: "10.0.0.6",
};

describe("AuditLogPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows a loading spinner while the first page loads", () => {
    vi.mocked(adminApi.listAuditLog).mockReturnValue(new Promise(() => {}));
    const { unmount } = render(<AuditLogPage />);
    expect(screen.getByRole("status")).toBeInTheDocument();
    unmount();
  });

  it("renders a table of entries (action, actor, object)", async () => {
    vi.mocked(adminApi.listAuditLog).mockResolvedValue({
      items: [entryA, entryB],
      meta: { next_cursor: null },
    });
    render(<AuditLogPage />);
    await waitFor(() => {
      expect(screen.getByRole("table", { name: /audit log/i })).toBeInTheDocument();
    });
    expect(screen.getByText("alert_rule.create")).toBeInTheDocument();
    expect(screen.getByText("user.delete")).toBeInTheDocument();
    expect(screen.getByText("admin-token")).toBeInTheDocument();
    expect(screen.getByText("rule-1")).toBeInTheDocument();
  });

  it("falls back to a short token id when actor_name is empty", async () => {
    vi.mocked(adminApi.listAuditLog).mockResolvedValue({
      items: [entryB],
      meta: { next_cursor: null },
    });
    render(<AuditLogPage />);
    await waitFor(() => {
      expect(screen.getByText(/token tok-def6/i)).toBeInTheDocument();
    });
  });

  it("shows the empty state when there are no entries", async () => {
    vi.mocked(adminApi.listAuditLog).mockResolvedValue({
      items: [],
      meta: { next_cursor: null },
    });
    render(<AuditLogPage />);
    await waitFor(() => {
      expect(screen.getByText(/no audit entries yet/i)).toBeInTheDocument();
    });
  });

  it("shows an error banner on fetch failure", async () => {
    vi.mocked(adminApi.listAuditLog).mockRejectedValue(new Error("network error"));
    render(<AuditLogPage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });

  it("shows 'Load more' when next_cursor is present and appends the next page", async () => {
    vi.mocked(adminApi.listAuditLog)
      .mockResolvedValueOnce({ items: [entryA], meta: { next_cursor: "cursor-1" } })
      .mockResolvedValueOnce({ items: [entryB], meta: { next_cursor: null } });
    render(<AuditLogPage />);

    await waitFor(() => {
      expect(screen.getByText("alert_rule.create")).toBeInTheDocument();
    });
    const loadMore = screen.getByRole("button", { name: /load more/i });
    expect(loadMore).toBeInTheDocument();

    fireEvent.click(loadMore);

    // Second page appended: both rows visible, and the second call used the cursor.
    await waitFor(() => {
      expect(screen.getByText("user.delete")).toBeInTheDocument();
    });
    expect(screen.getByText("alert_rule.create")).toBeInTheDocument();
    expect(vi.mocked(adminApi.listAuditLog).mock.calls[1][0]).toMatchObject({ cursor: "cursor-1" });
    // next_cursor now null → the button is gone.
    expect(screen.queryByRole("button", { name: /load more/i })).not.toBeInTheDocument();
  });

  it("does not show 'Load more' when next_cursor is null", async () => {
    vi.mocked(adminApi.listAuditLog).mockResolvedValue({
      items: [entryA],
      meta: { next_cursor: null },
    });
    render(<AuditLogPage />);
    await waitFor(() => {
      expect(screen.getByText("alert_rule.create")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /load more/i })).not.toBeInTheDocument();
  });
});

// ─── Design-token source-read pins ────────────────────────────────────────────

describe("AuditLogPage — design token pins (source-read)", () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const src = readFileSync(resolve(here, "../AuditLogPage.tsx"), "utf-8");

  it("no bare hex literals remain in source (RULE 10)", () => {
    expect(src).not.toMatch(/#[0-9A-Fa-f]{6}/);
  });

  it("no var() fallbacks remain in CSS properties (RULE 4)", () => {
    expect(src).not.toMatch(/var\(--color-[^)]+,\s*#[0-9A-Fa-f]{6}/);
  });

  it("--color-muted is not used in any text color property (RULE 5)", () => {
    expect(src).not.toContain("var(--color-muted)");
  });
});
