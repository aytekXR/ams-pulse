// Package config — Docker secrets (_FILE) resolution helper.
//
// Resolution order for each secret-bearing environment variable:
//
//  1. If ${NAME}_FILE is set, read the file at that path, trim one trailing
//     newline (\n or \r\n), and use the result.  A missing or unreadable file
//     is a HARD startup error (fail-closed, loud message) — do not fall back to
//     the plain env var; the operator explicitly chose file-based delivery.
//  2. Otherwise fall back to ${NAME}.
//
// "File wins" when both are set because the file is the deliberate operator
// choice: the :? guard in the hardened compose overlay is satisfied by the
// (possibly empty) plain env var, while the secret lives exclusively in the
// Docker secret file.
package config

import (
	"fmt"
	"os"
	"strings"
)

// GetSecret resolves a secret by name using the _FILE convention.
//
// Exported so that cmd/pulse/config.go (package main) can call it directly
// without duplicating the logic.
func GetSecret(name string) (string, error) {
	fileEnv := name + "_FILE"
	if path := os.Getenv(fileEnv); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("config: %s=%q — cannot read secret file: %w (fix: create the file or unset %s)", fileEnv, path, err, fileEnv)
		}
		// Trim exactly one trailing newline (\r\n or \n) — editors and `echo`
		// commonly add one; stripping more would silently corrupt secrets that
		// intentionally end with whitespace.
		secret := string(data)
		secret = strings.TrimSuffix(secret, "\r\n")
		secret = strings.TrimSuffix(secret, "\n")
		return secret, nil
	}
	return os.Getenv(name), nil
}
