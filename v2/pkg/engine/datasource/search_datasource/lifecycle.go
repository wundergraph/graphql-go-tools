package search_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// GraphQLExecutor executes GraphQL operations against the federated graph.
type GraphQLExecutor interface {
	Execute(ctx context.Context, operation string) ([]byte, error)
}

// GraphQLSubscriber subscribes to GraphQL operations and streams events.
// Each received []byte is a complete JSON response for one event.
type GraphQLSubscriber interface {
	Subscribe(ctx context.Context, operation string) (<-chan []byte, error)
}

// Manager handles the lifecycle of search indices: creation, population, subscriptions, and shutdown.
type Manager struct {
	factory          *Factory
	indexRegistry    *searchindex.IndexFactoryRegistry
	embedderRegistry *searchindex.EmbedderRegistry
	executor         GraphQLExecutor
	subscriber       GraphQLSubscriber
	config           *ParsedConfig

	indices   map[string]searchindex.Index
	pipelines map[string]map[string]*searchindex.EmbeddingPipeline // entity type → field name → pipeline

	cancelFuncs []context.CancelFunc
	mu          sync.Mutex
}

// NewManager creates a new lifecycle manager.
func NewManager(
	factory *Factory,
	indexRegistry *searchindex.IndexFactoryRegistry,
	embedderRegistry *searchindex.EmbedderRegistry,
	executor GraphQLExecutor,
	config *ParsedConfig,
) *Manager {
	return &Manager{
		factory:          factory,
		indexRegistry:    indexRegistry,
		embedderRegistry: embedderRegistry,
		executor:         executor,
		config:           config,
		indices:          make(map[string]searchindex.Index),
		pipelines:        make(map[string]map[string]*searchindex.EmbeddingPipeline),
	}
}

// SetSubscriber sets the optional subscriber for live index updates.
func (m *Manager) SetSubscriber(subscriber GraphQLSubscriber) {
	m.subscriber = subscriber
}

// Start creates indices, runs initial population, and starts subscriptions.
func (m *Manager) Start(ctx context.Context) error {
	// Create indices
	if err := m.createIndices(ctx); err != nil {
		return fmt.Errorf("creating indices: %w", err)
	}

	// Setup embedding pipelines
	if err := m.setupEmbeddingPipelines(); err != nil {
		return fmt.Errorf("setting up embedding pipelines: %w", err)
	}

	// Run initial population queries
	if err := m.runPopulations(ctx); err != nil {
		return fmt.Errorf("running populations: %w", err)
	}

	// Start subscriptions for live updates
	m.startSubscriptions(ctx)

	return nil
}

// Stop cancels all subscriptions and closes all indices.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cancel := range m.cancelFuncs {
		cancel()
	}
	m.cancelFuncs = nil

	var firstErr error
	for name, idx := range m.indices {
		if err := idx.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("closing index %s: %w", name, err)
		}
	}
	return firstErr
}

// GetIndex returns the index for the given name.
func (m *Manager) GetIndex(name string) (searchindex.Index, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := m.indices[name]
	return idx, ok
}

func (m *Manager) createIndices(ctx context.Context) error {
	for _, idxDir := range m.config.Indices {
		factory, err := m.indexRegistry.Get(idxDir.Backend)
		if err != nil {
			return fmt.Errorf("backend %q: %w", idxDir.Backend, err)
		}

		// Build schema from entity fields
		schema := m.buildIndexSchema(idxDir.Name)

		idx, err := factory.CreateIndex(ctx, idxDir.Name, schema, []byte(idxDir.ConfigJSON))
		if err != nil {
			return fmt.Errorf("creating index %q: %w", idxDir.Name, err)
		}

		m.indices[idxDir.Name] = idx
		m.factory.RegisterIndex(idxDir.Name, idx)
	}
	return nil
}

