package cacheconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func ptr[T any](value T) *T {
	return &value
}

// TestResolve pins the cascade resolution per field: with every global knob
// set, a subgraph inherits all of them unless it overrides that one knob.
func TestResolve(t *testing.T) {
	global := GlobalCacheConfig{
		DefaultTTL:             time.Minute,
		MaxTTL:                 time.Hour,
		NegativeCacheTTL:       5 * time.Second,
		EnablePartialCacheLoad: true,
		HashAnalyticsKeys:      true,
		ShadowMode:             true,
	}

	t.Run("a subgraph without an entry inherits the global level", func(t *testing.T) {
		caching := &CachingConfiguration{
			Global:    global,
			Subgraphs: map[string]SubgraphCacheConfig{"products": {DefaultTTL: ptr(time.Second)}},
		}
		assert.Equal(t, EffectiveSubgraphConfig{
			Enabled:                true,
			DefaultTTL:             time.Minute,
			MaxTTL:                 time.Hour,
			NegativeCacheTTL:       5 * time.Second,
			EnablePartialCacheLoad: true,
			HashAnalyticsKeys:      true,
			ShadowMode:             true,
			IncludeSubgraphHeaders: false,
		}, caching.Resolve("reviews"))
	})

	rows := []struct {
		name     string
		subgraph SubgraphCacheConfig
		want     EffectiveSubgraphConfig
	}{
		{
			name:     "no override inherits every global value",
			subgraph: SubgraphCacheConfig{},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "Enabled false vetoes the subgraph",
			subgraph: SubgraphCacheConfig{Enabled: ptr(false)},
			want: EffectiveSubgraphConfig{
				Enabled:                false,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "Enabled true keeps the subgraph enabled",
			subgraph: SubgraphCacheConfig{Enabled: ptr(true)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "DefaultTTL override wins",
			subgraph: SubgraphCacheConfig{DefaultTTL: ptr(10 * time.Second)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             10 * time.Second,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "DefaultTTL override to zero disables entity caching for the subgraph",
			subgraph: SubgraphCacheConfig{DefaultTTL: ptr(time.Duration(0)), NegativeCacheTTL: ptr(time.Duration(0))},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             0,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       0,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "MaxTTL override wins",
			subgraph: SubgraphCacheConfig{MaxTTL: ptr(2 * time.Minute)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 2 * time.Minute,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "NegativeCacheTTL override wins",
			subgraph: SubgraphCacheConfig{NegativeCacheTTL: ptr(time.Second)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "EnablePartialCacheLoad override wins",
			subgraph: SubgraphCacheConfig{EnablePartialCacheLoad: ptr(false)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: false,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "ShadowMode override wins",
			subgraph: SubgraphCacheConfig{ShadowMode: ptr(false)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             false,
				IncludeSubgraphHeaders: false,
			},
		},
		{
			name:     "IncludeSubgraphHeaders has no global level and is set per subgraph",
			subgraph: SubgraphCacheConfig{IncludeSubgraphHeaders: ptr(true)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: true,
			},
		},
		{
			name:     "Scope has no global level and is declared per subgraph",
			subgraph: SubgraphCacheConfig{Scope: ptr(CacheScopePrivate)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
				Scope:                  CacheScopePrivate,
			},
		},
		{
			// PUBLIC is the absence of privacy, so declaring it explicitly must
			// resolve exactly like declaring nothing.
			name:     "an explicit PUBLIC scope resolves like no declaration",
			subgraph: SubgraphCacheConfig{Scope: ptr(CacheScopePublic)},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
				Scope:                  CacheScopePublic,
			},
		},
		{
			name: "root field entries are carried through unchanged",
			subgraph: SubgraphCacheConfig{
				RootFields: []RootFieldCacheConfig{
					{TypeName: "Query", FieldName: "topProducts", TTL: 30 * time.Second},
				},
			},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
				RootFields: []RootFieldCacheConfig{
					{TypeName: "Query", FieldName: "topProducts", TTL: 30 * time.Second},
				},
			},
		},
		{
			name: "type declarations are carried through unchanged",
			subgraph: SubgraphCacheConfig{
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second},
					"User":    {MaxAge: 10 * time.Second, Scope: CacheScopePrivate},
				},
			},
			want: EffectiveSubgraphConfig{
				Enabled:                true,
				DefaultTTL:             time.Minute,
				MaxTTL:                 time.Hour,
				NegativeCacheTTL:       5 * time.Second,
				EnablePartialCacheLoad: true,
				HashAnalyticsKeys:      true,
				ShadowMode:             true,
				IncludeSubgraphHeaders: false,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second},
					"User":    {MaxAge: 10 * time.Second, Scope: CacheScopePrivate},
				},
			},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			caching := &CachingConfiguration{
				Global:    global,
				Subgraphs: map[string]SubgraphCacheConfig{"products": row.subgraph},
			}
			assert.Equal(t, row.want, caching.Resolve("products"))
		})
	}

	t.Run("an empty global level resolves to no caching at all", func(t *testing.T) {
		caching := &CachingConfiguration{}
		assert.Equal(t, EffectiveSubgraphConfig{
			Enabled: true,
		}, caching.Resolve("products"))
	})

	t.Run("EmitClientCacheControl defaults to true and is switched off globally", func(t *testing.T) {
		assert.Equal(t, true, GlobalCacheConfig{}.EmitsClientCacheControl())
		assert.Equal(t, false, GlobalCacheConfig{EmitClientCacheControl: ptr(false)}.EmitsClientCacheControl())
		assert.Equal(t, true, GlobalCacheConfig{EmitClientCacheControl: ptr(true)}.EmitsClientCacheControl())
	})

	t.Run("a nil configuration resolves to a disabled subgraph", func(t *testing.T) {
		var caching *CachingConfiguration
		assert.Equal(t, EffectiveSubgraphConfig{}, caching.Resolve("products"))
	})
}

