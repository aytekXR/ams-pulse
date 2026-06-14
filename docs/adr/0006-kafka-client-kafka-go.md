# ADR 0006: kafka-go chosen over franz-go for Kafka consumer

**Status:** Accepted · **Date:** 2026-06-14 (Wave 2)

## Context

Wave 2 adds a Kafka source (`internal/collector/kafka`) to consume AMS's native
Kafka producer output (node stats, ingest stats, stream stats). Two pure-Go Kafka
client libraries were evaluated:

- `github.com/segmentio/kafka-go` (v0.4.51)
- `github.com/twmb/franz-go` (latest)

The use case is a single consumer group reading JSON messages from a small number
of topics (3–5) produced by AMS. No producer functionality is needed.

Constraints:
- `CGO_ENABLED=0` required (D-001)
- Minimal dependency surface (single binary goal, D-003)
- AMS produces simple JSON blobs; no Avro/Protobuf schema registry

## Decision

Use **`github.com/segmentio/kafka-go`**.

## Rationale

| Criterion | kafka-go | franz-go |
|---|---|---|
| CGO-free | Yes | Yes |
| Consumer group API | Simple (one `Reader` call) | Feature-rich but more complex wiring |
| Dependency count | Lower (fewer transitive deps) | Higher (more features = more surface) |
| AMS message format fit | JSON only — simplicity wins | Avro/Protobuf/JSON — overkill |
| Community / maintenance | Mature, stable API | Newer, more actively developed |

kafka-go's `kafka.NewReader` provides a simple consumer-group-aware reader with
commit management in ~10 lines. franz-go offers better performance at very high
throughput (millions of msg/s) and more advanced features (EOS, transactional),
but those are not needed for AMS's stats producer which publishes at 1 msg/5 s
per stream.

For deployments that grow to require franz-go's throughput characteristics, the
`collector.Source` interface makes swapping the underlying library a contained
change to `internal/collector/kafka/kafka.go` only.

## Consequences

- `github.com/segmentio/kafka-go v0.4.51` is a direct dependency. It is pure-Go
  and `CGO_ENABLED=0` compatible.
- Kafka consumer tests run with a stub transport (D-007.5: no Kafka broker in the
  dev/CI environment). 8 contract tests verify message routing and error handling
  without a real broker.
- Consumer lag (`Lag()`) and parse error counts (`ParseErrors()`) are exposed for
  future `/healthz` surfacing (GAP-2-003, Wave 3).