func (m *Manager) buildIndexSchema(indexName string) searchindex.IndexConfig {
	schema := searchindex.IndexConfig{Name: indexName}
	for _, entity := range m.config.Entities {
		if entity.IndexName != indexName {
			continue
		}
		for _, f := range entity.Fields {
			schema.Fields = append(schema.Fields, searchindex.FieldConfig{
				Name:       f.FieldName,
				Type:       f.IndexType,
				Filterable: f.Filterable,
				Sortable:   f.Sortable,
				Dimensions: f.Dimensions,
				Weight:     f.Weight,
			})
		}
		// Add embedding fields as vector fields, resolving dimensions from the embedder.
		for _, ef := range entity.EmbeddingFields {
			dims := 0
			if m.embedderRegistry != nil {
				if embedder, err := m.embedderRegistry.Get(ef.Model); err == nil {
					dims = embedder.Dimensions()
				}
			}
			schema.Fields = append(schema.Fields, searchindex.FieldConfig{
				Name:       ef.FieldName,
				Type:       searchindex.FieldTypeVector,
				Dimensions: dims,
			})
		}
	}
	return schema
}

func (m *Manager) setupEmbeddingPipelines() error {
	for _, entity := range m.config.Entities {
		for _, ef := range entity.EmbeddingFields {
			transformer, err := searchindex.NewTemplateTransformer(ef.Template)
			if err != nil {
				return fmt.Errorf("creating template transformer for %s.%s: %w", entity.TypeName, ef.FieldName, err)
			}

			embedder, err := m.embedderRegistry.Get(ef.Model)
			if err != nil {
				return fmt.Errorf("embedder model %q for %s.%s: %w", ef.Model, entity.TypeName, ef.FieldName, err)
			}

			if m.pipelines[entity.TypeName] == nil {
				m.pipelines[entity.TypeName] = make(map[string]*searchindex.EmbeddingPipeline)
			}
			m.pipelines[entity.TypeName][ef.FieldName] = &searchindex.EmbeddingPipeline{
				Transformer: transformer,
				Embedder:    embedder,
			}
		}
	}
	return nil
}

func (m *Manager) runPopulations(ctx context.Context) error {
	for _, pop := range m.config.Populations {
		idx, ok := m.indices[pop.IndexName]
		if !ok {
			return fmt.Errorf("index %q not found for population", pop.IndexName)
		}

		entity := m.findEntity(pop.EntityTypeName)
		if entity == nil {
			return fmt.Errorf("entity %q not found for population", pop.EntityTypeName)
		}

		if err := m.populate(ctx, idx, entity, &pop); err != nil {
			return fmt.Errorf("populating index %q: %w", pop.IndexName, err)
		}

		// Schedule resync if configured
		if pop.ResyncInterval != "" {
			interval, err := time.ParseDuration(pop.ResyncInterval)
			if err != nil {
				return fmt.Errorf("invalid resync interval %q: %w", pop.ResyncInterval, err)
			}
			m.scheduleResync(ctx, idx, entity, &pop, interval)
		}
	}
	return nil
}

func (m *Manager) populate(ctx context.Context, idx searchindex.Index, entity *SearchableEntity, pop *PopulateDirective) error {
	responseBody, err := m.executor.Execute(ctx, pop.Query)
	if err != nil {
		return fmt.Errorf("executing population query: %w", err)
	}

	docs, err := ExtractEntities(responseBody, pop.Path, entity.TypeName, entity.KeyFields)
	if err != nil {
		return fmt.Errorf("extracting entities: %w", err)
	}

	if err := m.processEmbeddings(ctx, docs, entity); err != nil {
		return fmt.Errorf("processing embeddings: %w", err)
	}

	return idx.IndexDocuments(ctx, docs)
}

// processEmbeddings runs all embedding pipelines for the entity, populating vectors on each document.
func (m *Manager) processEmbeddings(ctx context.Context, docs []searchindex.EntityDocument, entity *SearchableEntity) error {
	entityPipelines, ok := m.pipelines[entity.TypeName]
	if !ok {
		return nil
	}

	fieldMaps := EntityFieldMaps(docs)
	for fieldName, pipeline := range entityPipelines {
		vectors, err := pipeline.ProcessBatch(ctx, fieldMaps)
		if err != nil {
			return fmt.Errorf("embedding field %s: %w", fieldName, err)
		}
		for i, vec := range vectors {
			if docs[i].Vectors == nil {
				docs[i].Vectors = make(map[string][]float32)
			}
			docs[i].Vectors[fieldName] = vec
		}
	}
	return nil
}

