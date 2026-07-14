/**
 * TierGate unit tests — non-vacuous suite.
 *
 * Each assertion is written to fail if the corresponding behaviour is broken:
 *
 * Structural (T1 fix):
 *   - Card wrapper div exists and carries expected layout/surface styles
 *
 * Text content (T2 fix):
 *   - Full prefix sentence "You are currently on the … plan." is rendered
 *   - tier label appears in <strong> inside <p>
 *
 * Element placement (T3 fix):
 *   - upgradeText is contained in the description <p>, not in another element
 *
 * Per-call-site combinations (T4 fix):
 *   - ReportsPage: heading + descriptionColor="var(--color-secondary)" + default maxWidth=400
 *   - AnomaliesPage: default descriptionColor="var(--color-secondary)" + default maxWidth=400
 *   - ProbesPage: default descriptionColor="var(--color-secondary)" + descriptionMaxWidth=420
 *   (the default was var(--color-muted) at baseline 2f53414; changed by the Wave 0
 *    WCAG fix — see TierGate.tsx header and WAVE-PLAN §3 C7)
 *
 * A11y (T5 fix + G-2):
 *   - Icon is wrapped in an aria-hidden span (component enforces AT suppression)
 *   - Upgrade link has className="tier-gate-cta" for focus-visible ring
 *
 * Existing contracts (preserved):
 *   - heading renders in an <h2>
 *   - tier label appears in <strong>
 *   - upgradeText appears in the description
 *   - upgrade link points to /settings#license
 *   - icon slot renders arbitrary ReactNode
 *   - descriptionMaxWidth default (400) and override (420) apply to paragraph
 *   - descriptionColor default and override wire through to paragraph style
 *   - different descriptionColor props produce different paragraph styles
 *
 * Note on jsdom CSS variable assertions: jsdom's cssstyle validator may reject
 * var(--token) as a typed property value (e.g. `color`), making
 * element.style.color unreliable for CSS-variable strings. We therefore
 * inspect the raw `style` attribute via getAttribute("style"), which reflects
 * whatever string React serialised onto the element.
 */
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TierGate } from "../TierGate";

// A minimal icon used across tests — testid lets us assert the slot renders.
const testIcon = (
  <svg
    data-testid="tier-gate-icon"
    width="48"
    height="48"
    viewBox="0 0 24 24"
    fill="none"
    stroke="var(--color-accent)"
    strokeWidth="1.5"
    aria-hidden
  >
    <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
  </svg>
);

// ─── Structural (T1 fix) ──────────────────────────────────────────────────────

describe("TierGate — card wrapper structure (T1 fix)", () => {
  it("renders a card wrapper div with surface background and border styles", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="free"
        upgradeText="Upgrade to Business to unlock usage reports."
      />,
    );
    const card = container.firstElementChild as HTMLElement;
    expect(card.tagName).toBe("DIV");
    const style = card.getAttribute("style") ?? "";
    // These assertions fail if the card div is removed or becomes a fragment.
    expect(style).toContain("var(--color-surface)");
    expect(style).toContain("var(--color-border)");
    expect(style).toContain("flex");
    expect(style).toContain("column");
    expect(style).toContain("center");
  });
});

// ─── Text content (T2 fix) ───────────────────────────────────────────────────

describe("TierGate — full prefix sentence (T2 fix)", () => {
  it("renders the complete prefix 'You are currently on the … plan.' in the paragraph", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="pro"
        upgradeText="Upgrade to Business to unlock usage reports."
      />,
    );
    const p = container.querySelector("p")!;
    // All three text nodes must be present — deleting any one causes this to fail.
    expect(p.textContent).toContain("You are currently on the");
    expect(p.textContent).toMatch(/plan\./);
    expect(p.textContent).toContain("pro");
  });

  it("renders the tier in a <strong> element inside the paragraph", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="business"
        upgradeText="Upgrade to Business."
      />,
    );
    const strong = container.querySelector("p strong")!;
    expect(strong).toBeInTheDocument();
    expect(strong.textContent).toBe("business");
  });
});

// ─── Element placement (T3 fix) ──────────────────────────────────────────────

