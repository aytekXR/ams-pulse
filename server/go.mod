module github.com/pulse-analytics/pulse/server

go 1.24

// Dependencies are added by implementation agents as packages land.
// Intended core deps (keep this list boring and minimal):
//   github.com/ClickHouse/clickhouse-go/v2   — event store
//   modernc.org/sqlite                       — metadata store (CGO-free)
//   github.com/jackc/pgx/v5                  — metadata store, HA option
//   github.com/prometheus/client_golang      — /metrics endpoint (F8)
//   github.com/segmentio/kafka-go            — optional AMS Kafka feed
//   nhooyr.io/websocket                      — live dashboard push
