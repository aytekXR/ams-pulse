import Foundation
import Testing
@testable import PulseKit

// MARK: - Model Decoding Tests

/// Tests for decoding API response models from JSON.
/// Fixtures are derived from the OpenAPI examples in `contracts/openapi/pulse-api.yaml`.
@Suite("Models")
struct ModelDecodingTests {

    // MARK: - LiveOverview

    @Test("LiveOverview decodes required fields")
    func test_LiveOverview_decodesRequiredFields() throws {
        let json = """
        {
            "ts": 1700000000000,
            "total_viewers": 1234,
            "total_publishers": 56,
            "protocol_mix": {},
            "apps": [],
            "nodes": []
        }
        """

        let data = json.data(using: .utf8)!
        let overview = try JSONDecoder().decode(LiveOverview.self, from: data)

        #expect(overview.ts == 1700000000000)
        #expect(overview.totalViewers == 1234)
        #expect(overview.totalPublishers == 56)
        #expect(overview.apps.isEmpty)
        #expect(overview.nodes.isEmpty)
    }

    @Test("LiveOverview decodes optional fields when present")
    func test_LiveOverview_decodesOptionalFieldsWhenPresent() throws {
        let json = """
        {
            "ts": 1700000000000,
            "total_viewers": 1234,
            "total_publishers": 56,
            "protocol_mix": {
                "webrtc": 500,
                "hls": 600,
                "rtmp": 100,
                "dash": 34,
                "other": 0
            },
            "apps": [
                {"app": "live", "viewers": 1000, "publishers": 50, "streams": 25}
            ],
            "nodes": [
                {
                    "node_id": "node-1",
                    "role": "origin",
                    "status": "up",
                    "last_seen": 1700000000000,
                    "cpu_pct": 45.5,
                    "mem_pct": 60.0,
                    "version": "3.0.3"
                }
            ]
        }
        """

        let data = json.data(using: .utf8)!
        let overview = try JSONDecoder().decode(LiveOverview.self, from: data)

        #expect(overview.protocolMix.webrtc == 500)
        #expect(overview.protocolMix.hls == 600)
        #expect(overview.apps.count == 1)
        #expect(overview.apps[0].app == "live")
        #expect(overview.nodes.count == 1)
        #expect(overview.nodes[0].nodeId == "node-1")
        #expect(overview.nodes[0].cpuPct == 45.5)
    }

    // MARK: - ProtocolMix

    @Test("ProtocolMix defaults to zero for missing fields")
    func test_ProtocolMix_defaultsToZeroForMissingFields() throws {
        let json = """
        {
            "webrtc": 100
        }
        """

        let data = json.data(using: .utf8)!
        let mix = try JSONDecoder().decode(ProtocolMix.self, from: data)

        #expect(mix.webrtc == 100)
        #expect(mix.hls == 0)
        #expect(mix.rtmp == 0)
        #expect(mix.dash == 0)
        #expect(mix.other == 0)
    }

    @Test("ProtocolMix all fields zero")
    func test_ProtocolMix_allFieldsZero() throws {
        let json = "{}"

        let data = json.data(using: .utf8)!
        let mix = try JSONDecoder().decode(ProtocolMix.self, from: data)

        #expect(mix.webrtc == 0)
        #expect(mix.hls == 0)
        #expect(mix.rtmp == 0)
        #expect(mix.dash == 0)
        #expect(mix.other == 0)
    }

    // MARK: - LiveStream

