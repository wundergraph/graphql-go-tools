package searchindex

import (
	"fmt"
	"sync"
)

// IndexFactoryRegistry maps backend names to IndexFactory implementations.
type IndexFactoryRegistry struct {
	mu        sync.RWMutex
	factories map[string]IndexFactory
}

// NewIndexFactoryRegistry creates a new empty registry.
func NewIndexFactoryRegistry() *IndexFactoryRegistry {
	return &IndexFactoryRegistry{
		factories: make(map[string]IndexFactory),
	}
}

// Register adds an IndexFactory for the given backend name.
func (r *IndexFactoryRegistry) Register(backend string, factory IndexFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[backend] = factory
}

// Get returns the IndexFactory for the given backend name.
func (r *IndexFactoryRegistry) Get(backend string) (IndexFactory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[backend]
	if !ok {
		return nil, fmt.Errorf("searchindex: unknown backend %q", backend)
	}
	return f, nil
}

// EmbedderRegistry maps model names to Embedder instances.
type EmbedderRegistry struct {
	mu        sync.RWMutex
	embedders map[string]Embedder
}

// NewEmbedderRegistry creates a new empty embedder registry.
func NewEmbedderRegistry() *EmbedderRegistry {
	return &EmbedderRegistry{
		embedders: make(map[string]Embedder),
	}
}

// Register adds an Embedder for the given model name.
func (r *EmbedderRegistry) Register(model string, embedder Embedder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.embedders[model] = embedder
}

// Get returns the Embedder for the given model name.
func (r *EmbedderRegistry) Get(model string) (Embedder, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.embedders[model]
	if !ok {
		return nil, fmt.Errorf("searchindex: unknown embedder model %q", model)
	}
	return e, nil
}
