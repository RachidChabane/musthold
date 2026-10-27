package deviation_test

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
)

// fakeTool stands for openssl: a query is the file's imprint, a reply's text
// stamps whatever imprint the reply file holds.
type fakeTool struct {
	queryErr error
	stamped  string
}

func (f fakeTool) Query(_ context.Context, dataFile, queryFile string) error {
	if f.queryErr != nil {
		return f.queryErr
	}
	content, err := os.ReadFile(dataFile)
	if err != nil {
		return err
	}
	return os.WriteFile(queryFile, []byte(deviation.Imprint(content)), 0o644)
}

func (f fakeTool) Read(_ context.Context, replyFile string) (deviation.Token, error) {
	imprint, err := os.ReadFile(replyFile)
	if err != nil {
		return deviation.Token{}, err
	}
	stamped, _ := time.Parse(time.RFC3339, f.stamped)
	return deviation.Token{Imprint: string(imprint), GenTime: stamped}, nil
}

// echoAuthority replies to a query with the imprint it holds; wrong replies with
// another one.
type echoAuthority struct {
	queries *[][]byte
	wrong   bool
	err     error
}

func (e echoAuthority) Post(_ context.Context, query []byte) ([]byte, error) {
	if e.queries != nil {
		*e.queries = append(*e.queries, query)
	}
	if e.err != nil {
		return nil, e.err
	}
	if e.wrong {
		return []byte(hex.EncodeToString(make([]byte, 32))), nil
	}
	return query, nil
}

func stamper(tool fakeTool, authority echoAuthority) deviation.Stamper {
	return deviation.Stamper{Tool: tool, Reader: tool, Authority: authority}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStamp(t *testing.T) {
	t.Run("a file is stamped by a query and a reply that replace its extension [verifies SP-2611~1]", func(t *testing.T) {
		file := writeFile(t, "prereg-commit.txt", "abc\n")
		var queries [][]byte
		token, err := stamper(fakeTool{stamped: "2026-10-26T15:55:27Z"}, echoAuthority{queries: &queries}).Stamp(t.Context(), file)
		if err != nil {
			t.Fatal(err)
		}
		if token.Imprint != deviation.Imprint([]byte("abc\n")) || len(queries) != 1 {
			t.Errorf("token %+v, %d queries", token, len(queries))
		}
		dir := filepath.Dir(file)
		for _, name := range []string{"prereg-commit.tsq", "prereg-commit.tsr"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	})

	t.Run("a reply that stamps another content leaves neither file [verifies SP-2611~1]", func(t *testing.T) {
		file := writeFile(t, "a.txt", "abc\n")
		_, err := stamper(fakeTool{}, echoAuthority{wrong: true}).Stamp(t.Context(), file)
		if !errors.Is(err, deviation.ErrImprint) {
			t.Fatalf("error %v", err)
		}
		assertOnlyFile(t, file)
	})

	t.Run("an authority or a tool that fails leaves neither file [verifies SP-2611~1]", func(t *testing.T) {
		file := writeFile(t, "a.txt", "abc\n")
		if _, err := stamper(fakeTool{}, echoAuthority{err: deviation.ErrAuthority}).Stamp(t.Context(), file); !errors.Is(err, deviation.ErrAuthority) {
			t.Fatalf("authority: %v", err)
		}
		assertOnlyFile(t, file)
		failure := errors.New("no openssl")
		if _, err := stamper(fakeTool{queryErr: failure}, echoAuthority{}).Stamp(t.Context(), file); !errors.Is(err, failure) {
			t.Fatalf("tool: %v", err)
		}
		assertOnlyFile(t, file)
	})
}

func assertOnlyFile(t *testing.T, file string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(file) {
		t.Errorf("the directory holds %v", entries)
	}
}

func TestHTTPAuthority(t *testing.T) {
	t.Run("the query is posted as a time-stamp query and the body is the reply [verifies SP-2612~1]", func(t *testing.T) {
		var method, contentType string
		var body []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method, contentType = r.Method, r.Header.Get("Content-Type")
			body, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte("reply"))
		}))
		defer server.Close()
		reply, err := deviation.HTTPAuthority{Client: server.Client(), URL: server.URL}.Post(t.Context(), []byte("query"))
		if err != nil {
			t.Fatal(err)
		}
		if method != http.MethodPost || contentType != "application/timestamp-query" || string(body) != "query" || string(reply) != "reply" {
			t.Errorf("%s %q %q -> %q", method, contentType, body, reply)
		}
	})

	t.Run("a status other than 200 is refused [verifies SP-2612~1]", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "busy", http.StatusServiceUnavailable)
		}))
		defer server.Close()
		_, err := deviation.HTTPAuthority{Client: server.Client(), URL: server.URL}.Post(t.Context(), nil)
		if !errors.Is(err, deviation.ErrAuthority) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("the default stamper asks the study's authority [verifies SP-2612~1]", func(t *testing.T) {
		authority, ok := deviation.DefaultStamper().Authority.(deviation.HTTPAuthority)
		if !ok || authority.URL != "http://timestamp.digicert.com" {
			t.Errorf("authority %+v", deviation.DefaultStamper().Authority)
		}
	})
}

func TestOpenSSLQuery(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not installed")
	}
	t.Run("the query is built locally for the file's SHA-256 [verifies SP-2611~1]", func(t *testing.T) {
		file := writeFile(t, "a.txt", "abc\n")
		query := filepath.Join(filepath.Dir(file), "a.tsq")
		if err := (deviation.OpenSSL{}).Query(t.Context(), file, query); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(query)
		if err != nil || len(content) == 0 {
			t.Fatalf("query %v, %v", content, err)
		}
		if err := (deviation.OpenSSL{Binary: "/nonexistent/openssl"}).Query(t.Context(), file, query); err == nil {
			t.Error("a missing binary was accepted")
		}
	})
}
