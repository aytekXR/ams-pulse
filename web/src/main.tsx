import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { initTheme } from "./lib/theme";
import { initDensity } from "./lib/density";

// Stamp data-theme and data-density on <html> synchronously before React
// hydrates so the correct CSS custom-property set is active before first paint.
// No inline script in index.html — CSP and csp.spec.ts byte-equality unchanged.
initTheme();
initDensity();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