    @Test("LiveStream decodes all fields")
    func test_LiveStream_decodesAllFields() throws {
        let json = """
        {
            "stream_id": "test-stream",
            "app": "live",
            "node_id": "node-1",
            "tenant": "acme-corp",
            "viewers": 500,
            "publisher_state": "publishing",
            "health_score": 95.5,
            "protocol_mix": {"webrtc": 300, "hls": 200},
            "bitrate_kbps": 2500.5,
            "started_at": 1699996400000,
            "viewer_rtt_ms": 45.5,
            "viewer_jitter_ms": 5.2,
            "viewer_loss_pct": 0.5
        }
        """

        let data = json.data(using: .utf8)!
        let stream = try JSONDecoder().decode(LiveStream.self, from: data)

        #expect(stream.streamId == "test-stream")
        #expect(stream.app == "live")
        #expect(stream.nodeId == "node-1")
        #expect(stream.tenant == "acme-corp")
        #expect(stream.viewers == 500)
        #expect(stream.publisherState == .publishing)
        #expect(stream.healthScore == 95.5)
        #expect(stream.protocolMix?.webrtc == 300)
        #expect(stream.bitrateKbps == 2500.5)
        #expect(stream.startedAt == 1699996400000)
        #expect(stream.viewerRttMs == 45.5)
        #expect(stream.viewerJitterMs == 5.2)
        #expect(stream.viewerLossPct == 0.5)
    }

    @Test("LiveStream max viewer count")
    func test_LiveStream_maxViewerCount() throws {
        let json = """
        {
            "stream_id": "test",
            "app": "live",
            "viewers": \(Int.max),
            "publisher_state": "publishing",
            "health_score": 100.0
        }
        """

        let data = json.data(using: .utf8)!
        let stream = try JSONDecoder().decode(LiveStream.self, from: data)
        #expect(stream.viewers == Int.max)
    }

    // MARK: - QoeTotals

    @Test("QoeTotals decodes snake_case fields")
    func test_QoeTotals_decodesSnakeCaseFields() throws {
        let json = """
        {
            "startup_p50_ms": 850.0,
            "startup_p95_ms": 2100.0,
            "rebuffer_ratio": 0.015,
            "error_rate": 0.005
        }
        """

        let data = json.data(using: .utf8)!
        let totals = try JSONDecoder().decode(QoeTotals.self, from: data)

        #expect(totals.startupP50Ms == 850.0)
        #expect(totals.startupP95Ms == 2100.0)
        #expect(totals.rebufferRatio == 0.015)
        #expect(totals.errorRate == 0.005)
    }

    // MARK: - BitrateBucket

    @Test("BitrateBucket optional P95")
    func test_BitrateBucket_optionalP95() throws {
        let json = """
        {
            "ts": 1700000000000,
            "bitrate_kbps_p50": 2500.0
        }
        """

        let data = json.data(using: .utf8)!
        let bucket = try JSONDecoder().decode(BitrateBucket.self, from: data)

        #expect(bucket.ts == 1700000000000)
        #expect(bucket.bitrateKbpsP50 == 2500.0)
        #expect(bucket.bitrateKbpsP95 == nil)
    }

    // MARK: - AlertHistoryEntry

    @Test("AlertHistoryEntry decodes all states")
    func test_AlertHistoryEntry_decodesAllStates() throws {
        let states = ["firing", "resolved", "delivery_failure"]
        let expected: [AlertState] = [.firing, .resolved, .deliveryFailure]

        for (stateStr, expectedState) in zip(states, expected) {
            let json = """
            {
                "id": "alert-1",
                "rule_id": "rule-1",
                "state": "\(stateStr)",
                "severity": "info",
                "ts": 1700000000000,
                "metric": "test",
                "value": 1.0,
                "threshold": 0.5
            }
            """

            let data = json.data(using: .utf8)!
            let entry = try JSONDecoder().decode(AlertHistoryEntry.self, from: data)
            #expect(entry.state == expectedState)
        }
    }

    @Test("AlertHistoryEntry unicode metric name")
    func test_AlertHistoryEntry_unicodeMetricName() throws {
        let json = """
        {
            "id": "alert-1",
            "rule_id": "rule-1",
            "state": "firing",
            "severity": "warning",
            "ts": 1700000000000,
            "metric": "viewer_count_\u{1F4CA}",
            "value": 100.0,
            "threshold": 50.0
        }
        """

        let data = json.data(using: .utf8)!
        let entry = try JSONDecoder().decode(AlertHistoryEntry.self, from: data)
        #expect(entry.metric.contains("\u{1F4CA}"))
    }

