// Package api_test — S91 source-type enum drift guard (D-155).
//
// The logtail collector was deleted in D-062 (SourceLogTail, its package, and its
// serve.go wiring all removed), yet the OpenAPI Source/SourceWrite `type` enums kept
// advertising a `log_tail` source type — a contract that promised a source kind the
// server can no longer collect. D-155 narrowed both enums to the three source types
// that actually exist (rest_poll, kafka, webhook).
//
// This test pins that enum against the loaded spec so the dead value cannot silently
// return (re-adding `log_tail` to the yaml fails here) AND a live type cannot silently
// vanish (dropping one of the three also fails). It is deliberately spec-driven rather
// than behavioral: amsSourceFromAPI does not validate `type` against the enum (it
// accepts any non-empty string), so the enum is a documentation contract only — the
// meaningful regression surface is the spec text itself.
package api_test

import (
	"sort"
	"testing"
)

// wantSourceTypeEnum is the exact set of configurable AMS source `type` values.
// It mirrors server/internal/domain/types.go's live source constants, MINUS two:
//   - log_tail — deleted in D-062 (the removal this test guards).
//   - host_agent (domain.SourceHostAgent) — an event-origin tag stamped on
//     ServerEvent.source (see ams-server-event.schema.json), NOT an operator-
//     configurable /admin/sources type; it was never in the API source-config enum.
var wantSourceTypeEnum = []string{"kafka", "rest_poll", "webhook"} // sorted

func TestS91_SourceTypeEnum_NoLogTail(t *testing.T) {
	doc := openAPISpec(t)

	for _, schemaName := range []string{"Source", "SourceWrite"} {
		ref := doc.Components.Schemas[schemaName]
		if ref == nil || ref.Value == nil {
			t.Fatalf("schema %q missing from spec", schemaName)
		}
		typeProp := ref.Value.Properties["type"]
		if typeProp == nil || typeProp.Value == nil {
			t.Fatalf("schema %q has no `type` property", schemaName)
		}
		rawEnum := typeProp.Value.Enum
		if len(rawEnum) == 0 {
			t.Fatalf("schema %q `type` has an empty enum — the contract no longer "+
				"constrains source types (drift guard is vacuous)", schemaName)
		}

		got := make([]string, 0, len(rawEnum))
		for _, v := range rawEnum {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("schema %q `type` enum has a non-string value %v (%T)", schemaName, v, v)
			}
			if s == "log_tail" {
				t.Errorf("schema %q `type` enum still lists the deleted `log_tail` source "+
					"(logtail collector removed in D-062) — contract drift reintroduced", schemaName)
			}
			got = append(got, s)
		}
		sort.Strings(got)

		if len(got) != len(wantSourceTypeEnum) {
			t.Fatalf("schema %q `type` enum = %v, want exactly %v", schemaName, got, wantSourceTypeEnum)
		}
		for i := range got {
			if got[i] != wantSourceTypeEnum[i] {
				t.Fatalf("schema %q `type` enum = %v, want exactly %v", schemaName, got, wantSourceTypeEnum)
			}
		}
	}
}
