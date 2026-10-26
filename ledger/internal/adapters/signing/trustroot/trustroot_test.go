package trustroot_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/ledger/internal/adapters/signing/trustroot"
	"github.com/RachidChabane/musthold/ledger/internal/testkit"
)

const verifiesFetch = " [verifies SP-226~1]"

var trustedRoot = []byte(`{"mediaType":"application/vnd.dev.sigstore.trustedroot+json;version=0.1"}`)

func TestFetch(t *testing.T) {
	repository := testkit.NewTUFRepository(t, trustedRoot)

	t.Run("every file the TUF client read is returned in the layout of the mirror"+verifiesFetch, func(t *testing.T) {
		server := httptest.NewServer(http.FileServer(http.Dir(repository.Directory)))
		defer server.Close()
		files, err := trustroot.Mirror{Root: repository.Root, URL: server.URL}.Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for path := range files {
			got = append(got, path)
		}
		slices.Sort(got)
		want := slices.Clone(repository.Files)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("files %v, want %v", got, want)
		}
		for path, content := range files {
			if strings.HasSuffix(path, testkit.TrustedRootTarget) && string(content) != string(trustedRoot) {
				t.Fatalf("target %s holds %q", path, content)
			}
		}
	})

	t.Run("a mirror that serves a tampered target gives no snapshot"+verifiesFetch, func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := httptest.NewRecorder()
			http.FileServer(http.Dir(repository.Directory)).ServeHTTP(recorder, r)
			body := recorder.Body.Bytes()
			if strings.HasSuffix(r.URL.Path, testkit.TrustedRootTarget) {
				body = append(body, ' ')
			}
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(body)
		}))
		defer server.Close()
		if files, err := (trustroot.Mirror{Root: repository.Root, URL: server.URL}).Fetch(context.Background()); err == nil {
			t.Fatalf("a tampered mirror gave %d files", len(files))
		}
	})

	t.Run("a repository that does not chain to the given root gives no snapshot"+verifiesFetch, func(t *testing.T) {
		server := httptest.NewServer(http.FileServer(http.Dir(repository.Directory)))
		defer server.Close()
		other := testkit.NewTUFRepository(t, trustedRoot)
		if _, err := (trustroot.Mirror{Root: other.Root, URL: server.URL}).Fetch(context.Background()); err == nil {
			t.Fatal("a repository signed by unknown keys was accepted")
		}
	})

	t.Run("an unreachable mirror is a failure"+verifiesFetch, func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		url := server.URL
		server.Close()
		if _, err := (trustroot.Mirror{Root: repository.Root, URL: url}).Fetch(context.Background()); err == nil {
			t.Fatal("fetched from nowhere")
		}
	})
}