    // MARK: - FleetNode

    @Test("FleetNode decodes all fields")
    func test_FleetNode_decodesAllFields() throws {
        let json = """
        {
            "node_id": "node-1",
            "role": "origin",
            "status": "up",
            "last_seen": 1700000000000,
            "version": "3.0.3",
            "cpu_pct": 45.5,
            "mem_pct": 60.0,
            "net_in_mbps": 100.5,
            "net_out_mbps": 500.0,
            "os_name": "Ubuntu 22.04",
            "os_arch": "amd64",
            "java_version": "17.0.1",
            "processor_count": 8
        }
        """

        let data = json.data(using: .utf8)!
        let node = try JSONDecoder().decode(FleetNode.self, from: data)

        #expect(node.nodeId == "node-1")
        #expect(node.role == .origin)
        #expect(node.status == .up)
        #expect(node.lastSeen == 1700000000000)
        #expect(node.version == "3.0.3")
        #expect(node.cpuPct == 45.5)
        #expect(node.memPct == 60.0)
        #expect(node.netInMbps == 100.5)
        #expect(node.netOutMbps == 500.0)
        #expect(node.osName == "Ubuntu 22.04")
        #expect(node.osArch == "amd64")
        #expect(node.javaVersion == "17.0.1")
        #expect(node.processorCount == 8)
    }

    @Test("FleetNode unicode version")
    func test_FleetNode_unicodeVersion() throws {
        let json = """
        {
            "node_id": "node-1",
            "role": "standalone",
            "status": "up",
            "last_seen": 1700000000000,
            "version": "3.0.3-\u{03B1}"
        }
        """

        let data = json.data(using: .utf8)!
        let node = try JSONDecoder().decode(FleetNode.self, from: data)
        #expect(node.version?.contains("\u{03B1}") == true)
    }

    // MARK: - AnomalyFlag

    @Test("AnomalyFlag decodes scope")
    func test_AnomalyFlag_decodesScope() throws {
        let json = """
        {
            "id": "anomaly-001",
            "metric": "viewer_count",
            "scope": {
                "app": "live",
                "stream_id": "stream-001"
            },
            "observed": 50.0,
            "expected": 500.0,
            "sigma": 4.5,
            "ts": 1700000000000
        }
        """

        let data = json.data(using: .utf8)!
        let anomaly = try JSONDecoder().decode(AnomalyFlag.self, from: data)

        #expect(anomaly.id == "anomaly-001")
        #expect(anomaly.metric == "viewer_count")
        #expect(anomaly.scope.app == "live")
        #expect(anomaly.scope.streamId == "stream-001")
        #expect(anomaly.observed == 50.0)
        #expect(anomaly.expected == 500.0)
        #expect(anomaly.sigma == 4.5)
        #expect(anomaly.ts == 1700000000000)
    }

    // MARK: - HealthStatus

    @Test("HealthStatus decodes components")
    func test_HealthStatus_decodesComponents() throws {
        let json = """
        {
            "status": "ok",
            "ams_env_configured": true,
            "components": {
                "clickhouse": {"status": "ok", "latency_ms": 5},
                "meta_store": {"status": "ok", "latency_ms": 2},
                "collector": {"status": "degraded", "message": "AMS unreachable"}
            }
        }
        """

        let data = json.data(using: .utf8)!
        let health = try JSONDecoder().decode(HealthStatus.self, from: data)

        #expect(health.status == .ok)
        #expect(health.amsEnvConfigured == true)
        #expect(health.components.clickhouse.status == .ok)
        #expect(health.components.clickhouse.latencyMs == 5)
        #expect(health.components.collector.status == .degraded)
        #expect(health.components.collector.message == "AMS unreachable")
        #expect(health.components.kafka == nil)
    }

