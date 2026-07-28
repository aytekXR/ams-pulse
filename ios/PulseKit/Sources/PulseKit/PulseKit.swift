// PulseKit — Foundation-only API client and models for Pulse analytics.
//
// This file re-exports the public surface of the module. When importing PulseKit,
// all public types from the submodules are available at the top level.
//
// This package MUST build and test on Linux (no UIKit/SwiftUI/AVFoundation imports).
// All platform-specific UI code belongs in the sibling PulseApp target.

// Re-export all model types from the Models/ directory.
// These are organized by domain:
//
// - Error.swift: APIError, AnyCodableValue
// - Pagination.swift: PaginatedMeta
// - Live.swift: LiveOverview, ProtocolMix, AppOverview, NodeHealth, NodeRole, NodeStatus,
//               LiveStreamList, LiveStream, PublisherState
// - QoE.swift: QoeSummaryResponse, QoeTotals, BitrateBucket
// - Alerts.swift: AlertHistoryList, AlertHistoryEntry, AlertState, AlertSeverity, AlertScope
// - Fleet.swift: FleetNodeList, FleetNode
// - Anomalies.swift: AnomalyList, AnomalyFlag
// - Health.swift: HealthStatus, HealthState, HealthComponents, ComponentStatus
// - Auth.swift: AuthMeResponse, AuthMethod, User, UserRole
// - License.swift: LicenseInfo, LicenseTier, TierLimits

@_exported import struct Foundation.URL
@_exported import struct Foundation.UUID
@_exported import struct Foundation.Date
@_exported import struct Foundation.Data
