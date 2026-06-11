# Handoffs — agent communication audit trail

Written only by ORCH-00 (work orders, decisions, gate reports) and by agents
returning completion/defect reports for their own work orders.

```
handoffs/
├── decisions.md          # append-only ruling log (ambiguities, waivers, contract-change approvals)
├── wave-0/
│   ├── WO-001.md         # work order (issued by ORCH-00)
│   └── WO-001-report.md  # completion report (returned by the assigned agent)
├── wave-1/
└── ...
```

Work order format and completion-report format: see `agents/README.md` §2.
First action of the build session: ORCH-00 creates `wave-0/WO-001.md` (CI activation,
assigned INFRA-01) per the wave plan in `agents/manifest.yaml`.
