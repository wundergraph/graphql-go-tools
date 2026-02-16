// Package openai provides an Embedder implementation backed by the OpenAI embeddings API.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

const (
	defaultEndpoint = "https://api.openai.com/v1/embeddings"
	maxBatchSize    = 2048
	maxRetries      = 3
	baseRetryDelay  = 500 * time.Millisecond
)

// Embedder implements searchindex.Embedder using the OpenAI embeddings API.
type Embedder struct {
	apiKey     string
	model      string
	dimensions int
	endpoint   string
	client     *http.Client
}

// Option configures an Embedder.
type Option func(*Embedder)

// WithEndpoint overrides the default OpenAI embeddings endpoint.
func WithEndpoint(endpoint string) Option {
	return func(e *Embedder) {
		e.endpoint = endpoint
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(e *Embedder) {
		e.client = client
	}
}

// NewEmbedder creates a new OpenAI embedder.
//
// Supported models and their default dimensions:
//   - "text-embedding-3-small": 1536
//   - "text-embedding-3-large": 3072
//
// The dimensions parameter allows requesting a shorter embedding from the API
// (supported by text-embedding-3-* models). Pass 0 to use the model's default.
func NewEmbedder(apiKey, model string, dimensions int, opts ...Option) *Embedder {
	e := &Embedder{
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		endpoint:   defaultEndpoint,
		client:     http.DefaultClient,
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.dimensions == 0 {
		switch model {
		case "text-embedding-3-small":
			e.dimensions = 1536
		case "text-embedding-3-large":
			e.dimensions = 3072
		default:
			e.dimensions = 1536
		}
	}
	return e
}

// Dimensions returns the dimensionality of the embeddings produced.
func (e *Embedder) Dimensions() int {
	return e.dimensions
}

// embeddingRequest is the JSON body sent to the OpenAI API.
type embeddingRequest struct {
	Input      []string `json:"input"`
	Model      string   `json:"model"`
	Dimensions int      `json:"dimensions,omitempty"`
}

// embeddingResponse is the JSON body returned by the OpenAI API.
type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *apiError       `json:"error,omitempty"`
}

type embeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Embed converts a batch of texts to embedding vectors.
// Texts are split into sub-batches of up to 2048 items per API request.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	results := make([][]float32, len(texts))

	for start := 0; start < len(texts); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]

		embeddings, err := e.requestWithRetry(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("openai embed batch [%d:%d]: %w", start, end, err)
		}

		for _, item := range embeddings {
			results[start+item.Index] = item.Embedding
		}
	}

	return results, nil
}

// EmbedSingle converts a single text to an embedding vector.
func (e *Embedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("openai: empty response for single embedding")
	}
	return embeddings[0], nil
}

// requestWithRetry sends the embedding request with exponential backoff on rate-limit errors.
func (e *Embedder) requestWithRetry(ctx context.Context, texts []string) ([]embeddingData, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		data, err := e.doRequest(ctx, texts)
		if err == nil {
			return data, nil
		}

		if isRetryable(err) {
			lastErr = err
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("openai: max retries exceeded: %w", lastErr)
}

// rateLimitError indicates a 429 response.
type rateLimitError struct {
	message string
}

func (e *rateLimitError) Error() string {
	return e.message
}

// serverError indicates a 5xx response.
type serverError struct {
	statusCode int
	message    string
}

func (e *serverError) Error() string {
	return e.message
}

func isRetryable(err error) bool {
	switch err.(type) {
	case *rateLimitError, *serverError:
		return true
	default:
		return false
	}
}

func (e *Embedder) doRequest(ctx context.Context, texts []string) ([]embeddingData, error) {
	reqBody := embeddingRequest{
		Input: texts,
		Model: e.model,
	}
	// Only include dimensions for text-embedding-3-* models that support it.
	if e.dimensions > 0 {
		reqBody.Dimensions = e.dimensions
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &rateLimitError{message: fmt.Sprintf("openai: rate limited (429): %s", string(respBody))}
	}

	if resp.StatusCode >= 500 {
		return nil, &serverError{
			statusCode: resp.StatusCode,
			message:    fmt.Sprintf("openai: server error (%d): %s", resp.StatusCode, string(respBody)),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if embResp.Error != nil {
		return nil, fmt.Errorf("openai: api error: %s (%s)", embResp.Error.Message, embResp.Error.Type)
	}

	return embResp.Data, nil
}

// Verify interface compliance at compile time.
var _ searchindex.Embedder = (*Embedder)(nil)
