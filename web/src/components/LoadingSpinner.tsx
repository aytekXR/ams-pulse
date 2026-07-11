import { useState, useEffect } from "react";

interface Props {
  size?: "sm" | "md" | "lg";
  label?: string;
}

/** Returns true when the OS prefers reduced motion. Guards against jsdom where matchMedia is undefined. */
function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

export function LoadingSpinner({ size = "md", label = "Loading…" }: Props) {
  const px = { sm: 16, md: 32, lg: 48 }[size];

  // Disable the spin animation when the user requests reduced motion (WCAG 2.1 SC 2.3.3).
  const [reducedMotion, setReducedMotion] = useState(prefersReducedMotion);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return;
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    const handler = (e: MediaQueryListEvent) => setReducedMotion(e.matches);
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, []);

  return (
    <div
      role="status"
      aria-label={label}
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 8,
        padding: "2rem",
      }}
    >
      <svg
        width={px}
        height={px}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        style={{ animation: reducedMotion ? "none" : "pulse-spin 0.8s linear infinite" }}
        aria-hidden
      >
        <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
      </svg>
      <span style={{ fontSize: 12, color: "var(--color-muted)" }}>{label}</span>
    </div>
  );
}