    // MARK: - ComponentStatus

    @Test("ComponentStatus optional fields")
    func test_ComponentStatus_optionalFields() throws {
        let json = """
        {
            "status": "down"
        }
        """

        let data = json.data(using: .utf8)!
        let status = try JSONDecoder().decode(ComponentStatus.self, from: data)

        #expect(status.status == .down)
        #expect(status.latencyMs == nil)
        #expect(status.message == nil)
    }

    // MARK: - AuthMeResponse

    @Test("AuthMeResponse decodes auth method")
    func test_AuthMeResponse_decodesAuthMethod() throws {
        let json = """
        {
            "name": "admin@example.com",
            "role": "admin",
            "auth_method": "bearer"
        }
        """

        let data = json.data(using: .utf8)!
        let auth = try JSONDecoder().decode(AuthMeResponse.self, from: data)

        #expect(auth.name == "admin@example.com")
        #expect(auth.role == "admin")
        #expect(auth.authMethod == .bearer)
    }

    // MARK: - LicenseInfo

    @Test("LicenseInfo all tiers")
    func test_LicenseInfo_allTiers() throws {
        let tiers = ["free", "pro", "business", "enterprise"]
        let expected: [LicenseTier] = [.free, .pro, .business, .enterprise]

        for (tierStr, expectedTier) in zip(tiers, expected) {
            let json = """
            {
                "tier": "\(tierStr)",
                "valid": true
            }
            """

            let data = json.data(using: .utf8)!
            let license = try JSONDecoder().decode(LicenseInfo.self, from: data)
            #expect(license.tier == expectedTier)
        }
    }

    // MARK: - TierLimits

    @Test("TierLimits all optional")
    func test_TierLimits_allOptional() throws {
        let json = "{}"

        let data = json.data(using: .utf8)!
        let limits = try JSONDecoder().decode(TierLimits.self, from: data)

        #expect(limits.maxStreams == nil)
        #expect(limits.maxNodes == nil)
        #expect(limits.retentionDays == nil)
        #expect(limits.dataApi == nil)
        #expect(limits.whiteLabel == nil)
    }

    // MARK: - User

    @Test("User decodes role")
    func test_User_decodesRole() throws {
        let json = """
        {
            "id": "user-001",
            "username": "admin",
            "role": "admin",
            "created_at": 1700000000000
        }
        """

        let data = json.data(using: .utf8)!
        let user = try JSONDecoder().decode(User.self, from: data)

        #expect(user.id == "user-001")
        #expect(user.username == "admin")
        #expect(user.role == .admin)
        #expect(user.createdAt == 1700000000000)
    }

    // MARK: - Edge Cases

