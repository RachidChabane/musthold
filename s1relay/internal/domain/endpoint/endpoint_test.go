package endpoint_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/endpoint"
)

func parse(t *testing.T, raw string) endpoint.Endpoint {
	t.Helper()
	parsed, err := endpoint.Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q): %v", raw, err)
	}
	return parsed
}

func TestParse(t *testing.T) {
	t.Run("a base address gets the closed-question route and a full address is kept [verifies SP-603~1]", func(t *testing.T) {
		cases := map[string]string{
			"http://localhost:11434":                   "http://localhost:11434/v1/systemone",
			"http://localhost:11434/":                  "http://localhost:11434/v1/systemone",
			"https://s1.example.test/custom/systemone": "https://s1.example.test/custom/systemone",
			"  http://localhost:8008  ":                "http://localhost:8008/v1/systemone",
		}
		for raw, want := range cases {
			if got := parse(t, raw).URL(); got != want {
				t.Errorf("Parse(%q).URL() = %q, want %q", raw, got, want)
			}
		}
	})

	t.Run("an address that is empty, not http or without host is refused [verifies SP-601~1]", func(t *testing.T) {
		cases := map[string]error{
			"":                     endpoint.ErrEmpty,
			"   ":                  endpoint.ErrEmpty,
			"ftp://host/x":         endpoint.ErrScheme,
			"localhost:11434":      endpoint.ErrScheme,
			"http:///v1/systemone": endpoint.ErrNoHost,
			"http://[::1":          endpoint.ErrUnparsable,
		}
		for raw, want := range cases {
			if _, err := endpoint.Parse(raw); !errors.Is(err, want) {
				t.Errorf("Parse(%q) = %v, want %v", raw, err, want)
			}
		}
	})
}

func TestDigest(t *testing.T) {
	t.Run("the digest ignores credentials, query, fragment, case and a trailing slash [verifies SP-603~1]", func(t *testing.T) {
		plain := parse(t, "https://s1.example.test:8443/v1/systemone").Digest()
		for _, variant := range []string{
			"https://user:secret@s1.example.test:8443/v1/systemone",
			"https://S1.Example.Test:8443/v1/systemone?key=secret#frag",
			"https://s1.example.test:8443/v1/systemone/",
		} {
			if got := parse(t, variant).Digest(); got != plain {
				t.Errorf("digest of %q = %s, want %s", variant, got, plain)
			}
		}
		if len(plain) != 64 || strings.Contains(plain, "secret") {
			t.Errorf("digest %q is not a SHA-256 in hex", plain)
		}
	})

	t.Run("two different endpoints have two different digests [verifies SP-603~1]", func(t *testing.T) {
		first := parse(t, "http://localhost:11434").Digest()
		for _, other := range []string{"http://localhost:8008", "https://localhost:11434", "http://localhost:11434/other"} {
			if parse(t, other).Digest() == first {
				t.Errorf("%q shares the digest of the first endpoint", other)
			}
		}
	})
}

func TestSource(t *testing.T) {
	t.Run("a source names the model and the endpoint digest, and the model unknown when none is named [verifies SP-600~1]", func(t *testing.T) {
		address := parse(t, "http://localhost:11434")
		if got, want := address.Source("tev1:0.8b"), "systemone:tev1:0.8b@"+address.Digest(); got != want {
			t.Errorf("Source = %q, want %q", got, want)
		}
		if got, want := address.Source(""), "systemone:unknown@"+address.Digest(); got != want {
			t.Errorf("Source = %q, want %q", got, want)
		}
	})

	t.Run("the origin keeps the scheme and host and drops the route [verifies SP-611~1]", func(t *testing.T) {
		if got := parse(t, "http://localhost:11434/v1/systemone").Origin(); got != "http://localhost:11434" {
			t.Errorf("Origin = %q", got)
		}
	})
}
