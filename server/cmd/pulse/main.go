// Command pulse is the single Pulse server binary (PRD §7.10: "single Go binary,
// stateless"). One binary, multiple run modes, so the default Docker Compose
// deployment runs everything in one container while large installs can split roles.
//
// Usage (target state):
//
//	pulse serve            # all-in-one: collector + query API + alert evaluator + reports
//	pulse serve --role=collector|api|alerter
//	pulse migrate          # apply contracts/db migrations
//	pulse diag             # diagnostic bundle exporter (PRD §7.13 support mitigation)
package main

import (
	"fmt"
	"os"
)

func main() {
	// TODO(BE-01): wire config loading (internal/config), role selection, graceful
	// shutdown, and service startup. No business logic in main — it only assembles
	// the services defined in internal/.
	fmt.Fprintln(os.Stderr, "pulse: skeleton build — not implemented yet")
	os.Exit(1)
}
