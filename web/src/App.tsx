/**
 * App shell: router + layout. Feature pages live under src/features/, one
 * folder per PRD feature; FE-01 fills them in per phase. Routes mirror the
 * left-nav information architecture:
 *
 *   /            → live ops dashboard (F1)        — Phase 1
 *   /analytics   → historical audience (F2)       — Phase 1 core, Phase 2 full
 *   /qoe         → viewer QoE (F3)                — Phase 2
 *   /ingest      → publisher/ingest health (F4)   — Phase 2
 *   /alerts      → rules, channels, history (F5)  — Phase 1
 *   /reports     → usage & billing (F6)           — Phase 2
 *   /fleet       → cluster nodes (F7)             — Phase 2
 *   /settings    → sources, tokens, license, users
 */
export function App() {
  // TODO(FE-01): router, layout chrome (nav, header, theme), auth gate.
  return <main>Pulse — skeleton build</main>;
}
