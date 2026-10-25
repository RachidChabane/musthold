package scan_test

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/scanreport"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
)

const (
	verifiesPlain = " [verifies SP-2220~1]"

	goldenPlainRepository = "testdata/scan-repository.golden.txt"
	goldenPlainChange     = "testdata/scan-change.golden.txt"
)

// rendered is everything a plain scan gave its readers before host mode
// existed: the statement, the terminal headline, the Markdown report and the
// files of its folders. The route killers a folder offers are new files of
// their own, checked apart.
func rendered(t *testing.T, report scan.Report) []byte {
	t.Helper()
	files, err := scanreport.Files(report)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "=== statement\n%s\n=== text\n%s=== markdown\n%s", report.Encoded, scanreport.Text(report), scanreport.Markdown(report))
	for _, file := range files {
		if routeKiller(file.Path) {
			continue
		}
		fmt.Fprintf(&out, "=== file %s (executable %t)\n%s\n", file.Path, file.Executable, file.Content)
	}
	return out.Bytes()
}

// routeKiller reports whether a file of a finding folder offers a route's
// killer: the killers' list, a killer script or a laid-out witness.
func routeKiller(file string) bool {
	base := path.Base(file)
	return base == "killers.md" || strings.HasPrefix(base, "killer-") || strings.Contains(file, "/witness/")
}

func checkGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(golden, got) {
		t.Fatalf("the plain scan differs from %s; rerun with -update after checking the change", path)
	}
}

func TestPlainScanIsUnchanged(t *testing.T) {
	t.Run("a scan without a host renders the same statement, headline, report and folders as before host mode existed"+verifiesPlain, func(t *testing.T) {
		checkGolden(t, goldenPlainRepository, rendered(t, run(t, newFixture(), request())))
	})
	t.Run("a change scan without a host renders the same statement, headline, report and folders as before host mode existed"+verifiesPlain, func(t *testing.T) {
		f := newFixture()
		f.source.changed = map[string][]int{totals: {4}}
		changeRequest := request()
		changeRequest.Base = "main"
		checkGolden(t, goldenPlainChange, rendered(t, run(t, f, changeRequest)))
	})
}
