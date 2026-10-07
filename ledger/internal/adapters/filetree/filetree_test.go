package filetree_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/filetree"
	"github.com/RachidChabane/spec-proof/ledger/internal/app"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

const (
	verifiesSignInput = " [verifies SP-151~1]"
	verifiesAppendDir = " [verifies SP-155~1]"
	verifiesExport    = " [verifies SP-157~1]"
)

func TestStatementDir(t *testing.T) {
	t.Run("the statements of a cache directory are listed with their locations"+verifiesSignInput, func(t *testing.T) {
		root := t.TempDir()
		statements := testkit.ValidStatements(t)
		testkit.WriteStatements(t, root, statements)
		listed, err := filetree.StatementDir{Root: root}.Statements()
		if err != nil || len(listed) != len(statements) {
			t.Fatalf("listed %d: %v", len(listed), err)
		}
		for _, statement := range listed {
			if !statement.Location.IsValid() || len(statement.Bytes) == 0 {
				t.Fatalf("statement %+v", statement)
			}
		}
	})

	t.Run("a file that is not a statement is refused"+verifiesSignInput, func(t *testing.T) {
		for _, stray := range []string{"README.md", "module/subject/notacachekey.json", "module/" + format.SHA256Hex(nil) + ".json"} {
			root := t.TempDir()
			testkit.WriteFile(t, filepath.Join(root, filepath.FromSlash(stray)), []byte("{}"))
			if _, err := (filetree.StatementDir{Root: root}).Statements(); !errors.Is(err, refusal.ErrRefused) {
				t.Errorf("%s: error %v", stray, err)
			}
		}
	})

	t.Run("a symbolic link is refused"+verifiesSignInput, func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(t.TempDir(), "outside.json")
		testkit.WriteFile(t, target, []byte("{}"))
		location := testkit.ValidStatements(t)[0].Location
		link := filepath.Join(root, filepath.FromSlash(location.StatementPath()))
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := (filetree.StatementDir{Root: root}).Statements(); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a missing directory is a failure"+verifiesSignInput, func(t *testing.T) {
		if _, err := (filetree.StatementDir{Root: filepath.Join(t.TempDir(), "absent")}).Statements(); err == nil {
			t.Fatal("listed a directory that does not exist")
		}
	})
}

func TestBundleDir(t *testing.T) {
	location := testkit.ValidStatements(t)[0].Location

	t.Run("bundles are written in the layout of the ledger"+verifiesSignInput, func(t *testing.T) {
		root := t.TempDir()
		if err := (filetree.BundleDir{Root: root}).WriteBundles([]app.Bundle{{Location: location, Bytes: []byte("bundle")}}); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(location.BundlePath())))
		if err != nil || string(got) != "bundle" {
			t.Fatalf("content %q: %v", got, err)
		}
	})

	t.Run("the same bytes again are accepted and other bytes are refused"+verifiesSignInput, func(t *testing.T) {
		root := t.TempDir()
		sink := filetree.BundleDir{Root: root}
		bundles := []app.Bundle{{Location: location, Bytes: []byte("bundle")}}
		if err := sink.WriteBundles(bundles); err != nil {
			t.Fatal(err)
		}
		if err := sink.WriteBundles(bundles); err != nil {
			t.Fatalf("second write: %v", err)
		}
		other := []app.Bundle{{Location: location, Bytes: []byte("other")}}
		if err := sink.WriteBundles(other); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("overwrite: error %v", err)
		}
	})
}

func TestAdditionDir(t *testing.T) {
	t.Run("files are listed below their ledger prefix"+verifiesAppendDir, func(t *testing.T) {
		root := t.TempDir()
		testkit.WriteFile(t, filepath.Join(root, "targets", "a.json"), []byte("a"))
		testkit.WriteFile(t, filepath.Join(root, "root.json"), []byte("r"))
		additions, err := filetree.AdditionDir{Root: root, Prefix: format.TrustDirectory}.Additions()
		if err != nil {
			t.Fatal(err)
		}
		if len(additions) != 2 || additions[0].Path != "trust/root.json" || additions[1].Path != "trust/targets/a.json" {
			t.Fatalf("additions %+v", additions)
		}
	})

	t.Run("an empty prefix keeps the layout the directory already has"+verifiesAppendDir, func(t *testing.T) {
		root := t.TempDir()
		testkit.WriteFile(t, filepath.Join(root, "proofs", "m", "s", "k.dsse.json"), []byte("b"))
		additions, err := filetree.AdditionDir{Root: root}.Additions()
		if err != nil || len(additions) != 1 || additions[0].Path != "proofs/m/s/k.dsse.json" {
			t.Fatalf("additions %+v: %v", additions, err)
		}
	})

	t.Run("sources are concatenated in order"+verifiesAppendDir, func(t *testing.T) {
		first := filetree.FixedAdditions{{Path: "seeds/a.json"}}
		second := filetree.FixedAdditions{{Path: "seeds/b.json", Policy: ledger.PolicyKeepExisting}}
		additions, err := filetree.AdditionSources{first, second}.Additions()
		if err != nil || len(additions) != 2 || additions[1].Path != "seeds/b.json" {
			t.Fatalf("additions %+v: %v", additions, err)
		}
	})

	t.Run("a failing source stops the listing"+verifiesAppendDir, func(t *testing.T) {
		missing := filetree.AdditionDir{Root: filepath.Join(t.TempDir(), "absent")}
		if _, err := (filetree.AdditionSources{missing}).Additions(); err == nil {
			t.Fatal("listed a directory that does not exist")
		}
	})
}

func TestExportDir(t *testing.T) {
	t.Run("an absent directory is created and an empty one is accepted"+verifiesExport, func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "out", "nested")
		if err := (filetree.ExportDir{Root: absent}).Prepare(); err != nil {
			t.Fatal(err)
		}
		if err := (filetree.ExportDir{Root: absent}).Prepare(); err != nil {
			t.Fatalf("an empty directory: %v", err)
		}
	})

	t.Run("a directory that holds files is refused"+verifiesExport, func(t *testing.T) {
		root := t.TempDir()
		testkit.WriteFile(t, filepath.Join(root, "stale.json"), []byte("{}"))
		if err := (filetree.ExportDir{Root: root}).Prepare(); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("files are written below the directory and never outside it"+verifiesExport, func(t *testing.T) {
		root := t.TempDir()
		sink := filetree.ExportDir{Root: root}
		if err := sink.WriteFile("proofs/m/s/k.dsse.json", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, "proofs", "m", "s", "k.dsse.json")); err != nil {
			t.Fatal(err)
		}
		for _, escape := range []string{"../outside", "a/../../outside", "/etc/passwd"} {
			if err := sink.WriteFile(escape, []byte("x")); !errors.Is(err, refusal.ErrRefused) {
				t.Errorf("%s: error %v", escape, err)
			}
		}
	})
}
