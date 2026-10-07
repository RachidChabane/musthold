package filesource_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/adapters/filesource"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/verify/internal/testkit"
)

const (
	verifiesBundles = " [verifies SP-213~1]"
	verifiesLocal   = " [verifies SP-214~1]"
	verifiesPending = " [verifies SP-216~1]"
)

func TestExportedLedger(t *testing.T) {
	statements := testkit.ValidStatements(t)

	t.Run("the proofs of an exported ledger are listed with their locations and the trust snapshot is skipped"+verifiesBundles, func(t *testing.T) {
		root := testkit.ExportBundles(t, testkit.NewIdentity(1), statements)
		testkit.WriteFile(t, filepath.Join(root, "trust", "targets", "x.json"), []byte("{}"))
		bundles, err := filesource.ExportedLedger{Root: root}.Bundles()
		if err != nil || len(bundles) != 3 {
			t.Fatalf("bundles %d: %v", len(bundles), err)
		}
		for _, bundle := range bundles {
			if !bundle.Location.IsValid() || len(bundle.Bytes) == 0 {
				t.Fatalf("bundle %+v", bundle)
			}
		}
	})

	t.Run("a file the verifier would otherwise ignore is refused"+verifiesBundles, func(t *testing.T) {
		for _, stray := range []string{"README.md", "mutants/x.age", "proofs/notaproof.json", "proofs/m/s/" + format.SHA256Hex(nil) + ".json", "seeds/a.json"} {
			root := testkit.ExportBundles(t, testkit.NewIdentity(1), statements)
			testkit.WriteFile(t, filepath.Join(root, filepath.FromSlash(stray)), []byte("x"))
			if _, err := (filesource.ExportedLedger{Root: root}).Bundles(); !refusal.Is(err) {
				t.Errorf("%s: error %v", stray, err)
			}
		}
	})

	t.Run("a symbolic link is refused"+verifiesBundles, func(t *testing.T) {
		root := testkit.ExportBundles(t, testkit.NewIdentity(1), statements)
		target := filepath.Join(t.TempDir(), "outside")
		testkit.WriteFile(t, target, []byte("x"))
		link := filepath.Join(root, "trust", "link")
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := (filesource.ExportedLedger{Root: root}).Bundles(); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a missing directory is a failure"+verifiesBundles, func(t *testing.T) {
		if _, err := (filesource.ExportedLedger{Root: filepath.Join(t.TempDir(), "absent")}).Bundles(); err == nil || refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestLocalCache(t *testing.T) {
	t.Run("the cached statements are listed with their locations"+verifiesLocal, func(t *testing.T) {
		root := testkit.WriteCache(t, testkit.ValidStatements(t))
		statements, err := filesource.LocalCache{Root: root}.LocalStatements()
		if err != nil || len(statements) != 3 {
			t.Fatalf("statements %d: %v", len(statements), err)
		}
	})

	t.Run("a file that is not a statement is refused"+verifiesLocal, func(t *testing.T) {
		root := testkit.WriteCache(t, testkit.ValidStatements(t))
		testkit.WriteFile(t, filepath.Join(root, "notes.txt"), []byte("x"))
		if _, err := (filesource.LocalCache{Root: root}).LocalStatements(); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestPendingDir(t *testing.T) {
	write := func(t *testing.T, name, content string) string {
		t.Helper()
		root := t.TempDir()
		testkit.WriteFile(t, filepath.Join(root, name), []byte(content))
		return root
	}

	t.Run("the pending file of a module is read"+verifiesPending, func(t *testing.T) {
		root := write(t, "discpower.json", `{"module":"discpower","pending":[{"criterion":"PARK-2~1","reason":"awaiting_host"}]}`)
		pending, err := filesource.PendingDir{Root: root}.Pending()
		if err != nil || len(pending) != 1 || pending[0].Module != "discpower" || len(pending[0].Pending) != 1 {
			t.Fatalf("pending %+v: %v", pending, err)
		}
	})

	t.Run("an absent directory means nothing is pending"+verifiesPending, func(t *testing.T) {
		pending, err := filesource.PendingDir{Root: filepath.Join(t.TempDir(), "absent")}.Pending()
		if err != nil || len(pending) != 0 {
			t.Fatalf("pending %+v: %v", pending, err)
		}
	})

	refused := map[string]struct{ name, content string }{
		"a file about another module":      {"alignment.json", `{"module":"discpower","pending":[]}`},
		"a file that is not JSON":          {"discpower.json", `{`},
		"an entry that breaks format":      {"discpower.json", `{"module":"discpower","pending":[{"criterion":"x","reason":"awaiting_host"}]}`},
		"a file that is not <module>.json": {"discpower.txt", `{}`},
	}
	for name, tt := range refused {
		t.Run(name+" is refused"+verifiesPending, func(t *testing.T) {
			if _, err := (filesource.PendingDir{Root: write(t, tt.name, tt.content)}).Pending(); !refusal.Is(err) {
				t.Fatalf("error %v", err)
			}
		})
	}

	t.Run("a subdirectory is refused"+verifiesPending, func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "nested.json"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := (filesource.PendingDir{Root: root}).Pending(); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})
}
