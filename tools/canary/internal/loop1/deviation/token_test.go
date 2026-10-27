package deviation_test

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
)

const repositoryRoot = "../../../../../"

// replyText renders a reply the way `openssl ts -reply -text` prints it.
func replyText(imprint []byte, stamped string) string {
	var out strings.Builder
	out.WriteString("Using configuration from /opt/homebrew/etc/openssl@3/openssl.cnf\nStatus info:\nStatus: Granted.\nStatus description: unspecified\n")
	out.WriteString("Failure info: unspecified\n\nTST info:\nVersion: 1\nPolicy OID: 2.16.840.1.114412.7.1\nHash Algorithm: sha256\nMessage data:\n")
	for offset := 0; offset < len(imprint); offset += 16 {
		end := min(offset+16, len(imprint))
		out.WriteString(dumpRow(offset, imprint[offset:end]))
	}
	out.WriteString("Serial number: 0xF10F5F280BFD60AEFA4B42120E8D9FB7\nTime stamp: " + stamped + "\nAccuracy: unspecified\nOrdering: no\nNonce: unspecified\nTSA: unspecified\nExtensions:\n")
	return out.String()
}

func dumpRow(offset int, row []byte) string {
	var hexColumn, ascii strings.Builder
	for index := range 16 {
		switch {
		case index >= len(row):
			hexColumn.WriteString("   ")
		default:
			fmt.Fprintf(&hexColumn, "%02x", row[index])
			separator := " "
			if index == 7 {
				separator = "-"
			}
			if index < 15 {
				hexColumn.WriteString(separator)
			}
		}
		if index < len(row) {
			ascii.WriteByte(printable(row[index]))
		}
	}
	return fmt.Sprintf("    %04x - %-47s   %s\n", offset, hexColumn.String(), ascii.String())
}

func printable(b byte) byte {
	if b < 0x20 || b > 0x7e {
		return '.'
	}
	return b
}

func TestParseToken(t *testing.T) {
	t.Run("a granted reply gives its imprint and its generation time [verifies SP-2610~1]", func(t *testing.T) {
		imprint, _ := hex.DecodeString("a7d32d65717859061c4df34f91024bd7b997c1e68873c19acab770d8d7aed93b")
		token, err := deviation.ParseToken(replyText(imprint, "Oct  6 15:55:27 2026 GMT"))
		if err != nil {
			t.Fatal(err)
		}
		if token.Imprint != hex.EncodeToString(imprint) {
			t.Errorf("imprint %s", token.Imprint)
		}
		if want := time.Date(2026, 10, 6, 15, 55, 27, 0, time.UTC); !token.GenTime.Equal(want) {
			t.Errorf("generation time %v", token.GenTime)
		}
	})

	t.Run("a fractional second is accepted [verifies SP-2610~1]", func(t *testing.T) {
		token, err := deviation.ParseToken(replyText(make([]byte, 32), "Oct 26 15:55:27.482 2026 GMT"))
		if err != nil {
			t.Fatal(err)
		}
		if token.GenTime.Nanosecond() != 482_000_000 {
			t.Errorf("generation time %v", token.GenTime)
		}
	})

	t.Run("any imprint of any length survives the dump, ASCII column included [verifies SP-2610~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			imprint := rapid.SliceOfN(rapid.Byte(), 1, 64).Draw(t, "imprint")
			token, err := deviation.ParseToken(replyText(imprint, "Jan  2 03:04:05 2026 GMT"))
			if err != nil {
				t.Fatal(err)
			}
			if token.Imprint != hex.EncodeToString(imprint) {
				t.Fatalf("read %s, rendered %x", token.Imprint, imprint)
			}
		})
	})

	t.Run("a reply that is not granted, not SHA-256 or lacks a member is refused [verifies SP-2610~1]", func(t *testing.T) {
		good := replyText(make([]byte, 32), "Oct 26 15:55:27 2026 GMT")
		for name, text := range map[string]string{
			"rejection":      strings.Replace(good, "Status: Granted.", "Status: Rejection.", 1),
			"sha1":           strings.Replace(good, "Hash Algorithm: sha256", "Hash Algorithm: sha1", 1),
			"no message":     strings.Replace(good, "Message data:", "Message:", 1),
			"no time":        strings.Replace(good, "Time stamp:", "Stamp:", 1),
			"unreadable day": strings.Replace(good, "Oct 26", "Foo 26", 1),
			"empty":          "",
		} {
			if _, err := deviation.ParseToken(text); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}
	})
}

func TestCommittedTokens(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not installed")
	}
	replies, err := filepath.Glob(repositoryRoot + "experiments/*/freeze/*-commit.tsr")
	if err != nil || len(replies) == 0 {
		t.Skipf("no committed token: %v", err)
	}
	t.Run("every committed token's imprint is the SHA-256 of its commit file [verifies SP-2610~1]", func(t *testing.T) {
		for _, reply := range replies {
			content, err := os.ReadFile(strings.TrimSuffix(reply, ".tsr") + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			token, err := deviation.OpenSSL{}.Read(t.Context(), reply)
			if err != nil {
				t.Fatalf("%s: %v", reply, err)
			}
			if want := deviation.Imprint(content); token.Imprint != want {
				t.Errorf("%s: imprint %s, want %s", reply, token.Imprint, want)
			}
		}
	})

	t.Run("the loop-1 pre-registration token was generated at 2026-10-26T15:55:27Z [verifies SP-2610~1]", func(t *testing.T) {
		token, err := deviation.OpenSSL{}.Read(t.Context(), repositoryRoot+"experiments/loop-1/freeze/prereg-commit.tsr")
		if err != nil {
			t.Fatal(err)
		}
		if want := time.Date(2026, 10, 26, 15, 55, 27, 0, time.UTC); !token.GenTime.Equal(want) {
			t.Errorf("generation time %v", token.GenTime)
		}
	})
}