describe("TierGate — upgradeText placement (T3 fix)", () => {
  it("renders upgradeText inside the description <p>, not elsewhere", () => {
    const upgradeText = "Upgrade to Enterprise to unlock anomaly detection.";
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Anomaly Detection requires Enterprise tier"
        tier="free"
        upgradeText={upgradeText}
      />,
    );
    const p = container.querySelector("p")!;
    expect(p).toBeInTheDocument();
    expect(p.textContent).toContain(upgradeText);
    // h2 must NOT contain the upgradeText — structural misplacement would fail here.
    const h2 = container.querySelector("h2")!;
    expect(h2.textContent).not.toContain("Upgrade to Enterprise");
  });
});

// ─── A11y: aria-hidden wrapper (G-2 / T5 fix) ────────────────────────────────

describe("TierGate — icon aria-hidden enforcement (G-2 / T5 fix)", () => {
  it("wraps the icon in an aria-hidden span so decorative SVG is always suppressed", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="free"
        upgradeText="Upgrade to Business to unlock usage reports."
      />,
    );
    // The component must render an aria-hidden wrapper regardless of whether
    // the caller set aria-hidden on the icon itself.
    const wrapper = container.querySelector('[aria-hidden="true"]');
    expect(wrapper).toBeInTheDocument();
    // The wrapper must be a SPAN — if the span is removed and {icon} is rendered
    // bare, querySelector returns the SVG (which also has aria-hidden="true"),
    // and svg.contains(svg) === true, making toContainElement pass vacuously.
    expect(wrapper!.tagName).toBe('SPAN');
    // The icon must be inside the aria-hidden wrapper.
    const icon = container.querySelector('[data-testid="tier-gate-icon"]');
    expect(wrapper).toContainElement(icon as HTMLElement);
  });

  it("adds className=tier-gate-cta to the upgrade link for focus-visible ring", () => {
    render(
      <TierGate
        icon={testIcon}
        heading="Anomaly Detection requires Enterprise tier"
        tier="free"
        upgradeText="Upgrade to Enterprise."
      />,
    );
    const link = screen.getByRole("link", { name: /upgrade license/i });
    expect(link).toHaveClass("tier-gate-cta");
  });
});

// ─── Per-call-site combinations (T4 fix) ─────────────────────────────────────

describe("TierGate — per-call-site prop combinations (T4 fix)", () => {
  it("ReportsPage: heading, descriptionColor=var(--color-secondary), default maxWidth=400", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="free"
        upgradeText="Upgrade to Business to unlock usage reports, scheduled exports, and tenant mapping."
        descriptionColor="var(--color-secondary)"
      />,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: /usage reports requires business tier/i }),
    ).toBeInTheDocument();
    const p = container.querySelector("p")!;
    expect(p.getAttribute("style")).toContain("var(--color-secondary)");
    expect(p.style.maxWidth).toBe("400px");
    expect(p.textContent).toContain(
      "Upgrade to Business to unlock usage reports, scheduled exports, and tenant mapping.",
    );
  });

  it("AnomaliesPage: default descriptionColor=var(--color-secondary), default maxWidth=400", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Anomaly Detection requires Enterprise tier"
        tier="free"
        upgradeText="Upgrade to Enterprise to unlock anomaly detection, baseline learning, and deviation alerts."
      />,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: /anomaly detection requires enterprise tier/i }),
    ).toBeInTheDocument();
    const p = container.querySelector("p")!;
    expect(p.getAttribute("style")).toContain("var(--color-secondary)");
    expect(p.style.maxWidth).toBe("400px");
    expect(p.textContent).toContain(
      "Upgrade to Enterprise to unlock anomaly detection, baseline learning, and deviation alerts.",
    );
  });

  it("ProbesPage: default descriptionColor=var(--color-secondary), descriptionMaxWidth=420", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Synthetic Probes requires Pro tier"
        tier="free"
        upgradeText="Upgrade to Pro or Enterprise to create synthetic stream probes and monitor playback health from outside your infrastructure."
        descriptionMaxWidth={420}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: /synthetic probes requires pro tier/i }),
    ).toBeInTheDocument();
    const p = container.querySelector("p")!;
    expect(p.style.maxWidth).toBe("420px");
    expect(p.getAttribute("style")).toContain("var(--color-secondary)");
    expect(p.textContent).toContain(
      "Upgrade to Pro or Enterprise to create synthetic stream probes",
    );
  });
});

