package systemone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/RachidChabane/musthold/s1relay/internal/app/ports"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/endpoint"
)

// Routes of an Ollama server outside the closed-question route.
const (
	ollamaVersionRoute  = "/api/version"
	ollamaGenerateRoute = "/api/generate"
	// keepAliveNow is the keep_alive value that unloads a model at once.
	keepAliveNow = 0
)

var _ ports.Unloader = (*OllamaUnloader)(nil)

// OllamaUnloader frees the model an Ollama server keeps in memory. It recognises
// Ollama by its version route; any other server is left alone.
type OllamaUnloader struct {
	endpoint endpoint.Endpoint
	model    string
	timeout  time.Duration
	http     *http.Client
}

// NewOllamaUnloader builds the unloader for an endpoint. The model is the one to
// unload when the batch did not learn another.
func NewOllamaUnloader(address endpoint.Endpoint, model string, timeout time.Duration) *OllamaUnloader {
	return &OllamaUnloader{endpoint: address, model: model, timeout: timeout, http: newHTTPClient()}
}

// Unload asks an Ollama server to drop the model with a generate request whose
// keep_alive is zero. A server without the version route is not Ollama.
func (u *OllamaUnloader) Unload(ctx context.Context, model string) (ports.UnloadResult, error) {
	ctx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()
	isOllama, err := u.detect(ctx)
	if err != nil {
		return ports.UnloadResult{}, fmt.Errorf("probe the server: %w", err)
	}
	if !isOllama {
		return ports.UnloadResult{Detail: "the endpoint is not an Ollama server"}, nil
	}
	if model == "" {
		model = u.model
	}
	if model == "" {
		return ports.UnloadResult{Detail: "no model name is known, so none can be unloaded"}, nil
	}
	body, err := json.Marshal(struct {
		Model     string `json:"model"`
		KeepAlive int    `json:"keep_alive"`
	}{Model: model, KeepAlive: keepAliveNow})
	if err != nil {
		return ports.UnloadResult{}, err
	}
	if _, status, err := u.do(ctx, http.MethodPost, ollamaGenerateRoute, body); err != nil {
		return ports.UnloadResult{}, fmt.Errorf("unload %s: %w", model, err)
	} else if status != http.StatusOK {
		return ports.UnloadResult{}, fmt.Errorf("unload %s: HTTP %d", model, status)
	}
	return ports.UnloadResult{Applicable: true}, nil
}

func (u *OllamaUnloader) detect(ctx context.Context) (bool, error) {
	reply, status, err := u.do(ctx, http.MethodGet, ollamaVersionRoute, nil)
	if err != nil {
		return false, err
	}
	if status != http.StatusOK {
		return false, nil
	}
	var version struct {
		Version string `json:"version"`
	}
	return json.Unmarshal(reply, &version) == nil && version.Version != "", nil
}

func (u *OllamaUnloader) do(ctx context.Context, method, route string, body []byte) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, method, u.endpoint.Origin()+route, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set(headerContentType, mediaJSON)
	response, err := u.http.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(response.Body, MaxReplyBytes))
	return reply, response.StatusCode, err
}
