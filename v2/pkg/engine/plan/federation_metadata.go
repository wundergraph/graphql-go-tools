package plan

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type FederationMetaData struct {
	Keys             FederationFieldConfigurations
	Requires         FederationFieldConfigurations
	Provides         FederationFieldConfigurations
	EntityInterfaces []EntityInterfaceConfiguration
	InterfaceObjects []EntityInterfaceConfiguration
	EntityCaching    EntityCacheConfigurations
	RootFieldCaching RootFieldCacheConfigurations

	entityTypeNames map[string]struct{}
}

type FederationInfo interface {
	HasKeyRequirement(typeName, requiresFields string) bool
	RequiredFieldsByKey(typeName string) []FederationFieldConfiguration
	RequiredFieldsByRequires(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool)
	HasEntity(typeName string) bool
	HasInterfaceObject(typeName string) bool
	HasEntityInterface(typeName string) bool
	EntityInterfaceNames() []string
	EntityCacheConfig(typeName string) *EntityCacheConfiguration
	RootFieldCacheConfig(typeName, fieldName string) *RootFieldCacheConfiguration
}

func (d *FederationMetaData) HasKeyRequirement(typeName, requiresFields string) bool {
	return d.Keys.HasSelectionSet(typeName, "", requiresFields)
}

func (d *FederationMetaData) RequiredFieldsByKey(typeName string) []FederationFieldConfiguration {
	return d.Keys.FilterByTypeAndResolvability(typeName, true)
}

func (d *FederationMetaData) HasEntity(typeName string) bool {
	_, ok := d.entityTypeNames[typeName]
	return ok
}

func (d *FederationMetaData) RequiredFieldsByRequires(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool) {
	return d.Requires.FirstByTypeAndField(typeName, fieldName)
}

func (d *FederationMetaData) HasInterfaceObject(typeName string) bool {
	return slices.ContainsFunc(d.InterfaceObjects, func(interfaceObjCfg EntityInterfaceConfiguration) bool {
		return slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) || interfaceObjCfg.InterfaceTypeName == typeName
	})
}

func (d *FederationMetaData) HasEntityInterface(typeName string) bool {
	return slices.ContainsFunc(d.EntityInterfaces, func(interfaceObjCfg EntityInterfaceConfiguration) bool {
		return slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) || interfaceObjCfg.InterfaceTypeName == typeName
	})
}

func (d *FederationMetaData) EntityInterfaceNames() (out []string) {
	if len(d.EntityInterfaces) == 0 {
		return nil
	}

	for i := range d.EntityInterfaces {
		out = append(out, d.EntityInterfaces[i].InterfaceTypeName)
	}

	return out
}

type EntityInterfaceConfiguration struct {
	InterfaceTypeName string
	ConcreteTypeNames []string
}

// EntityCacheConfiguration defines L2 caching behavior for a specific entity type.
// This configuration is subgraph-local: each subgraph configures caching for entities it provides.
// Caching is opt-in: entities without configuration will not be cached in L2.
type EntityCacheConfiguration struct {
	// TypeName is the GraphQL type name of the entity to cache (e.g., "User", "Product").
	// This must match the __typename returned by the subgraph for _entities queries.
	TypeName string `json:"type_name"`

	// CacheName identifies which LoaderCache instance to use for storing this entity.
	// Multiple entity types can share a cache by using the same CacheName.
	// The cache name must be registered in the Loader's caches map at runtime.
	CacheName string `json:"cache_name"`

	// TTL (Time To Live) specifies how long cached entities remain valid.
	// After TTL expires, the next request will fetch fresh data from the subgraph.
	// A zero TTL means entries never expire (not recommended for production).
	TTL time.Duration `json:"ttl"`

	// IncludeSubgraphHeaderPrefix controls whether forwarded headers affect cache keys.
	// When true, cache keys include a hash of the headers sent to the subgraph,
	// ensuring different header configurations (e.g., different auth tokens) use
	// separate cache entries. Set to true when subgraph responses vary by headers.
	IncludeSubgraphHeaderPrefix bool `json:"include_subgraph_header_prefix"`

	// EnablePartialCacheLoad enables fetching only cache-missed entities from the subgraph.
	// Default behavior (false): If ANY entity in a batch is missing from cache, ALL entities
	// are fetched from the subgraph. This keeps the cache fresh but may overfetch.
	// When enabled (true): Only missing entities are fetched; cached entities are served
	// directly from cache. This reduces subgraph load but cached entities may become stale
	// within their TTL window. Use when cache freshness is acceptable within TTL bounds.
	EnablePartialCacheLoad bool `json:"enable_partial_cache_load"`
}

// EntityCacheConfigurations is a collection of entity cache configurations.
type EntityCacheConfigurations []EntityCacheConfiguration

// FindByTypeName returns the cache configuration for the given entity type.
// Returns nil if no configuration exists (caching disabled for this entity).
func (c EntityCacheConfigurations) FindByTypeName(typeName string) *EntityCacheConfiguration {
	for i := range c {
		if c[i].TypeName == typeName {
			return &c[i]
		}
	}
	return nil
}

