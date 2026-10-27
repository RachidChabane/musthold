package deviation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The authority of every timestamp of the study, and the media type of its query.
const (
	AuthorityURL     = "http://timestamp.digicert.com"
	QueryContentType = "application/timestamp-query"
	// QuerySuffix and ReplySuffix are appended to a stamped file's name.
	QuerySuffix = ".tsq"
	ReplySuffix = ".tsr"

	partSuffix       = ".part"
	opensslBinary    = "openssl"
	maxReplyBytes    = 1 << 20
	authorityTimeout = time.Minute
	fileMode         = 0o644
)

// Errors of a timestamp that cannot be taken.
var (
	// ErrAuthority is returned when the authority does not answer with a reply.
	ErrAuthority = errors.New("the time-stamp authority did not return a reply")
	// ErrImprint is returned when a reply stamps something other than the file.
	ErrImprint = errors.New("the time-stamp reply stamps another content")
)

// Tool builds the RFC 3161 query of a file.
type Tool interface {
	// Query writes the query of dataFile's SHA-256 to queryFile.
	Query(ctx context.Context, dataFile, queryFile string) error
}

// Authority answers an RFC 3161 query.
type Authority interface {
	Post(ctx context.Context, query []byte) ([]byte, error)
}

// TokenReader reads the imprint and the generation time of a reply file.
type TokenReader interface {
	Read(ctx context.Context, replyFile string) (Token, error)
}

// OpenSSL runs the openssl command line.
type OpenSSL struct {
	// Binary is the command; empty means "openssl".
	Binary string
}

var (
	_ Tool        = OpenSSL{}
	_ TokenReader = OpenSSL{}
)

func (o OpenSSL) binary() string {
	if o.Binary == "" {
		return opensslBinary
	}
	return o.Binary
}

// Query implements Tool.
func (o OpenSSL) Query(ctx context.Context, dataFile, queryFile string) error {
	command := exec.CommandContext(ctx, o.binary(), "ts", "-query", "-data", dataFile, "-sha256", "-cert", "-out", queryFile)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("openssl ts -query: %w: %s", err, output)
	}
	return nil
}

// ReplyText prints a reply as text.
func (o OpenSSL) ReplyText(ctx context.Context, replyFile string) (string, error) {
	command := exec.CommandContext(ctx, o.binary(), "ts", "-reply", "-in", replyFile, "-text")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("openssl ts -reply: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// Read implements TokenReader.
func (o OpenSSL) Read(ctx context.Context, replyFile string) (Token, error) {
	text, err := o.ReplyText(ctx, replyFile)
	if err != nil {
		return Token{}, err
	}
	return ParseToken(text)
}

// HTTPAuthority posts queries to an RFC 3161 service over HTTP.
type HTTPAuthority struct {
	Client *http.Client
	URL    string
}

var _ Authority = HTTPAuthority{}

// Post implements Authority.
func (a HTTPAuthority) Post(ctx context.Context, query []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL, bytes.NewReader(query))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", QueryContentType)
	response, err := a.Client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuthority, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP status %d", ErrAuthority, response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, maxReplyBytes))
}

// Stamper takes the timestamp of a file.
type Stamper struct {
	Tool      Tool
	Reader    TokenReader
	Authority Authority
}

// DefaultStamper stamps with the openssl command line and the study's authority.
func DefaultStamper() Stamper {
	return Stamper{
		Tool:      OpenSSL{},
		Reader:    OpenSSL{},
		Authority: HTTPAuthority{Client: &http.Client{Timeout: authorityTimeout}, URL: AuthorityURL},
	}
}

// QueryPath and ReplyPath name the query and the reply of a stamped file: its name
// with the extension replaced, so that prereg-commit.txt is stamped by
// prereg-commit.tsq and prereg-commit.tsr.
func QueryPath(file string) string { return stem(file) + QuerySuffix }

// ReplyPath names the reply of a stamped file.
func ReplyPath(file string) string { return stem(file) + ReplySuffix }

func stem(file string) string { return strings.TrimSuffix(file, filepath.Ext(file)) }

// Timestamp writes the query and the reply (QueryPath, ReplyPath), the query and the authority's reply
// for the SHA-256 of file, with the default stamper.
func Timestamp(ctx context.Context, file string) (Token, error) {
	return DefaultStamper().Stamp(ctx, file)
}

// Stamp writes the query and the reply of file. The reply is kept only when it is a
// granted token whose imprint is the SHA-256 of the file; on any failure neither
// file is left behind.
func (s Stamper) Stamp(ctx context.Context, file string) (Token, error) {
	queryFile, replyFile := QueryPath(file), ReplyPath(file)
	token, err := s.stamp(ctx, file, queryFile, replyFile)
	if err != nil {
		_ = os.Remove(queryFile)
		_ = os.Remove(replyFile + partSuffix)
		return Token{}, err
	}
	return token, nil
}

func (s Stamper) stamp(ctx context.Context, file, queryFile, replyFile string) (Token, error) {
	if err := s.Tool.Query(ctx, file, queryFile); err != nil {
		return Token{}, err
	}
	query, err := os.ReadFile(queryFile)
	if err != nil {
		return Token{}, err
	}
	reply, err := s.Authority.Post(ctx, query)
	if err != nil {
		return Token{}, err
	}
	part := replyFile + partSuffix
	if err := os.WriteFile(part, reply, fileMode); err != nil {
		return Token{}, err
	}
	token, err := s.Reader.Read(ctx, part)
	if err != nil {
		return Token{}, err
	}
	if err := checkImprint(file, token); err != nil {
		return Token{}, err
	}
	return token, os.Rename(part, replyFile)
}

func checkImprint(file string, token Token) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if want := Imprint(content); token.Imprint != want {
		return fmt.Errorf("%w: %s, not %s", ErrImprint, token.Imprint, want)
	}
	return nil
}

// Imprint is the message imprint a timestamp of content carries: its SHA-256, in
// lowercase hexadecimal.
func Imprint(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