func (m *Manager) scheduleResync(ctx context.Context, idx searchindex.Index, entity *SearchableEntity, pop *PopulateDirective, interval time.Duration) {
	resyncCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancelFuncs = append(m.cancelFuncs, cancel)
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-resyncCtx.Done():
				return
			case <-ticker.C:
				if err := m.populate(resyncCtx, idx, entity, pop); err != nil {
					log.Printf("search_datasource: resync error for %s: %v", pop.IndexName, err)
				}
			}
		}
	}()
}

func (m *Manager) startSubscriptions(ctx context.Context) {
	if m.subscriber == nil {
		return
	}

	for _, sub := range m.config.Subscriptions {
		idx, ok := m.indices[sub.IndexName]
		if !ok {
			log.Printf("search_datasource: index %q not found for subscription, skipping", sub.IndexName)
			continue
		}

		entity := m.findEntity(sub.EntityTypeName)
		if entity == nil {
			log.Printf("search_datasource: entity %q not found for subscription, skipping", sub.EntityTypeName)
			continue
		}

		subCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.cancelFuncs = append(m.cancelFuncs, cancel)
		m.mu.Unlock()

		go m.runSubscription(subCtx, idx, entity, &sub)
	}
}

func (m *Manager) runSubscription(ctx context.Context, idx searchindex.Index, entity *SearchableEntity, sub *SubscribeDirective) {
	events, err := m.subscriber.Subscribe(ctx, sub.Subscription)
	if err != nil {
		log.Printf("search_datasource: subscribe to %q failed: %v", sub.IndexName, err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case eventData, ok := <-events:
			if !ok {
				return
			}
			if err := m.handleSubscriptionEvent(ctx, idx, entity, sub, eventData); err != nil {
				log.Printf("search_datasource: subscription event error for %s: %v", sub.IndexName, err)
			}
		}
	}
}

func (m *Manager) handleSubscriptionEvent(ctx context.Context, idx searchindex.Index, entity *SearchableEntity, sub *SubscribeDirective, eventData []byte) error {
	// Try deletion path first if configured.
	if sub.DeletionPath != "" {
		if err := m.handleDeletion(ctx, idx, entity, sub.DeletionPath, eventData); err == nil {
			return nil
		}
	}

	// Handle upsert via the regular path.
	docs, err := ExtractEntities(eventData, sub.Path, entity.TypeName, entity.KeyFields)
	if err != nil {
		return fmt.Errorf("extracting entities from event: %w", err)
	}

	if err := m.processEmbeddings(ctx, docs, entity); err != nil {
		return fmt.Errorf("processing embeddings: %w", err)
	}

	return idx.IndexDocuments(ctx, docs)
}

func (m *Manager) handleDeletion(ctx context.Context, idx searchindex.Index, entity *SearchableEntity, deletionPath string, eventData []byte) error {
	var raw any
	if err := json.Unmarshal(eventData, &raw); err != nil {
		return err
	}

	current := raw
	for _, segment := range strings.Split(deletionPath, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object at path segment %q", segment)
		}
		val, ok := obj[segment]
		if !ok {
			return fmt.Errorf("path segment %q not found", segment)
		}
		current = val
	}

	switch v := current.(type) {
	case map[string]any:
		id := buildIdentity(v, entity)
		return idx.DeleteDocument(ctx, id)
	case []any:
		ids := make([]searchindex.DocumentIdentity, 0, len(v))
		for _, item := range v {
			if obj, ok := item.(map[string]any); ok {
				ids = append(ids, buildIdentity(obj, entity))
			}
		}
		return idx.DeleteDocuments(ctx, ids)
	default:
		return fmt.Errorf("unexpected type %T at deletion path", current)
	}
}

func buildIdentity(obj map[string]any, entity *SearchableEntity) searchindex.DocumentIdentity {
	keyFields := make(map[string]any, len(entity.KeyFields))
	for _, kf := range entity.KeyFields {
		keyFields[kf] = obj[kf]
	}
	return searchindex.DocumentIdentity{
		TypeName:  entity.TypeName,
		KeyFields: keyFields,
	}
}

func (m *Manager) findEntity(typeName string) *SearchableEntity {
	for i := range m.config.Entities {
		if m.config.Entities[i].TypeName == typeName {
			return &m.config.Entities[i]
		}
	}
	return nil
}
