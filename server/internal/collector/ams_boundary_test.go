// ams_boundary_test.go — enforces ARCHITECTURE.md §3 rule 2 (AMS isolation).
//
// The rule: only the collector boundary parses AMS wire formats; everything
// downstream consumes normalized domain types. That is what keeps the Phase-3
// "swap the collector, not the product" plan real.
//
// It had been a prose rule with no enforcement, and external review round 6
// (H-07) found a package that had drifted outside it — `internal/cluster`, which
// consumes amsclient.ClusterNodeDTO directly. The resolution was to name that
// package as part of the boundary (it is functionally the cluster collector) and
// to make the boundary a test, so the NEXT drift fails CI instead of waiting for
// a reviewer.
package collector

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// amsImportPath is the package whose types are AMS wire shapes.
const amsImportPath = "github.com/aytekXR/ams-pulse/server/pkg/amsclient"

// allowedAMSImporters is the complete set of non-test packages permitted to
// import amsclient, expressed as paths relative to server/.
//
// Adding an entry here is a deliberate architectural decision, not a formality:
// it widens the surface that a non-AMS backend would have to reimplement. If you
// are tempted to add one, first check whether the package should consume a
// domain type instead.
var allowedAMSImporters = map[string]string{
	"pkg/amsclient":                 "the AMS client itself",
	"internal/collector":            "ARCHITECTURE §3 rule 2 — the collector boundary",
	"internal/collector/restpoller": "collector boundary (REST polling)",
	"internal/cluster":              "collector boundary by exception — functionally the cluster collector (H-07); named in ARCHITECTURE §3 rule 2",
	"cmd/pulse":                     "composition root — wires the client, never interprets wire shapes",
}

// serverRoot walks up from this test file to the server/ module root.
func serverRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd() // .../server/internal/collector
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate server/go.mod above %s", dir)
	return ""
}

// TestAMSBoundary_ImportersAreCollectorOnly fails when a package outside the
// declared boundary imports amsclient.
//
// Non-test files only: tests legitimately construct AMS DTOs as fixtures, and
// forbidding that would push test data into awkward indirection.
func TestAMSBoundary_ImportersAreCollectorOnly(t *testing.T) {
	root := serverRoot(t)
	fset := token.NewFileSet()
	violations := map[string][]string{}
	checked := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		checked++
		for _, imp := range f.Imports {
			p, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil || p != amsImportPath {
				continue
			}
			rel, _ := filepath.Rel(root, filepath.Dir(path))
			rel = filepath.ToSlash(rel)
			if _, ok := allowedAMSImporters[rel]; !ok {
				relFile, _ := filepath.Rel(root, path)
				violations[rel] = append(violations[rel], filepath.ToSlash(relFile))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if checked == 0 {
		t.Fatal("scanned 0 Go files — the walk is broken, so this test proves nothing")
	}

	for pkg, files := range violations {
		t.Errorf("ARCHITECTURE §3 rule 2 violation: package %q imports %s (files: %v).\n"+
			"Consume a normalized domain type instead, or — if this package really is part of the\n"+
			"collector boundary — add it to allowedAMSImporters here AND name it in ARCHITECTURE §3.",
			pkg, amsImportPath, files)
	}
	t.Logf("scanned %d non-test Go files; %d packages may import amsclient", checked, len(allowedAMSImporters))
}

// The allow-list is only meaningful if it is accurate: an entry naming a package
// that no longer imports amsclient is stale documentation masquerading as a rule.
func TestAMSBoundary_AllowListHasNoStaleEntries(t *testing.T) {
	root := serverRoot(t)
	fset := token.NewFileSet()
	actual := map[string]bool{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			if p, uerr := strconv.Unquote(imp.Path.Value); uerr == nil && p == amsImportPath {
				rel, _ := filepath.Rel(root, filepath.Dir(path))
				actual[filepath.ToSlash(rel)] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	for pkg, why := range allowedAMSImporters {
		if pkg == "pkg/amsclient" {
			continue // the package itself never imports itself
		}
		if !actual[pkg] {
			t.Errorf("stale allow-list entry %q (%s): it no longer imports %s — remove it "+
				"and narrow ARCHITECTURE §3 rule 2 accordingly", pkg, why, amsImportPath)
		}
	}
}