// ─── Existing contracts (preserved) ──────────────────────────────────────────

describe("TierGate — tier-entitlement upsell gate (existing contracts)", () => {
  it("renders the feature heading in an h2", () => {
    render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="free"
        upgradeText="Upgrade to Business to unlock usage reports."
      />,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: /usage reports requires business tier/i }),
    ).toBeInTheDocument();
  });

  it("renders the current tier label in bold", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="pro"
        upgradeText="Upgrade to Business to unlock usage reports."
      />,
    );
    const strong = container.querySelector("strong");
    expect(strong).toBeInTheDocument();
    expect(strong).toHaveTextContent("pro");
  });

  it("renders the upgradeText in the description paragraph", () => {
    render(
      <TierGate
        icon={testIcon}
        heading="Anomaly Detection requires Enterprise tier"
        tier="free"
        upgradeText="Upgrade to Enterprise to unlock anomaly detection."
      />,
    );
    expect(
      screen.getByText(/upgrade to enterprise to unlock anomaly detection/i),
    ).toBeInTheDocument();
  });

  it("renders the Upgrade License link pointing to /settings#license", () => {
    render(
      <TierGate
        icon={testIcon}
        heading="Synthetic Probes requires Pro tier"
        tier="free"
        upgradeText="Upgrade to Pro to unlock probes."
      />,
    );
    const link = screen.getByRole("link", { name: /upgrade license/i });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute("href", "/settings#license");
  });

  it("renders the icon slot", () => {
    render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="free"
        upgradeText="Upgrade to Business to unlock usage reports."
      />,
    );
    expect(screen.getByTestId("tier-gate-icon")).toBeInTheDocument();
  });

  it("applies default descriptionMaxWidth of 400 to the paragraph", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Anomaly Detection requires Enterprise tier"
        tier="free"
        upgradeText="Upgrade to Enterprise to unlock anomaly detection."
      />,
    );
    const p = container.querySelector("p")!;
    // React appends "px" to numeric dimension values.
    expect(p.style.maxWidth).toBe("400px");
  });

  it("applies custom descriptionMaxWidth to the paragraph", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Synthetic Probes requires Pro tier"
        tier="free"
        upgradeText="Upgrade to Pro to unlock probes."
        descriptionMaxWidth={420}
      />,
    );
    const p = container.querySelector("p")!;
    expect(p.style.maxWidth).toBe("420px");
  });

  it("applies default descriptionColor var(--color-secondary) to the paragraph", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Anomaly Detection requires Enterprise tier"
        tier="free"
        upgradeText="Upgrade to Enterprise to unlock anomaly detection."
      />,
    );
    const p = container.querySelector("p")!;
    // Use the raw HTML attribute which React serialises directly.
    const styleAttr = p.getAttribute("style") ?? "";
    expect(styleAttr).toContain("var(--color-secondary)");
  });

  it("applies custom descriptionColor when provided", () => {
    const { container } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="free"
        upgradeText="Upgrade to Business to unlock usage reports."
        descriptionColor="var(--color-secondary)"
      />,
    );
    const p = container.querySelector("p")!;
    const styleAttr = p.getAttribute("style") ?? "";
    expect(styleAttr).toContain("var(--color-secondary)");
  });

  it("different descriptionColor props produce different paragraph styles", () => {
    // c1 uses the default (var(--color-secondary)); c2 passes an explicit override.
    const { container: c1 } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="free"
        upgradeText="Upgrade to Business."
      />,
    );
    const { container: c2 } = render(
      <TierGate
        icon={testIcon}
        heading="Usage Reports requires Business tier"
        tier="free"
        upgradeText="Upgrade to Business."
        descriptionColor="var(--color-muted)"
      />,
    );
    const style1 = c1.querySelector("p")!.getAttribute("style") ?? "";
    const style2 = c2.querySelector("p")!.getAttribute("style") ?? "";
    // The two paragraphs must carry different colour values.
    expect(style1).not.toBe(style2);
  });
});
