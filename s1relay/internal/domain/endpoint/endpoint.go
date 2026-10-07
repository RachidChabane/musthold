// Package endpoint holds the value object of a System One endpoint: where its
// closed-question route is, and the digest and source name that identify it in a
// proof without revealing credentials.
package endpoint

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// Route is the path of the closed-question route of a System One endpoint.
const Route = "/v1/systemone"

// SourcePrefix starts the source name a relayed answer is imported under.
const SourcePrefix = "systemone:"

// UnknownModel names the model of an answer when neither the configuration nor the
// endpoint named one.
const UnknownModel = "unknown"

const (
	sourceSeparator = "@"
	schemeHTTP      = "http"
	schemeHTTPS     = "https"
	pathRoot        = "/"
)

// Errors of an endpoint address that cannot be used.
var (
	ErrEmpty      = errors.New("the endpoint is empty")
	ErrScheme     = errors.New("the endpoint must be an http or https URL")
	ErrNoHost     = errors.New("the endpoint names no host")
	ErrUnparsable = errors.New("the endpoint is not a URL")
)

// Endpoint is a System One endpoint address.
type Endpoint struct {
	address *url.URL
}

// Parse reads the configured address of an endpoint. An address without a path is
// the base of a server and gets the closed-question route; an address with a path
// is the route itself.
func Parse(raw string) (Endpoint, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Endpoint{}, ErrEmpty
	}
	address, err := url.Parse(trimmed)
	if err != nil {
		return Endpoint{}, fmt.Errorf("%w: %v", ErrUnparsable, err)
	}
	if address.Scheme != schemeHTTP && address.Scheme != schemeHTTPS {
		return Endpoint{}, fmt.Errorf("%w: %q", ErrScheme, address.Scheme)
	}
	if address.Hostname() == "" {
		return Endpoint{}, ErrNoHost
	}
	address.Fragment = ""
	if address.Path == "" || address.Path == pathRoot {
		address.Path = Route
	}
	return Endpoint{address: address}, nil
}

// URL is the address a question set is posted to.
func (e Endpoint) URL() string { return e.address.String() }

// Origin is the scheme and host of the endpoint, where its server-level routes
// live.
func (e Endpoint) Origin() string {
	return (&url.URL{Scheme: e.address.Scheme, Host: e.address.Host, User: e.address.User}).String()
}

// canonical is the address with credentials, query and fragment removed, the
// scheme and host in lower case and the path cleaned.
func (e Endpoint) canonical() string {
	clean := path.Clean(e.address.Path)
	return (&url.URL{Scheme: strings.ToLower(e.address.Scheme), Host: strings.ToLower(e.address.Host), Path: clean}).String()
}

// Digest identifies the endpoint: the SHA-256, in hex, of its canonical address.
// It carries no credential, so it may be recorded in a proof.
func (e Endpoint) Digest() string {
	sum := sha256.Sum256([]byte(e.canonical()))
	return hex.EncodeToString(sum[:])
}

// Source is the name a relayed answer is imported under: the model and the digest
// of the endpoint it came from.
func (e Endpoint) Source(model string) string {
	if model == "" {
		model = UnknownModel
	}
	return SourcePrefix + model + sourceSeparator + e.Digest()
}
