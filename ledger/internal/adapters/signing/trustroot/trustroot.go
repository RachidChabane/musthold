// Package trustroot fetches the TUF metadata of the Sigstore trusted root from a
// mirror and keeps every file the TUF client reads, in the layout the mirror serves
// them. The set is what `ledger append --trust` stores and what the verifier reads
// offline: the roots from the embedded one on, the timestamp, snapshot and targets
// metadata and the trusted root target.
package trustroot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/theupdateframework/go-tuf/v2/metadata/fetcher"
)

// trustedRootTarget is the TUF target that holds the trusted root of Sigstore.
const trustedRootTarget = "trusted_root.json"

// PublicGoodMirror is the mirror of the public-good Sigstore instance.
const PublicGoodMirror = tuf.DefaultMirror

// Mirror is a TUF repository to fetch from.
type Mirror struct {
	// Root is the TUF root every update is verified back to.
	Root []byte
	URL  string
	// Fetcher reads the mirror; nil reads it over HTTP.
	Fetcher fetcher.Fetcher
}

// Fetch updates the TUF metadata from the root to the latest, verifying every
// signature, expiry and hash, downloads the trusted root target, and returns every
// file the client read as a path below the mirror and its bytes. The set is only
// returned when the whole update verified.
func (m Mirror) Fetch(ctx context.Context) (map[string][]byte, error) {
	inner := m.Fetcher
	if inner == nil {
		inner = fetcher.NewDefaultFetcher()
	}
	recorder := &recordingFetcher{inner: inner, base: strings.TrimSuffix(m.URL, "/") + "/", files: map[string][]byte{}}
	client, err := tuf.New(&tuf.Options{
		Root:              m.Root,
		RepositoryBaseURL: m.URL,
		DisableLocalCache: true,
		Fetcher:           recorder,
		Context:           ctx,
	})
	if err != nil {
		return nil, fmt.Errorf("tuf: %w", err)
	}
	if _, err := client.GetTarget(trustedRootTarget); err != nil {
		return nil, fmt.Errorf("tuf: %w", err)
	}
	return recorder.snapshot(), nil
}

// recordingFetcher passes every download to the fetcher it wraps and remembers the
// files that arrived.
type recordingFetcher struct {
	inner fetcher.Fetcher
	base  string

	mutex sync.Mutex
	files map[string][]byte
}

// DownloadFile downloads through the wrapped fetcher and records a success.
func (r *recordingFetcher) DownloadFile(urlPath string, maxLength int64, timeout time.Duration) ([]byte, error) {
	content, err := r.inner.DownloadFile(urlPath, maxLength, timeout)
	if err != nil {
		return nil, err
	}
	relative, found := strings.CutPrefix(urlPath, r.base)
	if !found {
		return nil, fmt.Errorf("%q is outside the mirror %q", urlPath, r.base)
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.files[relative] = append([]byte(nil), content...)
	return content, nil
}

func (r *recordingFetcher) snapshot() map[string][]byte {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	copied := make(map[string][]byte, len(r.files))
	for path, content := range r.files {
		copied[path] = content
	}
	return copied
}
