package cacheconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestExtractTypeCacheConfigs pins the SDL bridge: which declarations become
// the per-type tier, which are skipped, and what the operator is told about the
// skipped ones.
func TestExtractTypeCacheConfigs(t *testing.T) {
	rows := []struct {
		name         string
		sdl          string
		wantTypes    map[string]TypeCacheConfig
		wantWarnings []string
	}{
		{
			name: "a well-formed declaration becomes the type's entry",
			sdl: `type Product @key(fields: "upc") @cache(maxAge: 60) {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"Product": {MaxAge: time.Minute},
			},
			wantWarnings: nil,
		},
		{
			name: "an omitted scope declares the type public",
			sdl: `type Product @cache(maxAge: 30) {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"Product": {MaxAge: 30 * time.Second, Scope: CacheScopePublic},
			},
			wantWarnings: nil,
		},
		{
			name: "an explicit PUBLIC scope reads the same as omitting it",
			sdl: `type Product @cache(maxAge: 30, scope: PUBLIC) {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"Product": {MaxAge: 30 * time.Second, Scope: CacheScopePublic},
			},
			wantWarnings: nil,
		},
		{
			name: "a PRIVATE scope is carried through",
			sdl: `type User @cache(maxAge: 10, scope: PRIVATE) {
				id: ID!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"User": {MaxAge: 10 * time.Second, Scope: CacheScopePrivate},
			},
			wantWarnings: nil,
		},
		{
			name: "maxAge 0 declares the type uncacheable",
			sdl: `type Product @cache(maxAge: 0) {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"Product": {MaxAge: 0},
			},
			wantWarnings: nil,
		},
		{
			name: "a declaration on a type EXTENSION is read too",
			sdl: `extend type Product @key(fields: "upc") @cache(maxAge: 120) {
				upc: String! @external
				stock: Int!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"Product": {MaxAge: 2 * time.Minute},
			},
			wantWarnings: nil,
		},
		{
			name: "every declared type of one SDL is extracted, entity or not",
			sdl: `type Product @key(fields: "upc") @cache(maxAge: 60) {
				upc: String!
				warehouse: Warehouse!
			}
			type Warehouse @cache(maxAge: 3600, scope: PRIVATE) {
				id: ID!
			}
			type Review {
				id: ID!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"Product":   {MaxAge: time.Minute},
				"Warehouse": {MaxAge: time.Hour, Scope: CacheScopePrivate},
			},
			wantWarnings: nil,
		},
		{
			name: "an SDL without the directive declares nothing",
			sdl: `type Product @key(fields: "upc") {
				upc: String!
			}`,
			wantTypes:    map[string]TypeCacheConfig{},
			wantWarnings: nil,
		},
		{
			name: "a maxAge that is not an Int skips the declaration and warns",
			sdl: `type Product @cache(maxAge: "60") {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{},
			wantWarnings: []string{
				`type "Product": @cache was skipped, its maxAge is not a non-negative Int`,
			},
		},
		{
			name: "a missing maxAge skips the declaration and warns",
			sdl: `type Product @cache(scope: PRIVATE) {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{},
			wantWarnings: []string{
				`type "Product": @cache was skipped, its maxAge is not a non-negative Int`,
			},
		},
		{
			name: "a negative maxAge skips the declaration and warns",
			sdl: `type Product @cache(maxAge: -1) {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{},
			wantWarnings: []string{
				`type "Product": @cache was skipped, its maxAge is not a non-negative Int`,
			},
		},
		{
			name: "an unknown scope skips the declaration instead of guessing PUBLIC",
			sdl: `type Product @cache(maxAge: 60, scope: TENANT) {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{},
			wantWarnings: []string{
				`type "Product": @cache was skipped, its scope "TENANT" is not a CacheScope value`,
			},
		},
		{
			name: "a scope that is not an enum value skips the declaration",
			sdl: `type Product @cache(maxAge: 60, scope: 1) {
				upc: String!
			}`,
			wantTypes: map[string]TypeCacheConfig{},
			wantWarnings: []string{
				`type "Product": @cache was skipped, its scope is not a CacheScope value`,
			},
		},
		{
			name: "one skipped declaration leaves the other types extracted",
			sdl: `type Product @cache(maxAge: "60") {
				upc: String!
			}
			type Warehouse @cache(maxAge: 90) {
				id: ID!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"Warehouse": {MaxAge: 90 * time.Second},
			},
			wantWarnings: []string{
				`type "Product": @cache was skipped, its maxAge is not a non-negative Int`,
			},
		},
		{
			name: "a type declared twice keeps its definition's declaration",
			sdl: `type Product @cache(maxAge: 60) {
				upc: String!
			}
			extend type Product @cache(maxAge: 3600) {
				stock: Int!
			}`,
			wantTypes: map[string]TypeCacheConfig{
				"Product": {MaxAge: time.Minute},
			},
			wantWarnings: []string{
				`type "Product": a repeated @cache declaration was ignored`,
			},
		},
		{
			name:      "an SDL that does not parse yields no declaration and one warning",
			sdl:       `type Product @cache(maxAge: 60) {`,
			wantTypes: map[string]TypeCacheConfig{},
			wantWarnings: []string{
				"no @cache declaration was read, the SDL does not parse: external: unexpected token - got: EOF want one of: [], locations: [{Line:0 Column:0}], path: []",
			},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			types, warnings := ExtractTypeCacheConfigs(row.sdl)
			assert.Equal(t, row.wantTypes, types)
			assert.Equal(t, row.wantWarnings, warnings)
		})
	}

	t.Run("the extracted map feeds a subgraph's per-type tier directly", func(t *testing.T) {
		types, warnings := ExtractTypeCacheConfigs(`type Product @key(fields: "upc") @cache(maxAge: 60) {
			upc: String!
		}
		type User @key(fields: "id") @cache(maxAge: 10, scope: PRIVATE) {
			id: ID!
		}`)
		assert.Equal(t, []string(nil), warnings)

		caching := &CachingConfiguration{
			Global:    GlobalCacheConfig{DefaultTTL: time.Hour},
			Subgraphs: map[string]SubgraphCacheConfig{"products": {Types: types}},
		}
		effective := caching.Resolve("products")

		product, ok := effective.Entities([]string{"Product"})
		assert.True(t, ok)
		assert.Equal(t, EntityCacheConfig{TTL: time.Minute}, product)

		user, ok := effective.Entities([]string{"User"})
		assert.True(t, ok)
		assert.Equal(t, EntityCacheConfig{
			TTL:   10 * time.Second,
			Scope: CacheScopePrivate,
		}, user)
	})
}

// TestCacheDirectiveDefinition pins the SDL composition tooling references, so
// the spelling this reader accepts and the spelling schemas are validated
// against cannot drift apart.
func TestCacheDirectiveDefinition(t *testing.T) {
	assert.Equal(t, `enum CacheScope {
  PUBLIC
  PRIVATE
}

directive @cache(maxAge: Int!, scope: CacheScope = PUBLIC) on OBJECT`, CacheDirectiveDefinition)
}