    @Test("LiveStreamList empty items")
    func test_LiveStreamList_emptyItems() throws {
        let json = """
        {
            "items": [],
            "meta": {}
        }
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(LiveStreamList.self, from: data)

        #expect(list.items.isEmpty)
        #expect(list.meta.nextCursor == nil)
        #expect(list.meta.total == nil)
    }

    @Test("Timestamp max Int64 value")
    func test_Timestamp_maxInt64Value() throws {
        let json = """
        {
            "ts": \(Int64.max),
            "total_viewers": 0,
            "total_publishers": 0,
            "protocol_mix": {},
            "apps": [],
            "nodes": []
        }
        """

        let data = json.data(using: .utf8)!
        let overview = try JSONDecoder().decode(LiveOverview.self, from: data)
        #expect(overview.ts == Int64.max)
    }

    // MARK: - Failure Paths

    @Test("LiveOverview missing required field throws")
    func test_LiveOverview_missingRequiredField_throws() throws {
        let json = """
        {
            "ts": 1700000000000,
            "total_viewers": 1234
        }
        """

        let data = json.data(using: .utf8)!
        #expect(throws: DecodingError.self) {
            _ = try JSONDecoder().decode(LiveOverview.self, from: data)
        }
    }

    @Test("NodeHealth invalid status enum throws")
    func test_NodeHealth_invalidStatusEnum_throws() throws {
        let json = """
        {
            "node_id": "node-1",
            "status": "invalid_status"
        }
        """

        let data = json.data(using: .utf8)!
        #expect(throws: DecodingError.self) {
            _ = try JSONDecoder().decode(NodeHealth.self, from: data)
        }
    }

    @Test("AlertState unknown value throws")
    func test_AlertState_unknownValue_throws() throws {
        let json = """
        {
            "id": "alert-1",
            "rule_id": "rule-1",
            "state": "unknown_state",
            "severity": "info",
            "ts": 1700000000000,
            "metric": "test",
            "value": 1.0,
            "threshold": 0.5
        }
        """

        let data = json.data(using: .utf8)!
        #expect(throws: DecodingError.self) {
            _ = try JSONDecoder().decode(AlertHistoryEntry.self, from: data)
        }
    }

    @Test("Truncated JSON throws")
    func test_truncatedJSON_throws() throws {
        let json = """
        {
            "ts": 1700000000000,
            "total_viewers": 1234,
        """

        let data = json.data(using: .utf8)!
        #expect(throws: DecodingError.self) {
            _ = try JSONDecoder().decode(LiveOverview.self, from: data)
        }
    }

    @Test("Wrong content type array instead of object")
    func test_wrongContentType_arrayInsteadOfObject() throws {
        let json = "[]"

        let data = json.data(using: .utf8)!
        #expect(throws: DecodingError.self) {
            _ = try JSONDecoder().decode(LiveOverview.self, from: data)
        }
    }

    // MARK: - Fixture Tests (Regression)

    @Test("Live overview fixture matches spec")
    func test_liveOverviewFixture_matchesSpec() throws {
        let json = """
        {
            "ts": 1700000000000,
            "total_viewers": 1234,
            "total_publishers": 56,
            "protocol_mix": {
                "webrtc": 500,
                "hls": 600,
                "rtmp": 100,
                "dash": 34,
                "other": 0
            },
            "apps": [
                {"app": "live", "viewers": 1000, "publishers": 50, "streams": 25},
                {"app": "webinar", "viewers": 234, "publishers": 6, "streams": 3}
            ],
            "nodes": [
                {
                    "node_id": "node-1",
                    "role": "origin",
                    "status": "up",
                    "last_seen": 1700000000000,
                    "cpu_pct": 45.5,
                    "mem_pct": 60.0,
                    "version": "3.0.3"
                },
                {
                    "node_id": "node-2",
                    "role": "edge",
                    "status": "degraded",
                    "last_seen": 1699999990000,
                    "cpu_pct": 85.0,
                    "mem_pct": 90.0,
                    "version": "3.0.3"
                }
            ]
        }
        """

        let data = json.data(using: .utf8)!
        let overview = try JSONDecoder().decode(LiveOverview.self, from: data)

        #expect(overview.ts == 1700000000000)
        #expect(overview.totalViewers == 1234)
        #expect(overview.totalPublishers == 56)
        #expect(overview.protocolMix.webrtc == 500)
        #expect(overview.apps.count == 2)
        #expect(overview.nodes.count == 2)
        #expect(overview.nodes[0].role == .origin)
        #expect(overview.nodes[1].status == .degraded)
    }

    @Test("Live streams fixture matches spec")
    func test_liveStreamsFixture_matchesSpec() throws {
        let json = """
        {
            "items": [
                {
                    "stream_id": "stream-001",
                    "app": "live",
                    "node_id": "node-1",
                    "tenant": "acme-corp",
                    "viewers": 500,
                    "publisher_state": "publishing",
                    "health_score": 95.5,
                    "protocol_mix": {"webrtc": 300, "hls": 200},
                    "bitrate_kbps": 2500.5,
                    "started_at": 1699996400000,
                    "viewer_rtt_ms": 45.5,
                    "viewer_jitter_ms": 5.2,
                    "viewer_loss_pct": 0.5
                },
                {
                    "stream_id": "stream-002",
                    "app": "webinar",
                    "viewers": 100,
                    "publisher_state": "idle",
                    "health_score": 80.0
                }
            ],
            "meta": {
                "next_cursor": "eyJsYXN0X2lkIjoic3RyZWFtLTAwMiJ9",
                "total": 150
            }
        }
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(LiveStreamList.self, from: data)

        #expect(list.items.count == 2)
        #expect(list.items[0].publisherState == .publishing)
        #expect(list.items[1].publisherState == .idle)
        #expect(list.meta.nextCursor != nil)
        #expect(list.meta.total == 150)
    }

    @Test("QoE summary fixture matches spec")
    func test_qoeSummaryFixture_matchesSpec() throws {
        let json = """
        {
            "totals": {
                "startup_p50_ms": 850.0,
                "startup_p95_ms": 2100.0,
                "rebuffer_ratio": 0.015,
                "error_rate": 0.005
            },
            "bitrate_timeline": [
                {"ts": 1700000000000, "bitrate_kbps_p50": 2500.0, "bitrate_kbps_p95": 4500.0},
                {"ts": 1700003600000, "bitrate_kbps_p50": 2400.0}
            ]
        }
        """

        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(QoeSummaryResponse.self, from: data)

        #expect(response.totals.startupP50Ms == 850.0)
        #expect(response.bitrateTimeline.count == 2)
        #expect(response.bitrateTimeline[0].bitrateKbpsP95 == 4500.0)
        #expect(response.bitrateTimeline[1].bitrateKbpsP95 == nil)
    }

    @Test("Alerts history fixture matches spec")
    func test_alertsHistoryFixture_matchesSpec() throws {
        let json = """
        {
            "items": [
                {
                    "id": "alert-001",
                    "rule_id": "rule-rebuffer",
                    "state": "firing",
                    "severity": "critical",
                    "ts": 1700000000000,
                    "metric": "rebuffer_ratio",
                    "value": 0.15,
                    "threshold": 0.1,
                    "scope": {
                        "node_id": "node-1",
                        "app": "live",
                        "stream_id": "stream-001",
                        "tenant": "acme"
                    },
                    "cooldown_until": 1700000300000,
                    "group_key": "stream-001"
                },
                {
                    "id": "alert-002",
                    "rule_id": "rule-viewers",
                    "state": "resolved",
                    "severity": "warning",
                    "ts": 1699996400000,
                    "metric": "viewer_count",
                    "value": 0.0,
                    "threshold": 10.0
                },
                {
                    "id": "alert-003",
                    "rule_id": "rule-delivery",
                    "state": "delivery_failure",
                    "severity": "info",
                    "ts": 1699990000000,
                    "metric": "error_rate",
                    "value": 0.02,
                    "threshold": 0.01
                }
            ],
            "meta": {"next_cursor": null, "total": 3}
        }
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(AlertHistoryList.self, from: data)

        #expect(list.items.count == 3)
        #expect(list.items[0].state == .firing)
        #expect(list.items[1].state == .resolved)
        #expect(list.items[2].state == .deliveryFailure)
    }

    @Test("Fleet nodes fixture matches spec")
    func test_fleetNodesFixture_matchesSpec() throws {
        let json = """
        {
            "items": [
                {
                    "node_id": "node-1",
                    "role": "origin",
                    "status": "up",
                    "last_seen": 1700000000000,
                    "version": "3.0.3",
                    "cpu_pct": 45.5,
                    "mem_pct": 60.0,
                    "net_in_mbps": 100.5,
                    "net_out_mbps": 500.0,
                    "os_name": "Ubuntu 22.04",
                    "os_arch": "amd64",
                    "java_version": "17.0.1",
                    "processor_count": 8
                },
                {
                    "node_id": "node-2",
                    "role": "edge",
                    "status": "degraded",
                    "last_seen": 1699999990000,
                    "version": null
                },
                {
                    "node_id": "node-3",
                    "role": "standalone",
                    "status": "up",
                    "last_seen": 1700000000000
                }
            ],
            "meta": {"total": 3}
        }
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(FleetNodeList.self, from: data)

        #expect(list.items.count == 3)
        #expect(list.items[0].role == .origin)
        #expect(list.items[1].role == .edge)
        #expect(list.items[2].role == .standalone)
    }

    @Test("Anomalies fixture matches spec")
    func test_anomaliesFixture_matchesSpec() throws {
        let json = """
        {
            "items": [
                {
                    "id": "anomaly-001",
                    "metric": "viewer_count",
                    "scope": {"app": "live", "stream_id": "stream-001"},
                    "observed": 50.0,
                    "expected": 500.0,
                    "sigma": 4.5,
                    "ts": 1700000000000
                },
                {
                    "id": "anomaly-002",
                    "metric": "ingest_bitrate_kbps",
                    "scope": {"node_id": "node-1", "app": "webinar"},
                    "observed": 8000.0,
                    "expected": 2500.0,
                    "sigma": 3.2,
                    "ts": 1699996400000
                }
            ],
            "meta": {"next_cursor": null, "total": 2}
        }
        """

        let data = json.data(using: .utf8)!
        let list = try JSONDecoder().decode(AnomalyList.self, from: data)

        #expect(list.items.count == 2)
        #expect(list.items[0].sigma == 4.5)
        #expect(list.items[1].metric == "ingest_bitrate_kbps")
    }

    @Test("Healthz fixture matches spec")
    func test_healthzFixture_matchesSpec() throws {
        let json = """
        {
            "status": "ok",
            "ams_env_configured": true,
            "components": {
                "clickhouse": {"status": "ok", "latency_ms": 5},
                "meta_store": {"status": "ok", "latency_ms": 2, "message": null},
                "collector": {"status": "degraded", "message": "AMS unreachable"},
                "kafka": {"status": "ok", "latency_ms": 10}
            }
        }
        """

        let data = json.data(using: .utf8)!
        let health = try JSONDecoder().decode(HealthStatus.self, from: data)

        #expect(health.status == .ok)
        #expect(health.amsEnvConfigured == true)
        #expect(health.components.clickhouse.latencyMs == 5)
        #expect(health.components.kafka != nil)
    }

    @Test("Auth me fixture matches spec")
    func test_authMeFixture_matchesSpec() throws {
        let json = """
        {
            "name": "admin@example.com",
            "role": "admin",
            "auth_method": "bearer"
        }
        """

        let data = json.data(using: .utf8)!
        let auth = try JSONDecoder().decode(AuthMeResponse.self, from: data)

        #expect(auth.name == "admin@example.com")
        #expect(auth.authMethod == .bearer)
    }

    @Test("License fixture matches spec")
    func test_licenseFixture_matchesSpec() throws {
        let json = """
        {
            "tier": "business",
            "valid": true,
            "limits": {
                "max_streams": 1000,
                "max_nodes": 5,
                "retention_days": 395,
                "data_api": true,
                "white_label": false
            },
            "expires_at": 1735689600000,
            "offline_file": false
        }
        """

        let data = json.data(using: .utf8)!
        let license = try JSONDecoder().decode(LicenseInfo.self, from: data)

        #expect(license.tier == .business)
        #expect(license.valid == true)
        #expect(license.limits?.maxStreams == 1000)
        #expect(license.limits?.maxNodes == 5)
        #expect(license.expiresAt == 1735689600000)
        #expect(license.offlineFile == false)
    }
}
