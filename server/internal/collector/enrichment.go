// Package collector — enrichment interfaces.
//
// GeoResolver and UAParser are injected into the normalization layer.
// Wave 1: no-op implementations are the default. Wave 2 may inject real ones.
package collector

import "github.com/pulse-analytics/pulse/server/internal/domain"

// GeoResolver maps an IP address to geo metadata.
// The no-op resolver returns an empty block (acceptable for wave 1).
type GeoResolver interface {
	Resolve(ip string) domain.GeoEnrichment
}

// UAParser parses a User-Agent string into client metadata.
type UAParser interface {
	Parse(ua string) domain.ClientEnrichment
}

// NoopGeoResolver is a pass-through resolver for wave 1.
type NoopGeoResolver struct{}

func (NoopGeoResolver) Resolve(_ string) domain.GeoEnrichment { return domain.GeoEnrichment{} }

// NoopUAParser is a pass-through parser for wave 1.
type NoopUAParser struct{}

func (NoopUAParser) Parse(_ string) domain.ClientEnrichment { return domain.ClientEnrichment{} }
