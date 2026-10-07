package sigstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/sigstore/sigstore-go/pkg/sign"
)

// Public-good Sigstore services and the audience of the CI identity token.
const (
	PublicFulcioURL = "https://fulcio.sigstore.dev"
	PublicRekorURL  = "https://rekor.sigstore.dev"
	IDTokenAudience = "sigstore"

	// IDTokenEnvironmentVariable carries an identity token handed in explicitly.
	IDTokenEnvironmentVariable = "SIGSTORE_ID_TOKEN"
	// The GitHub Actions variables that let a job with `id-token: write` request
	// an identity token.
	ActionsRequestURLVariable   = "ACTIONS_ID_TOKEN_REQUEST_URL"
	ActionsRequestTokenVariable = "ACTIONS_ID_TOKEN_REQUEST_TOKEN"

	tokenRequestTimeout = 30 * time.Second
	maxTokenResponse    = 1 << 20
	bearerPrefix        = "Bearer "
)

// ErrNoIDToken is returned when the environment offers no identity token.
var ErrNoIDToken = errors.New("no OIDC identity token: set " + IDTokenEnvironmentVariable +
	" or run in a job with `id-token: write`")

// KeylessConfig says where keyless signing gets its certificate and its log entry.
type KeylessConfig struct {
	FulcioURL string
	RekorURL  string
	IDToken   string
}

// NewKeyless returns a signer that obtains a Fulcio certificate for a fresh
// ephemeral key and a Rekor entry for every statement.
func NewKeyless(config KeylessConfig) (*Signer, error) {
	if config.IDToken == "" {
		return nil, ErrNoIDToken
	}
	options := sign.BundleOptions{
		CertificateProvider:        sign.NewFulcio(&sign.FulcioOptions{BaseURL: config.FulcioURL}),
		CertificateProviderOptions: &sign.CertificateProviderOptions{IDToken: config.IDToken},
		TransparencyLogs:           []sign.Transparency{sign.NewRekor(&sign.RekorOptions{BaseURL: config.RekorURL})},
	}
	factory := func() (sign.Keypair, error) { return sign.NewEphemeralKeypair(nil) }
	return NewSigner(factory, options), nil
}

// TokenSource finds the OIDC identity token of the job.
type TokenSource struct {
	LookupEnv func(string) (string, bool)
	Client    *http.Client
}

// Token returns the token handed in explicitly, or requests one from the GitHub
// Actions token service.
func (s TokenSource) Token(ctx context.Context) (string, error) {
	if token, found := s.LookupEnv(IDTokenEnvironmentVariable); found && token != "" {
		return token, nil
	}
	requestURL, hasURL := s.LookupEnv(ActionsRequestURLVariable)
	requestToken, hasToken := s.LookupEnv(ActionsRequestTokenVariable)
	if !hasURL || !hasToken || requestURL == "" || requestToken == "" {
		return "", ErrNoIDToken
	}
	return s.requestActionsToken(ctx, requestURL, requestToken)
}

func (s TokenSource) requestActionsToken(ctx context.Context, requestURL, requestToken string) (string, error) {
	parsed, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("token request URL: %w", err)
	}
	query := parsed.Query()
	query.Set("audience", IDTokenAudience)
	parsed.RawQuery = query.Encode()

	ctx, cancel := context.WithTimeout(ctx, tokenRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", bearerPrefix+requestToken)
	client := s.Client
	if client == nil {
		client = &http.Client{}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("request identity token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request identity token: status %d", response.StatusCode)
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxTokenResponse)).Decode(&body); err != nil {
		return "", fmt.Errorf("identity token response: %w", err)
	}
	if body.Value == "" {
		return "", errors.New("identity token response carries no token")
	}
	return body.Value, nil
}
