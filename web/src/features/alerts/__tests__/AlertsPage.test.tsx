/**
 * AlertsPage MSW-based integration tests.
 *
 * Default msw handlers (see src/test/mocks/server.ts) return:
 *   GET /alerts/rules    → { items: [{ id:"rule-1", name:"High CPU Alert", ... }] }
 *   GET /alerts/channels → { items: [] }
 *   GET /alerts/history  → { items: [] }
 *
 * Tests assert on real DOM output driven by those responses, plus one
 * end-to-end create-rule interaction that posts to POST /alerts/rules.
 */
import { describe, it, expect } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AlertsPage } from "../AlertsPage";
import { ToastProvider } from "@/components/Toast";
import { http, HttpResponse } from "msw";
import { server } from "@/test/mocks/server";

// AlertsPage uses useToast() which requires ToastProvider in the tree.
function renderAlerts() {
  return render(
    <ToastProvider>
      <AlertsPage />
    </ToastProvider>
  );
}

// Convenience: wait until the initial load completes and the rules tab is visible.
async function waitForRulesLoaded() {
  await waitFor(() => screen.getByText("High CPU Alert"));
}

// ── Tests ───────────────────────────────────────────────────────────────────

describe("AlertsPage (msw)", () => {
  it("renders the Alerts page heading", () => {
    renderAlerts();
    expect(
      screen.getByRole("heading", { name: /alerts/i })
    ).toBeInTheDocument();
  });

  it("renders the three tab buttons (rules / channels / history)", () => {
    renderAlerts();
    expect(screen.getByRole("button", { name: /rules/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /channels/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /history/i })).toBeInTheDocument();
  });

  it("renders rule name from GET /alerts/rules response", async () => {
    renderAlerts();
    await waitFor(() => {
      expect(screen.getByText("High CPU Alert")).toBeInTheDocument();
    });
  });

  it("renders severity badge from the rule data", async () => {
    renderAlerts();
    await waitForRulesLoaded();
    // Rule has severity: "warning" → Badge shows "warning"
    expect(screen.getByText("warning")).toBeInTheDocument();
  });

  it("renders the metric/operator/threshold detail line", async () => {
    renderAlerts();
    await waitForRulesLoaded();
    // Rendered as "cpu_pct gt 80 · window 300s · cooldown 300s"
    expect(screen.getByText(/cpu_pct/)).toBeInTheDocument();
  });

  it("shows '+ New rule' button on the rules tab after load", async () => {
    renderAlerts();
    await waitForRulesLoaded();
    expect(
      screen.getByRole("button", { name: /\+ new rule/i })
    ).toBeInTheDocument();
  });

  it("opens AlertRuleForm when '+ New rule' is clicked", async () => {
    renderAlerts();
    await waitForRulesLoaded();
    fireEvent.click(screen.getByRole("button", { name: /\+ new rule/i }));
    expect(
      screen.getByRole("heading", { name: /new alert rule/i })
    ).toBeInTheDocument();
  });

  it("cancels the rule form when Cancel is clicked", async () => {
    renderAlerts();
    await waitForRulesLoaded();
    fireEvent.click(screen.getByRole("button", { name: /\+ new rule/i }));
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(
      screen.queryByRole("heading", { name: /new alert rule/i })
    ).not.toBeInTheDocument();
  });

  it("POSTs to /alerts/rules and receives the created rule on form submit", async () => {
    // delay: null removes per-keystroke artificial delay (vitest 4 / rolldown runs
    // noticeably slower than vitest 3 / esbuild; without this the 5 s timeout fires).
    const user = userEvent.setup({ delay: null });
    let capturedBody: unknown;

    // Override the default POST handler to capture the request body.
    server.use(
      http.post("http://localhost/api/v1/alerts/rules", async ({ request }) => {
        capturedBody = await request.json();
        return HttpResponse.json(
          {
            id: "rule-created",
            name: "CPU Alert Test",
            metric: "cpu_pct",
            operator: "gt",
            threshold: 75,
            window_s: 300,
            severity: "warning",
            cooldown_s: 300,
            enabled: true,
            muted: false,
            created_at: 1_700_000_000_000,
            updated_at: 1_700_000_000_000,
          },
          { status: 201 }
        );
      })
    );

    renderAlerts();
    await waitForRulesLoaded();

    // Open the form
    await user.click(screen.getByRole("button", { name: /\+ new rule/i }));

    // Fill Name
    const nameInput = screen.getByPlaceholderText(/e\.g\. High CPU/i);
    await user.clear(nameInput);
    await user.type(nameInput, "CPU Alert Test");

    // Fill Threshold
    const thresholdInput = screen.getByPlaceholderText("0");
    await user.clear(thresholdInput);
    await user.type(thresholdInput, "75");

    // Submit
    await user.click(screen.getByRole("button", { name: /save rule/i }));

    // The POST must have been made with the correct payload
    await waitFor(() => {
      expect(capturedBody).toBeDefined();
    });
    expect((capturedBody as { name: string }).name).toBe("CPU Alert Test");
    expect((capturedBody as { threshold: number }).threshold).toBe(75);
    expect((capturedBody as { enabled: boolean }).enabled).toBe(true);
  });

  it("shows empty-state on Channels tab when API returns no channels", async () => {
    renderAlerts();
    await waitForRulesLoaded();
    fireEvent.click(screen.getByRole("button", { name: /channels/i }));
    await waitFor(() => {
      expect(
        screen.getByText(/no notification channels/i)
      ).toBeInTheDocument();
    });
  });

  it("shows empty-state on History tab when API returns no history", async () => {
    renderAlerts();
    await waitForRulesLoaded();
    fireEvent.click(screen.getByRole("button", { name: /history/i }));
    await waitFor(() => {
      expect(screen.getByText(/no alert history/i)).toBeInTheDocument();
    });
  });
});
