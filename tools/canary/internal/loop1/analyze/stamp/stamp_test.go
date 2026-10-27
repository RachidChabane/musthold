package stamp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// opensslText is what `openssl ts -reply -text` prints for the token of
// experiments/loop-1/freeze/prereg-commit.tsr.
const opensslText = `Using configuration from /opt/homebrew/etc/openssl@3/openssl.cnf
Status info:
Status: Granted.
Status description: unspecified
Failure info: unspecified

TST info:
Version: 1
Policy OID: 2.16.840.1.114412.7.1
Hash Algorithm: sha256
Message data:
    0000 - a7 d3 2d 65 71 78 59 06-1c 4d f3 4f 91 02 4b d7   ..-eqxY..M.O..K.
    0010 - b9 97 c1 e6 88 73 c1 9a-ca b7 70 d8 d7 ae d9 3b   .....s....p....;
Serial number: 0xF10F5F280BFD60AEFA4B42120E8D9FB7
Time stamp: Oct 26 15:55:27 2026 GMT
Accuracy: unspecified
Ordering: no
Nonce: unspecified
TSA: unspecified
Extensions:
`

func TestParse(t *testing.T) {
	t.Run("the algorithm, the imprint and the time are read from openssl's text [verifies SP-2586~1]", func(t *testing.T) {
		token, err := Parse(opensslText)
		if err != nil {
			t.Fatal(err)
		}
		if token.Algorithm != SHA256 || token.Imprint != "a7d32d65717859061c4df34f91024bd7b997c1e68873c19acab770d8d7aed93b" ||
			!token.Time.Equal(time.Date(2026, 10, 26, 15, 55, 27, 0, time.UTC)) {
			t.Fatalf("got %+v", token)
		}
	})
	t.Run("a short last line and a single-digit day are read [verifies SP-2586~1]", func(t *testing.T) {
		text := "Hash Algorithm: sha1\nMessage data:\n    0000 - 0a 0b 0c 0d 0e 0f 10 11-12 13 14 15 16 17 18 19   ................\n" +
			"    0010 - 1a ab cd 1d                                       ..ab\nSerial number: 0x1\nTime stamp: Nov  3 01:02:03.5 2026 GMT\n"
		token, err := Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		if token.Imprint != "0a0b0c0d0e0f101112131415161718191aabcd1d" || token.Algorithm != "sha1" || token.Time.Day() != 3 {
			t.Fatalf("got %+v", token)
		}
	})
	t.Run("text without an imprint is refused [verifies SP-2586~1]", func(t *testing.T) {
		if _, err := Parse("Status: Rejection\n"); !errors.Is(err, ErrToken) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("the openssl adapter reads a committed token [verifies SP-2586~1]", func(t *testing.T) {
		token, err := OpenSSL{}.Read(context.Background(), "../../../../../../experiments/loop-1/freeze/prereg-commit.tsr")
		if errors.Is(err, ErrToken) && strings.Contains(err.Error(), "executable file not found") {
			t.Skip("openssl is not installed")
		}
		if err != nil {
			t.Fatal(err)
		}
		text, _ := os.ReadFile("../../../../../../experiments/loop-1/freeze/prereg-commit.txt")
		digest := sha256.Sum256(text)
		if token.Imprint != hex.EncodeToString(digest[:]) {
			t.Fatalf("imprint %s", token.Imprint)
		}
	})
}

// fakeImprints answers from a map of token paths to tokens.
type fakeImprints map[string]Token

func (f fakeImprints) Read(_ context.Context, tsr string) (Token, error) {
	token, found := f[filepath.Base(tsr)]
	if !found {
		return Token{}, ErrToken
	}
	return token, nil
}

const (
	protocolLine = "- 2026-11-03T08:00:00Z | protocol | all | §6.5 | the batch resumes inside the window | the operator resumed it at 08:00"
	exposedLine  = "- 2026-11-04T08:00:00Z | token-exposed | a-control-1 | §7.5 | the token is never in a record | stderr.txt held it"
	unshapedLine = "the operator paused the batch for a day"
	quotaLine    = "- 2026-11-05T08:00:00Z | quota-stop | a-control-2 | §6.5 | a quota stop voids the run | 429"
)

func digestOf(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

// stampDir writes each entry's file and token, and returns the matching tokens.
func stampDir(t *testing.T, entries map[string]string) (string, fakeImprints) {
	t.Helper()
	dir := t.TempDir()
	tokens := fakeImprints{}
	for name, line := range entries {
		content := line + "\n"
		if err := os.WriteFile(filepath.Join(dir, name+TextExtension), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+TokenExtension), []byte("token"), 0o644); err != nil {
			t.Fatal(err)
		}
		tokens[name+TokenExtension] = Token{Algorithm: SHA256, Imprint: digestOf(content), Time: time.Now()}
	}
	return dir, tokens
}

func TestCheckDeviations(t *testing.T) {
	log := []byte("# Deviations\n\n" + protocolLine + "\n" + exposedLine + "\n" + unshapedLine + "\n" + quotaLine + "\n")
	all := map[string]string{
		"2026-11-03-resumed": protocolLine,
		"2026-11-04-token":   exposedLine,
		"2026-11-05-paused":  unshapedLine,
	}
	t.Run("every protocol and token-exposed entry, unshaped lines included, has its file and token [verifies SP-2584~1]", func(t *testing.T) {
		dir, tokens := stampDir(t, all)
		if err := CheckDeviations(context.Background(), log, dir, tokens); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a missing file or token is refused, naming the entry [verifies SP-2584~1]", func(t *testing.T) {
		dir, tokens := stampDir(t, map[string]string{"2026-11-03-resumed": protocolLine, "2026-11-04-token": exposedLine})
		if err := os.Remove(filepath.Join(dir, "2026-11-04-token"+TokenExtension)); err != nil {
			t.Fatal(err)
		}
		err := CheckDeviations(context.Background(), log, dir, tokens)
		if !errors.Is(err, ErrDeviationStamps) || !strings.Contains(err.Error(), "paused the batch") ||
			!strings.Contains(err.Error(), "2026-11-04-token.tsr") || strings.Contains(err.Error(), "quota-stop") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("an imprint that is not the SHA-256 of its file is refused [verifies SP-2585~1]", func(t *testing.T) {
		dir, tokens := stampDir(t, all)
		tokens["2026-11-03-resumed"+TokenExtension] = Token{Algorithm: SHA256, Imprint: digestOf(protocolLine)}
		tokens["2026-11-04-token"+TokenExtension] = Token{Algorithm: "sha1", Imprint: digestOf(exposedLine + "\n")}
		err := CheckDeviations(context.Background(), log, dir, tokens)
		if !errors.Is(err, ErrDeviationStamps) || !strings.Contains(err.Error(), "2026-11-03-resumed.tsr") || !strings.Contains(err.Error(), "2026-11-04-token.tsr") {
			t.Fatalf("got %v", err)
		}
	})
}