// RootFieldCacheConfiguration defines L2 caching behavior for a specific root field.
// This configuration is subgraph-local: each subgraph configures caching for root fields it provides.
type RootFieldCacheConfiguration struct {
	// TypeName is the type containing the field (e.g., "Query", "Mutation")
	TypeName string `json:"type_name"`
	// FieldName is the name of the root field to cache (e.g., "topProducts", "me")
	FieldName string `json:"field_name"`
	// CacheName is the name of the cache to use (maps to LoaderCache instances)
	CacheName string `json:"cache_name"`
	// TTL is the time-to-live for cached responses
	TTL time.Duration `json:"ttl"`
	// IncludeSubgraphHeaderPrefix indicates if forwarded headers affect cache key.
	// When true, different header values result in different cache keys.
	IncludeSubgraphHeaderPrefix bool `json:"include_subgraph_header_prefix"`
}

// RootFieldCacheConfigurations is a collection of root field cache configurations.
type RootFieldCacheConfigurations []RootFieldCacheConfiguration

// FindByTypeAndField returns the cache configuration for the given type and field.
// Returns nil if no configuration exists (caching disabled for this root field).
func (c RootFieldCacheConfigurations) FindByTypeAndField(typeName, fieldName string) *RootFieldCacheConfiguration {
	for i := range c {
		if c[i].TypeName == typeName && c[i].FieldName == fieldName {
			return &c[i]
		}
	}
	return nil
}

// EntityCacheConfig returns the cache configuration for the given entity type.
// Returns nil if no configuration exists (caching should be disabled for this entity).
func (d *FederationMetaData) EntityCacheConfig(typeName string) *EntityCacheConfiguration {
	return d.EntityCaching.FindByTypeName(typeName)
}

// RootFieldCacheConfig returns the cache configuration for the given root field.
// Returns nil if no configuration exists (caching should be disabled for this root field).
func (d *FederationMetaData) RootFieldCacheConfig(typeName, fieldName string) *RootFieldCacheConfiguration {
	return d.RootFieldCaching.FindByTypeAndField(typeName, fieldName)
}

type FederationFieldConfiguration struct {
	TypeName              string         `json:"type_name"`            // TypeName is the name of the Entity the Fragment is for
	FieldName             string         `json:"field_name,omitempty"` // FieldName is empty for key requirements, otherwise, it is the name of the field that has requires or provides directive
	SelectionSet          string         `json:"selection_set"`        // SelectionSet is the selection set that is required for the given field (keys, requires, provides)
	DisableEntityResolver bool           `json:"-"`                    // applicable only for the keys. If true it means that the given entity could not be resolved by this key.
	Conditions            []KeyCondition `json:"conditions,omitempty"` // conditions stores coordinates under which we could use implicit key, while on other paths this key is not available

	parsedSelectionSet *ast.Document
	RemappedPaths      map[string]string
}

type KeyCondition struct {
	Coordinates []FieldCoordinate `json:"coordinates"`
	FieldPath   []string          `json:"field_path"`
}

type FieldCoordinate struct {
	TypeName  string `json:"type_name"`
	FieldName string `json:"field_name"`
}

// parseSelectionSet parses the selection set and stores the parsed AST in parsedSelectionSet.
// should have pointer receiver to preserve the value
func (f *FederationFieldConfiguration) parseSelectionSet() error {
	if f.parsedSelectionSet != nil {
		return nil
	}

	doc, report := RequiredFieldsFragment(f.TypeName, f.SelectionSet, false)
	if report.HasErrors() {
		return report
	}

	f.parsedSelectionSet = doc
	return nil
}

// String - implements fmt.Stringer
// NOTE: do not change to pointer receiver, it won't work for not pointer values
func (f FederationFieldConfiguration) String() string {
	b, _ := json.Marshal(f)
	return string(b)
}

type FederationFieldConfigurations []FederationFieldConfiguration

func (f *FederationFieldConfigurations) FilterByTypeAndResolvability(typeName string, skipUnresovable bool) (out []FederationFieldConfiguration) {
	for i := range *f {
		if (*f)[i].TypeName != typeName || (*f)[i].FieldName != "" {
			continue
		}
		if skipUnresovable && (*f)[i].DisableEntityResolver {
			continue
		}
		out = append(out, (*f)[i])
	}
	return out
}

func (f *FederationFieldConfigurations) UniqueTypes() (out []string) {
	seen := map[string]struct{}{}
	for i := range *f {
		seen[(*f)[i].TypeName] = struct{}{}
	}

	for k := range seen {
		out = append(out, k)
	}
	return out
}

func (f *FederationFieldConfigurations) FirstByTypeAndField(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool) {
	for i := range *f {
		if (*f)[i].TypeName == typeName && (*f)[i].FieldName == fieldName {
			return (*f)[i], true
		}
	}
	return FederationFieldConfiguration{}, false
}

func (f *FederationFieldConfigurations) HasSelectionSet(typeName, fieldName, selectionSet string) bool {
	for i := range *f {
		if typeName == (*f)[i].TypeName &&
			fieldName == (*f)[i].FieldName &&
			selectionSet == (*f)[i].SelectionSet {
			return true
		}
	}
	return false
}

func (f *FederationFieldConfigurations) AppendIfNotPresent(config FederationFieldConfiguration) (added bool) {
	ok := f.HasSelectionSet(config.TypeName, config.FieldName, config.SelectionSet)
	if ok {
		return false
	}

	*f = append(*f, config)

	return true
}
