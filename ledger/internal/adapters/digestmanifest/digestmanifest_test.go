package digestmanifest_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/digestmanifest"
	"github.com/RachidChabane/spec-proof/ledger/internal/app"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

const verifiesManifest = " [verifies SP-151~1]"

func statementsOf(t *testing.T) []app.Statement {
	t.Helper()
	var statements []app.Statement
	for _, statement := range testkit.ValidStatements(t) {
		statements = append(statements, app.Statement{Location: statement.Location, Bytes: statement.Bytes})
	}
	return statements
}

func manifestFor(statements []app.Statement) string {
	var lines []string
	for _, statement := range statements {
		lines = append(lines, format.SHA256Hex(statement.Bytes)+"  "+statement.Location.StatementPath())
	}
	return strings.Join(lines, "\n") + "\n"
}

func writeManifest(t *testing.T, content string) digestmanifest.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "digests.sha256")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return digestmanifest.File{Path: path}
}

func TestCheck(t *testing.T) {
	statements := statementsOf(t)

	t.Run("statements that match their manifest are accepted"+verifiesManifest, func(t *testing.T) {
		if err := writeManifest(t, manifestFor(statements)).Check(statements); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an altered statement is refused"+verifiesManifest, func(t *testing.T) {
		manifest := writeManifest(t, manifestFor(statements))
		altered := append([]app.Statement(nil), statements...)
		altered[0].Bytes = append(append([]byte(nil), altered[0].Bytes...), ' ')
		if err := manifest.Check(altered); !errors.Is(err, refusal.ErrRefused) || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a statement missing from the manifest is refused"+verifiesManifest, func(t *testing.T) {
		manifest := writeManifest(t, manifestFor(statements[1:]))
		if err := manifest.Check(statements); !errors.Is(err, refusal.ErrRefused) || !strings.Contains(err.Error(), "is not in the manifest") {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a manifest entry without statement is refused"+verifiesManifest, func(t *testing.T) {
		manifest := writeManifest(t, manifestFor(statements))
		if err := manifest.Check(statements[1:]); !errors.Is(err, refusal.ErrRefused) || !strings.Contains(err.Error(), "not in the run") {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a malformed manifest is refused"+verifiesManifest, func(t *testing.T) {
		duplicate := format.SHA256Hex(nil) + "  a/b/c.json\n" + format.SHA256Hex(nil) + "  a/b/c.json\n"
		for name, content := range map[string]string{
			"no separator":  "abc\n",
			"short digest":  "abcd  a/b/c.json\n",
			"empty path":    format.SHA256Hex(nil) + "  \n",
			"listed twice":  duplicate,
			"single spaces": format.SHA256Hex(nil) + " a/b/c.json\n",
		} {
			if err := writeManifest(t, content).Check(statements); !errors.Is(err, refusal.ErrRefused) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("a missing manifest is a failure"+verifiesManifest, func(t *testing.T) {
		if err := (digestmanifest.File{Path: filepath.Join(t.TempDir(), "absent")}).Check(statements); err == nil || errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})
}
