// Package kafka consumes the native AMS Kafka producer feed (instance and
// stream stats every 15s when server.kafka_brokers is configured). Optional
// source: customers who already enabled Kafka for DIY Grafana get instant
// richer data — part of converting the DIY crowd (PRD §7.7).
package kafka

// TODO(BE-01): Consumer implementing collector.Source.
