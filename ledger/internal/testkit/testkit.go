// Package testkit builds the fixtures the tests of the ledger module share: valid
// statements taken from the golden vectors of the format, statement and bare
// repository directories, an Ed25519 identity and an environment that isolates git
// from the configuration of the machine.
package testkit

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

const (
	goldenRelativePath = "format/testdata/golden"
	maxAscent          = 12
	directoryMode      = 0o755
	fileMode           = 0o644
	privateFileMode    = 0o600
)

// Statement is a valid, canonical, CI-produced statement and the module directory
// that would hold it in a cache.
type Statement struct {
	Module   string
	Bytes    []byte
	Location format.ProofLocation
}

var goldenModules = map[string]string{
	"alignment-valid.json":            "alignment",
	"discriminating-power-valid.json": "discpower",
	"completeness-valid.json":         "completeness",
}

// ValidStatements returns one valid statement of each predicate type, read from the
// golden vectors of the format as canonical bytes (the vector files end with a
// newline that the statement does not have).
func ValidStatements(t testing.TB) []Statement {
	t.Helper()
	directory := goldenDirectory(t)
	var statements []Statement
	for _, name := range []string{"alignment-valid.json", "discriminating-power-valid.json", "completeness-valid.json"} {
		raw, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		canonical := bytes.TrimSuffix(raw, []byte("\n"))
		admitted, err := format.Admit(canonical)
		if err != nil {
			t.Fatalf("golden %s: %v", name, err)
		}
		module := goldenModules[name]
		statements = append(statements, Statement{Module: module, Bytes: canonical, Location: admitted.Location(module)})
	}
	return statements
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

// WriteStatements writes statements below root as a module cache would.
func WriteStatements(t testing.TB, root string, statements []Statement) {
	t.Helper()
	for _, statement := range statements {
		WriteFile(t, filepath.Join(root, filepath.FromSlash(statement.Location.StatementPath())), statement.Bytes)
	}
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

// GitEnvironment isolates git from the user's configuration, hooks and identity.
func GitEnvironment(t testing.TB) []string {
	t.Helper()
	return []string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
	}
}

// NewBareRepository creates an empty bare repository and returns its directory.
func NewBareRepository(t testing.TB) string {
	t.Helper()
	directory := t.TempDir()
	Git(t, directory, "init", "--bare", "--quiet")
	return directory
}

// Git runs git in dir with the isolated environment and returns its output.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	return GitInput(t, dir, "", args...)
}

// GitInput runs git in dir with the given standard input.
func GitInput(t testing.TB, dir, input string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), GitEnvironment(t)...)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(bytes.TrimSpace(output))
}

// Ed25519Identity is a fixed Ed25519 key and its encodings.
type Ed25519Identity struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
	PEM     []byte
}

// NewEd25519Identity derives an identity from a one-byte seed, so that two seeds
// give two distinct keys and every run gives the same ones.
func NewEd25519Identity(t testing.TB, seedByte byte) Ed25519Identity {
	t.Helper()
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seedByte}, ed25519.SeedSize))
	encoded, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := private.Public().(ed25519.PublicKey)
	return Ed25519Identity{
		Private: private,
		Public:  public,
		PEM:     pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}),
	}
}

// WriteKeyFile writes the key as an owner-only PEM file.
func (i Ed25519Identity) WriteKeyFile(t testing.TB, directory string) string {
	t.Helper()
	path := filepath.Join(directory, "ed25519.pem")
	if err := os.WriteFile(path, i.PEM, privateFileMode); err != nil {
		t.Fatal(err)
	}
	return path
}
