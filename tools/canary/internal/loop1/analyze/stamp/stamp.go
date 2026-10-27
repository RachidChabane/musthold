// Package stamp reads RFC 3161 timestamp tokens and checks the timestamps of the
// deviation log (PREREG §12): every protocol and token-exposed entry has a
// `deviation-timestamps/<UTC date>-<slug>.txt` file holding its line, and a `.tsr`
// token whose message imprint is the SHA-256 of that file. The token is read
// through the Imprints port; OpenSSL is its adapter.
package stamp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Token is what a timestamp token states.
type Token struct {
	// Algorithm is the hash algorithm of the imprint, as openssl names it.
	Algorithm string
	// Imprint is the message imprint, in lowercase hexadecimal.
	Imprint string
	// Time is the token's time.
	Time time.Time
}

// Imprints reads a timestamp token.
type Imprints interface {
	Read(ctx context.Context, tsr string) (Token, error)
}

// Names of the files of a timestamp.
const (
	TextExtension  = ".txt"
	TokenExtension = ".tsr"
	// SHA256 is openssl's name of the only imprint algorithm the study uses.
	SHA256 = "sha256"
)

// The lines of `openssl ts -reply -text` the reader takes.
const (
	algorithmLabel = "Hash Algorithm:"
	timeLabel      = "Time stamp:"
	messageLabel   = "Message data:"
	timeLayout     = "Jan _2 15:04:05 2006 MST"
)

// dumpLine is one line of openssl's hex dump: an offset, then up to sixteen
// bytes, the eighth and ninth joined by a dash, then their characters.
var dumpLine = regexp.MustCompile(`^\s*[0-9a-f]{4} - ((?:[0-9a-f]{2}[ -]){0,15}[0-9a-f]{2})`)

// ErrToken is returned for text that does not state a token.
var ErrToken = errors.New("the timestamp token cannot be read")

// Parse reads the text that `openssl ts -reply -in X.tsr -text` prints.
func Parse(text string) (Token, error) {
	var token Token
	var imprint strings.Builder
	inMessage := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, algorithmLabel):
			token.Algorithm = strings.TrimSpace(strings.TrimPrefix(trimmed, algorithmLabel))
			inMessage = false
		case strings.HasPrefix(trimmed, timeLabel):
			at, err := time.Parse(timeLayout, strings.TrimSpace(strings.TrimPrefix(trimmed, timeLabel)))
			if err != nil {
				return Token{}, fmt.Errorf("%w: %v", ErrToken, err)
			}
			token.Time = at.UTC()
			inMessage = false
		case trimmed == messageLabel:
			inMessage = true
		case inMessage:
			match := dumpLine.FindStringSubmatch(line)
			if match == nil {
				inMessage = false
				continue
			}
			imprint.WriteString(strings.NewReplacer(" ", "", "-", "").Replace(match[1]))
		}
	}
	token.Imprint = imprint.String()
	if token.Algorithm == "" || token.Imprint == "" || token.Time.IsZero() {
		return Token{}, fmt.Errorf("%w: the algorithm, the message imprint or the time is missing", ErrToken)
	}
	return token, nil
}

// OpenSSL reads tokens with the openssl binary.
type OpenSSL struct {
	// Binary is the openssl executable; empty means openssl on the PATH.
	Binary string
}

const opensslBinary = "openssl"

// Read runs `openssl ts -reply -in <tsr> -text` and parses its output.
func (o OpenSSL) Read(ctx context.Context, tsr string) (Token, error) {
	binary := o.Binary
	if binary == "" {
		binary = opensslBinary
	}
	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(ctx, binary, "ts", "-reply", "-in", tsr, "-text")
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return Token{}, fmt.Errorf("%w: %s: %v: %s", ErrToken, tsr, err, strings.TrimSpace(stderr.String()))
	}
	return Parse(stdout.String())
}

// ErrDeviationStamps refuses a deviation log whose timestamps do not hold.
var ErrDeviationStamps = errors.New("a deviation entry is not timestamped as PREREG §12 requires")

// CheckDeviations checks every protocol and token-exposed entry of the log
// against the timestamp directory, naming every entry at fault.
func CheckDeviations(ctx context.Context, log []byte, dir string, imprints Imprints) error {
	texts, err := readTexts(dir)
	if err != nil {
		return err
	}
	var problems []string
	for _, line := range strings.Split(string(log), "\n") {
		entries := study.ParseDeviations(line)
		if len(entries) != 1 || !entries[0].Kind.Timestamped() {
			continue
		}
		if problem := checkEntry(ctx, strings.TrimSpace(line), texts, imprints); problem != "" {
			problems = append(problems, problem)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrDeviationStamps, strings.Join(problems, "; "))
	}
	return nil
}

// textFile is one timestamped file with its bytes.
type textFile struct {
	path    string
	content []byte
}

func readTexts(dir string) ([]textFile, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*"+TextExtension))
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	texts := make([]textFile, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		texts = append(texts, textFile{path: path, content: content})
	}
	return texts, nil
}

func checkEntry(ctx context.Context, line string, texts []textFile, imprints Imprints) string {
	index := slices.IndexFunc(texts, func(text textFile) bool { return strings.TrimSpace(string(text.content)) == line })
	if index < 0 {
		return fmt.Sprintf("%q has no %s file", line, TextExtension)
	}
	text := texts[index]
	tsr := strings.TrimSuffix(text.path, TextExtension) + TokenExtension
	if _, err := os.Stat(tsr); err != nil {
		return fmt.Sprintf("%s has no token %s", filepath.Base(text.path), filepath.Base(tsr))
	}
	token, err := imprints.Read(ctx, tsr)
	if err != nil {
		return fmt.Sprintf("%s: %v", filepath.Base(tsr), err)
	}
	digest := sha256.Sum256(text.content)
	if token.Algorithm != SHA256 || !strings.EqualFold(token.Imprint, hex.EncodeToString(digest[:])) {
		return fmt.Sprintf("%s: the imprint (%s %s) is not the SHA-256 of %s", filepath.Base(tsr), token.Algorithm, token.Imprint, filepath.Base(text.path))
	}
	return ""
}
