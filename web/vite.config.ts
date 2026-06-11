import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev server proxies API + WebSocket to a locally running `pulse serve`.
// In production the built assets are served by the pulse binary itself
// (internal/api), so there is no separate web container to operate.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": "/src" },
  },
  server: {
    proxy: {
      "/api": "http://localhost:8090",
      "/live/ws": { target: "ws://localhost:8090", ws: true },
    },
  },
});
