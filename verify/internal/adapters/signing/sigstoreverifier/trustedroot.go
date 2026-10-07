package sigstoreverifier

import (
	"context"
	_ "embed" // the public-good TUF root is embedded as a byte slice
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/theupdateframework/go-tuf/v2/metadata"
	"github.com/theupdateframework/go-tuf/v2/metadata/fetcher"

	"github.com/RachidChabane/spec-proof/format"
)

// trustedRootTarget is the TUF target that holds the trusted root of Sigstore.
const trustedRootTarget = "trusted_root.json"

// snapshotBaseURL is the base URL the TUF client believes it reads from when it
// reads a snapshot directory. It is never contacted: the snapshot fetcher strips it.
const snapshotBaseURL = "https://tuf-snapshot.invalid"

//go:embed embedded/public-good-root.json
var publicGoodRoot []byte

// PublicGoodRoot returns the TUF root of the public-good Sigstore instance that is
// embedded in the binary. Every trusted root is verified back to it, or to the root
// a caller passes in its place.
func PublicGoodRoot() []byte {
	return append([]byte(nil), publicGoodRoot...)
}

// TrustedRoot is a trusted root that TUF verified, with the digest of its exact
// bytes.
type TrustedRoot struct {
	Material *root.TrustedRoot
	Digest   string
}

// Source says where the TUF repository of the trusted root is read from.
type Source struct {
	// Root is the TUF root every update is verified back to.
	Root []byte
	// Fetcher reads the repository; nil reads the public-good mirror.
	Fetcher fetcher.Fetcher
	// BaseURL is the repository URL the fetcher serves; empty means the public-good
	// mirror when Fetcher is nil.
	BaseURL string
}

// FromSnapshot reads the repository from an exported snapshot directory: the TUF
// metadata and targets exactly as a mirror serves them. No connection is opened.
func FromSnapshot(trustedRootOfTUF []byte, directory string) Source {
	return Source{Root: trustedRootOfTUF, Fetcher: SnapshotFetcher{Directory: directory, BaseURL: snapshotBaseURL}, BaseURL: snapshotBaseURL}
}

// FromMirror reads the repository from the public-good mirror.
func FromMirror(trustedRootOfTUF []byte) Source {
	return Source{Root: trustedRootOfTUF, BaseURL: tuf.DefaultMirror}
}

// Load updates the TUF metadata from the root back to the latest, verifying every
// signature, expiry and hash on the way, and returns the trusted root target. The
// metadata is held in memory: nothing is cached on disk.
func (s Source) Load(ctx context.Context) (TrustedRoot, error) {
	options := &tuf.Options{
		Root:              s.Root,
		RepositoryBaseURL: s.BaseURL,
		DisableLocalCache: true,
		Fetcher:           s.Fetcher,
		Context:           ctx,
	}
	client, err := tuf.New(options)
	if err != nil {
		return TrustedRoot{}, fmt.Errorf("tuf: %w", err)
	}
	raw, err := client.GetTarget(trustedRootTarget)
	if err != nil {
		return TrustedRoot{}, fmt.Errorf("tuf: %w", err)
	}
	material, err := root.NewTrustedRootFromJSON(raw)
	if err != nil {
		return TrustedRoot{}, fmt.Errorf("trusted root: %w", err)
	}
	return TrustedRoot{Material: material, Digest: format.SHA256Hex(raw)}, nil
}

// SnapshotFetcher serves the files of a TUF repository from a directory laid out as
// the repository URL space is: `1.root.json`, `timestamp.json`, `2.snapshot.json`,
// `targets/<digest>.trusted_root.json` and so on.
type SnapshotFetcher struct {
	Directory string
	// BaseURL is the prefix the TUF client puts before every path.
	BaseURL string
}

var _ fetcher.Fetcher = SnapshotFetcher{}

// DownloadFile reads the file the URL names. A file that is absent answers like a
// server would, with status 404, which is how TUF learns that no newer root
// exists; a file longer than maxLength is refused.
func (f SnapshotFetcher) DownloadFile(urlPath string, maxLength int64, _ time.Duration) ([]byte, error) {
	relative, found := strings.CutPrefix(urlPath, strings.TrimSuffix(f.BaseURL, "/")+"/")
	if !found {
		return nil, fmt.Errorf("%q is outside the snapshot %q", urlPath, f.BaseURL)
	}
	local := filepath.FromSlash(path.Clean(relative))
	if !filepath.IsLocal(local) {
		return nil, fmt.Errorf("%q leaves the snapshot", urlPath)
	}
	content, err := os.ReadFile(filepath.Join(f.Directory, local))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, &metadata.ErrDownloadHTTP{StatusCode: http.StatusNotFound, URL: urlPath}
	case err != nil:
		return nil, err
	case int64(len(content)) > maxLength:
		return nil, &metadata.ErrDownloadLengthMismatch{Msg: fmt.Sprintf("%s is longer than %d bytes", relative, maxLength)}
	}
	return content, nil
}
