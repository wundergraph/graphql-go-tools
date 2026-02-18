package searchindex

import "context"

// Embedder converts text to vector embeddings. Pluggable: OpenAI, Ollama, local models.
type Embedder interface {
	// Embed converts a batch of texts to embedding vectors.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedSingle converts a single text to an embedding vector.
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
	// Dimensions returns the dimensionality of the embeddings produced by this embedder.
	Dimensions() int
}

// TextTransformer converts entity fields into a string for embedding.
type TextTransformer interface {
	Transform(fields map[string]any) string
}

// FuncTransformer allows arbitrary Go functions for programmatic use.
type FuncTransformer struct {
	Fn func(fields map[string]any) string
}

// Transform calls the underlying function.
func (f *FuncTransformer) Transform(fields map[string]any) string {
	return f.Fn(fields)
}

// EmbeddingPipeline combines a transformer and embedder for derived embeddings.
type EmbeddingPipeline struct {
	Transformer TextTransformer
	Embedder    Embedder
}

// Process converts entity fields → string → embedding vector.
func (p *EmbeddingPipeline) Process(ctx context.Context, fields map[string]any) ([]float32, error) {
	text := p.Transformer.Transform(fields)
	return p.Embedder.EmbedSingle(ctx, text)
}

// ProcessBatch converts multiple entities' fields → strings → embedding vectors.
func (p *EmbeddingPipeline) ProcessBatch(ctx context.Context, fieldSets []map[string]any) ([][]float32, error) {
	texts := make([]string, len(fieldSets))
	for i, fields := range fieldSets {
		texts[i] = p.Transformer.Transform(fields)
	}
	return p.Embedder.Embed(ctx, texts)
}
