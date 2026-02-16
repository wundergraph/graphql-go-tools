package searchtesting

import (
	"context"
	"hash/fnv"
	"math"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

const mockDimensions = 4

// MockEmbedder implements searchindex.Embedder with deterministic hash-based vectors.
// Given the same text, it always produces the same 4-dimensional unit vector.
type MockEmbedder struct{}

var _ searchindex.Embedder = (*MockEmbedder)(nil)

func (m *MockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		result[i] = hashToVector(text)
	}
	return result, nil
}

func (m *MockEmbedder) EmbedSingle(_ context.Context, text string) ([]float32, error) {
	return hashToVector(text), nil
}

func (m *MockEmbedder) Dimensions() int {
	return mockDimensions
}

// hashToVector produces a deterministic 4-dimensional unit vector from any string.
func hashToVector(text string) []float32 {
	h := fnv.New64a()
	h.Write([]byte(text))
	seed := h.Sum64()

	vec := make([]float32, mockDimensions)
	var norm float64
	for i := 0; i < mockDimensions; i++ {
		// Mix bits for each dimension using different shifts
		mixed := seed ^ (seed >> uint(16+i*8))
		// Map to [-1, 1]
		val := float64(int64(mixed)) / float64(math.MaxInt64)
		vec[i] = float32(val)
		norm += val * val
	}
	// Normalize to unit vector for cosine distance consistency
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}
	return vec
}