// TestRootField pins the per-coordinate lookup: exact coordinate match, the
// subgraph-level fold, the inert-entry miss, and the veto.
func TestRootField(t *testing.T) {
	t.Run("the exact coordinate entry is returned", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled: true,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts", TTL: time.Minute, PartialBatchLoad: true},
				{TypeName: "Query", FieldName: "topReviews", TTL: time.Second},
			},
		}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:         "Query",
			FieldName:        "topProducts",
			TTL:              time.Minute,
			PartialBatchLoad: true,
		}, entry)
	})

	t.Run("both coordinates must match", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled: true,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts", TTL: time.Minute},
			},
		}
		entry, ok := effective.RootField("Query", "topReviews")
		assert.False(t, ok)
		assert.Equal(t, RootFieldCacheConfig{}, entry)

		entry, ok = effective.RootField("Mutation", "topProducts")
		assert.False(t, ok)
		assert.Equal(t, RootFieldCacheConfig{}, entry)
	})

	t.Run("subgraph shadow mode and header inclusion fold into the entry", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled:                true,
			ShadowMode:             true,
			IncludeSubgraphHeaders: true,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts", TTL: time.Minute},
			},
		}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:               "Query",
			FieldName:              "topProducts",
			TTL:                    time.Minute,
			ShadowMode:             true,
			IncludeSubgraphHeaders: true,
		}, entry)
	})

	t.Run("a private subgraph makes every one of its root fields private", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled: true,
			Scope:   CacheScopePrivate,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts", TTL: time.Minute},
			},
		}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "topProducts",
			TTL:       time.Minute,
			Scope:     CacheScopePrivate,
		}, entry)
	})

	t.Run("a private coordinate stays private inside a public subgraph", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled: true,
			Scope:   CacheScopePublic,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "me", TTL: time.Minute, Scope: CacheScopePrivate},
				{TypeName: "Query", FieldName: "topProducts", TTL: time.Minute},
			},
		}
		private, ok := effective.RootField("Query", "me")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "me",
			TTL:       time.Minute,
			Scope:     CacheScopePrivate,
		}, private)

		// Its sibling is untouched: privacy never widens beyond what declared it.
		public, ok := effective.RootField("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "topProducts",
			TTL:       time.Minute,
			Scope:     CacheScopePublic,
		}, public)
	})

	t.Run("subgraph shadow mode makes a TTL-less entry effective", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled:    true,
			ShadowMode: true,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts"},
			},
		}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:   "Query",
			FieldName:  "topProducts",
			ShadowMode: true,
		}, entry)
	})

	t.Run("an entry enabling nothing is a miss", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled: true,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts", PartialBatchLoad: true},
			},
		}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.False(t, ok)
		assert.Equal(t, RootFieldCacheConfig{}, entry)
	})

	t.Run("the subgraph veto hides every entry", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled: false,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts", TTL: time.Minute},
			},
		}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.False(t, ok)
		assert.Equal(t, RootFieldCacheConfig{}, entry)
	})

	t.Run("a subgraph without entries has no cached root fields", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{Enabled: true, DefaultTTL: time.Minute}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.False(t, ok)
		assert.Equal(t, RootFieldCacheConfig{}, entry)
	})

	t.Run("a TTL-less entry falls back to the subgraph DefaultTTL", func(t *testing.T) {
		// Resolve has already folded the global default into DefaultTTL, so this
		// one field carries the whole "subgraph, else global" half of the ladder.
		effective := EffectiveSubgraphConfig{
			Enabled:    true,
			DefaultTTL: 5 * time.Minute,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts", PartialBatchLoad: true},
			},
		}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:         "Query",
			FieldName:        "topProducts",
			TTL:              5 * time.Minute,
			PartialBatchLoad: true,
		}, entry)
	})

	t.Run("the coordinate TTL wins over the subgraph DefaultTTL", func(t *testing.T) {
		effective := EffectiveSubgraphConfig{
			Enabled:    true,
			DefaultTTL: 5 * time.Minute,
			RootFields: []RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "topProducts", TTL: 30 * time.Second},
			},
		}
		entry, ok := effective.RootField("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "topProducts",
			TTL:       30 * time.Second,
		}, entry)
	})

	t.Run("the whole ladder from the raw cascade: global default, no narrower TTL anywhere", func(t *testing.T) {
		caching := &CachingConfiguration{
			Global: GlobalCacheConfig{DefaultTTL: 2 * time.Minute},
			Subgraphs: map[string]SubgraphCacheConfig{
				"products": {
					RootFields: []RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "topProducts"},
					},
				},
			},
		}
		entry, ok := caching.Resolve("products").RootField("Query", "topProducts")
		assert.True(t, ok)
		assert.Equal(t, RootFieldCacheConfig{
			TypeName:  "Query",
			FieldName: "topProducts",
			TTL:       2 * time.Minute,
		}, entry)
	})

	t.Run("a bare DefaultTTL never caches a coordinate without an entry", func(t *testing.T) {
		caching := &CachingConfiguration{
			Global: GlobalCacheConfig{DefaultTTL: 2 * time.Minute},
			Subgraphs: map[string]SubgraphCacheConfig{
				"products": {
					RootFields: []RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "topProducts"},
					},
				},
			},
		}
		entry, ok := caching.Resolve("products").RootField("Query", "topReviews")
		assert.False(t, ok)
		assert.Equal(t, RootFieldCacheConfig{}, entry)
	})
}

func TestCacheScopeString(t *testing.T) {
	assert.Equal(t, "PUBLIC", CacheScopePublic.String())
	assert.Equal(t, "PRIVATE", CacheScopePrivate.String())
	assert.Equal(t, "CacheScope(7)", CacheScope(7).String())
}
