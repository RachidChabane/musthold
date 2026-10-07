package testkit

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/examples/repository/repository"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

const tufValidity = 100 * 365 * 24 * time.Hour

// TrustedRootTarget is the name of the TUF target a trusted root is published as.
const TrustedRootTarget = "trusted_root.json"

// TUFRepository is a TUF repository on disk, laid out as a mirror serves it, that
// holds one target.
type TUFRepository struct {
	// Root is the root the repository's metadata chains to.
	Root []byte
	// Directory holds the files of the repository.
	Directory string
	// Files lists the files a client reads when it starts from Root: everything but
	// the root itself.
	Files []string
}

// NewTUFRepository writes a consistent-snapshot TUF repository whose only target is
// trusted_root.json with the given content.
func NewTUFRepository(t testing.TB, target []byte) TUFRepository {
	t.Helper()
	expires := time.Now().Add(tufValidity)
	roles := repository.New()
	roles.SetTargets(metadata.TARGETS, metadata.Targets(expires))
	roles.SetSnapshot(metadata.Snapshot(expires))
	roles.SetTimestamp(metadata.Timestamp(expires))
	roles.SetRoot(metadata.Root(expires))

	keys := map[string]crypto.Signer{}
	for _, name := range []string{metadata.ROOT, metadata.TARGETS, metadata.SNAPSHOT, metadata.TIMESTAMP} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		keys[name] = key
		tufKey, err := metadata.KeyFromPublicKey(key.Public())
		if err != nil {
			t.Fatal(err)
		}
		if err := roles.Root().Signed.AddKey(tufKey, name); err != nil {
			t.Fatal(err)
		}
	}
	info, err := metadata.TargetFile().FromBytes(TrustedRootTarget, target, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	roles.Targets(metadata.TARGETS).Signed.Targets[TrustedRootTarget] = info
	roles.Snapshot().Signed.Meta["targets.json"] = metadata.MetaFile(1)
	roles.Timestamp().Signed.Meta["snapshot.json"] = metadata.MetaFile(1)

	sign := func(role string, signWith func(signature.Signer) error) {
		signer, err := signature.LoadSigner(keys[role], crypto.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if err := signWith(signer); err != nil {
			t.Fatal(err)
		}
	}
	sign(metadata.TARGETS, func(s signature.Signer) error { _, err := roles.Targets(metadata.TARGETS).Sign(s); return err })
	sign(metadata.SNAPSHOT, func(s signature.Signer) error { _, err := roles.Snapshot().Sign(s); return err })
	sign(metadata.TIMESTAMP, func(s signature.Signer) error { _, err := roles.Timestamp().Sign(s); return err })
	sign(metadata.ROOT, func(s signature.Signer) error { _, err := roles.Root().Sign(s); return err })

	directory := t.TempDir()
	targetFile := "targets/" + hex.EncodeToString(info.Hashes["sha256"]) + "." + TrustedRootTarget
	writers := map[string]func(string) error{
		"1.root.json":     func(path string) error { return roles.Root().ToFile(path, true) },
		"1.targets.json":  func(path string) error { return roles.Targets(metadata.TARGETS).ToFile(path, true) },
		"1.snapshot.json": func(path string) error { return roles.Snapshot().ToFile(path, true) },
		"timestamp.json":  func(path string) error { return roles.Timestamp().ToFile(path, true) },
		targetFile:        func(path string) error { return os.WriteFile(path, target, fileMode) },
	}
	for name, write := range writers {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
			t.Fatal(err)
		}
		if err := write(path); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.ReadFile(filepath.Join(directory, "1.root.json"))
	if err != nil {
		t.Fatal(err)
	}
	return TUFRepository{
		Root: root, Directory: directory,
		Files: []string{"1.snapshot.json", "1.targets.json", "timestamp.json", targetFile},
	}
}
