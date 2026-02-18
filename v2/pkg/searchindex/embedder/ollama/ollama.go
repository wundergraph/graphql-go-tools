// Package ollama provides an Embedder implementation backed by the Ollama embed API.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

const (
	defaultBaseURL = "http://localhost:11434"
)

// Embedder implements searchindex.Embedder using the Ollama embed API.
type Embedder struct {
	baseURL    string
	model      string
	dimensions int
	client     *http.Client
}

// Option configures an Embedder.
type Option func(*Embedder)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(e *Embedder) {
		e.client = client
	}
}

// NewEmbedder creates a new Ollama embedder.
//
// baseURL is the Ollama server address (e.g. "http://localhost:11434").
// Pass an empty string to use the default (http://localhost:11434).
//
// model is the Ollama model to use for embeddings (e.g. "nomic-embed-text",
// "mxbai-embed-large", "all-minilm").
//
// dimensions is the dimensionality of the vectors produced by the chosen model.
// The caller must provide this value because Ollama does not report it in the API response.
func NewEmbedder(baseURL, model string, dimensions int, opts ...Option) *Embedder {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	e := &Embedder{
		baseURL:    baseURL,
		model:      model,
		dimensions: dimensions,
		client:     http.DefaultClient,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Dimensions returns the dimensionality of the embeddings produced.
func (e *Embedder) Dimensions() int {
	return e.dimensions
}

// embedRequest is the JSON body sent to the Ollama /api/embed endpoint.
type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embedResponse is the JSON body returned by the Ollama /api/embed endpoint.
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

// Embed converts a batch of texts to embedding vectors.
// The Ollama /api/embed endpoint accepts multiple texts in a single request.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embedRequest{
		Model: e.model,
		Input: texts,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	endpoint := e.baseURL + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var embResp embedResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	if embResp.Error != "" {
		return nil, fmt.Errorf("ollama: api error: %s", embResp.Error)
	}

	if len(embResp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama: expected %d embeddings, got %d", len(texts), len(embResp.Embeddings))
	}

	return embResp.Embeddings, nil
}

// EmbedSingle converts a single text to an embedding vector.
func (e *Embedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("ollama: empty response for single embedding")
	}
	return embeddings[0], nil
}

// Verify interface compliance at compile time.
var _ searchindex.Embedder = (*Embedder)(nil)
