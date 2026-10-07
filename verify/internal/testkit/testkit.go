// Package testkit builds the fixtures the tests of the verify module share: valid
// statements taken from the golden vectors of the format, their local variants,
// Ed25519 identities, exported ledgers and trust policies.
package testkit

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

const (
	goldenRelativePath = "format/testdata/golden"
	maxAscent          = 12
	directoryMode      = 0o755
	fileMode           = 0o644
)

// Statement is a valid canonical statement of a module.
type Statement struct {
	Module   string
	Bytes    []byte
	Location format.ProofLocation
}

var goldenFiles = []struct{ name, module string }{
	{"alignment-valid.json", "alignment"},
	{"discriminating-power-valid.json", "discpower"},
	{"completeness-valid.json", "completeness"},
}

// ValidStatements returns one valid CI-produced statement of each predicate type.
func ValidStatements(t testing.TB) []Statement {
	t.Helper()
	directory := goldenDirectory(t)
	var statements []Statement
	for _, golden := range goldenFiles {
		raw, err := os.ReadFile(filepath.Join(directory, golden.name))
		if err != nil {
			t.Fatal(err)
		}
		canonical := bytes.TrimSuffix(raw, []byte("\n"))
		admitted, err := format.Admit(canonical)
		if err != nil {
			t.Fatalf("golden %s: %v", golden.name, err)
		}
		statements = append(statements, Statement{Module: golden.module, Bytes: canonical, Location: admitted.Location(golden.module)})
	}
	return statements
}

// AsLocal returns the statement as a local cache holds it: the same statement
// produced by `local`.
func (s Statement) AsLocal(t testing.TB) Statement {
	t.Helper()
	local := bytes.Replace(s.Bytes, []byte(`"producedBy":"ci"`), []byte(`"producedBy":"local"`), 1)
	if bytes.Equal(local, s.Bytes) {
		t.Fatal("statement has no producedBy member")
	}
	admitted, err := format.Admit(local)
	if err != nil {
		t.Fatalf("local variant: %v", err)
	}
	return Statement{Module: s.Module, Bytes: local, Location: admitted.Location(s.Module)}
}

func goldenDirectory(t testing.TB) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for range maxAscent {
		candidate := filepath.Join(directory, goldenRelativePath)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		directory = filepath.Dir(directory)
	}
	t.Fatal("golden vectors of the format not found")
	return ""
}

// WriteFile writes a file, creating its directories.
func WriteFile(t testing.TB, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, fileMode); err != nil {
		t.Fatal(err)
	}
}

// Identity is a fixed Ed25519 key.
type Identity struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}

// NewIdentity derives a key from a one-byte seed.
func NewIdentity(seedByte byte) Identity {
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seedByte}, ed25519.SeedSize))
	public, _ := private.Public().(ed25519.PublicKey)
	return Identity{Private: private, Public: public}
}

// PolicyYAML renders a trust policy that pins the given keys.
func PolicyYAML(identities ...Identity) []byte {
	var policy bytes.Buffer
	policy.WriteString("version: 1\ned25519:\n")
	for _, identity := range identities {
		policy.WriteString("  - publicKey: " + base64.StdEncoding.EncodeToString(identity.Public) + "\n")
	}
	return policy.Bytes()
}

// Layout is the directory `.spec-proof` of a repository and what is below it.
type Layout struct {
	Root     string
	Verified string
	Policy   string
	Pending  string
}

// NewLayout creates an empty `.spec-proof` directory in a temporary directory.
func NewLayout(t testing.TB) Layout {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".spec-proof")
	if err := os.MkdirAll(root, directoryMode); err != nil {
		t.Fatal(err)
	}
	return Layout{
		Root:     root,
		Verified: filepath.Join(root, "verified"),
		Policy:   filepath.Join(root, "trust-policy.yaml"),
		Pending:  filepath.Join(root, format.PendingDirectory),
	}
}

// ExportBundles writes the statements as signed proofs of the identity below a
// fresh directory shaped like the output of `ledger export`.
func ExportBundles(t testing.TB, identity Identity, statements []Statement) string {
	t.Helper()
	root := t.TempDir()
	for _, statement := range statements {
		bundle, err := format.SignEnvelope(statement.Bytes, identity.Private).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		WriteFile(t, filepath.Join(root, filepath.FromSlash(statement.Location.BundlePath())), bundle)
	}
	return root
}

// WriteCache writes statements as a module's local cache holds them.
func WriteCache(t testing.TB, statements []Statement) string {
	t.Helper()
	root := t.TempDir()
	for _, statement := range statements {
		WriteFile(t, filepath.Join(root, filepath.FromSlash(statement.Location.StatementPath())), statement.Bytes)
	}
	return root
}
