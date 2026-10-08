package api

import (
	"fmt"
	"sort"
)

// Typed accessors for request bodies decoded into map[string]any.
//
// A PRESENT field of the wrong JSON type is an error the caller turns into a 422 —
// never a silent zero. Before S126, rule and schedule parsing used `x, _ := body[k].(T)`,
// so `"threshold": "90"` stored a threshold of 0 and the rule fired on every sample.
// Absent fields and JSON null both mean "not given" (ok=false, no error).

func strField(body map[string]any, key string) (string, bool, error) {
	v, present := body[key]
	if !present || v == nil {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be a string, got %s", key, jsonType(v))
	}
	return s, true, nil
}

func numField(body map[string]any, key string) (float64, bool, error) {
	v, present := body[key]
	if !present || v == nil {
		return 0, false, nil
	}
	n, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("%s must be a number, got %s", key, jsonType(v))
	}
	return n, true, nil
}

func boolField(body map[string]any, key string) (bool, bool, error) {
	v, present := body[key]
	if !present || v == nil {
		return false, false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, false, fmt.Errorf("%s must be true or false, got %s", key, jsonType(v))
	}
	return b, true, nil
}

// objField returns a JSON object field, or nil when absent/null.
func objField(body map[string]any, key string) (map[string]any, error) {
	v, present := body[key]
	if !present || v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object, got %s", key, jsonType(v))
	}
	return m, nil
}

// stringObject checks that obj holds only the allowed keys, each a string (or null).
func stringObject(field string, obj map[string]any, allowed ...string) error {
	for k, v := range obj {
		known := false
		for _, a := range allowed {
			if k == a {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("%s: unknown field %q (allowed: %v)", field, k, allowed)
		}
		if _, ok := v.(string); v != nil && !ok {
			return fmt.Errorf("%s.%s must be a string, got %s", field, k, jsonType(v))
		}
	}
	return nil
}

// knownFields refuses a top-level key the endpoint does not read — before S126 a key
// written as the runbooks showed it (`cron_expr`, `app_filter`, a singular
// `maintenance_window`) was dropped without a word, so a "scoped" statement covered every
// tenant and a "maintenance window" never suppressed anything. Keys a GET returns
// (id, created_at, …) are accepted and ignored, so GET → edit → PUT keeps working.
// hints maps a common wrong name to the right one.
func knownFields(body map[string]any, fields []string, hints map[string]string) error {
	allowed := make(map[string]bool, len(fields))
	for _, f := range fields {
		allowed[f] = true
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if allowed[k] {
			continue
		}
		if h, ok := hints[k]; ok {
			return fmt.Errorf("unknown field %q (use %s)", k, h)
		}
		return fmt.Errorf("unknown field %q", k)
	}
	return nil
}

// jsonType names the JSON type of a value decoded by encoding/json.
func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case float64:
		return "a number"
	case bool:
		return "a boolean"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	default:
		return fmt.Sprintf("%T", v)
	}
}
