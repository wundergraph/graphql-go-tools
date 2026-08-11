package cacheconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestEntities pins the entity static tier: which lifetime an entity fetch
// resolves once per-type declarations join the cascade, when a declaration
// vetoes caching outright, and how privacy folds together.
func TestEntities(t *testing.T) {
	rows := []struct {
		name      string
		effective EffectiveSubgraphConfig
		typeNames []string
		want      EntityCacheConfig
		wantOK    bool
	}{
		{
			name:      "an undeclared type falls through to the subgraph DefaultTTL",
			effective: EffectiveSubgraphConfig{Enabled: true, DefaultTTL: time.Minute},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{TTL: time.Minute},
			wantOK:    true,
		},
		{
			name:      "no lifetime anywhere caches nothing",
			effective: EffectiveSubgraphConfig{Enabled: true},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{},
			wantOK:    false,
		},
		{
			name:      "the NegativeCacheTTL alone still enables an undeclared type",
			effective: EffectiveSubgraphConfig{Enabled: true, NegativeCacheTTL: 5 * time.Second},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{TTL: 0},
			wantOK:    true,
		},
		{
			name:      "shadow mode alone does not enable entity caching",
			effective: EffectiveSubgraphConfig{Enabled: true, ShadowMode: true},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{},
			wantOK:    false,
		},
		{
			name: "the subgraph veto beats a positive declaration",
			effective: EffectiveSubgraphConfig{
				Enabled:    false,
				DefaultTTL: time.Minute,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second},
				},
			},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{},
			wantOK:    false,
		},
		{
			name: "a declared type beats the subgraph DefaultTTL",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second},
				},
			},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{TTL: 30 * time.Second},
			wantOK:    true,
		},
		{
			name: "a declaration LONGER than the subgraph default wins too",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: time.Hour},
				},
			},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{TTL: time.Hour},
			wantOK:    true,
		},
		{
			name: "another type's declaration leaves this one on the subgraph default",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Types: map[string]TypeCacheConfig{
					"User": {MaxAge: 30 * time.Second},
				},
			},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{TTL: time.Minute},
			wantOK:    true,
		},
		{
			name: "a type declared uncacheable gets no configuration at all",
			effective: EffectiveSubgraphConfig{
				Enabled:          true,
				DefaultTTL:       time.Minute,
				NegativeCacheTTL: 5 * time.Second,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 0},
				},
			},
			typeNames: []string{"Product"},
			want:      EntityCacheConfig{},
			wantOK:    false,
		},
		{
			name: "a private subgraph makes an undeclared type private",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Scope:      CacheScopePrivate,
			},
			typeNames: []string{"Product"},
			want: EntityCacheConfig{
				TTL:   time.Minute,
				Scope: CacheScopePrivate,
			},
			wantOK: true,
		},
		{
			name: "a PUBLIC declaration cannot make a private subgraph public",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Scope:      CacheScopePrivate,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second, Scope: CacheScopePublic},
				},
			},
			typeNames: []string{"Product"},
			want: EntityCacheConfig{
				TTL:   30 * time.Second,
				Scope: CacheScopePrivate,
			},
			wantOK: true,
		},
		{
			name: "a PRIVATE declaration makes one type of a public subgraph private",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second, Scope: CacheScopePrivate},
				},
			},
			typeNames: []string{"Product"},
			want: EntityCacheConfig{
				TTL:   30 * time.Second,
				Scope: CacheScopePrivate,
			},
			wantOK: true,
		},
		{
			name: "a sibling type of the same subgraph stays public",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second, Scope: CacheScopePrivate},
				},
			},
			typeNames: []string{"User"},
			want:      EntityCacheConfig{TTL: time.Minute},
			wantOK:    true,
		},
		{
			name: "a batch over several declared types takes the shortest lifetime",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second},
					"Brand":   {MaxAge: 10 * time.Second},
				},
			},
			typeNames: []string{
				"Product",
				"Brand",
			},
			want:   EntityCacheConfig{TTL: 10 * time.Second},
			wantOK: true,
		},
		{
			name: "an undeclared concrete type contributes the subgraph default",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: 5 * time.Second,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second},
				},
			},
			typeNames: []string{
				"Product",
				"Brand",
			},
			want:   EntityCacheConfig{TTL: 5 * time.Second},
			wantOK: true,
		},
		{
			name: "one private declaration turns the whole batch private",
			effective: EffectiveSubgraphConfig{
				Enabled:    true,
				DefaultTTL: time.Minute,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second},
					"Brand":   {MaxAge: 45 * time.Second, Scope: CacheScopePrivate},
				},
			},
			typeNames: []string{
				"Product",
				"Brand",
			},
			want: EntityCacheConfig{
				TTL:   30 * time.Second,
				Scope: CacheScopePrivate,
			},
			wantOK: true,
		},
		{
			name: "one uncacheable declaration vetoes the whole batch",
			effective: EffectiveSubgraphConfig{
				Enabled:          true,
				DefaultTTL:       time.Minute,
				NegativeCacheTTL: 5 * time.Second,
				Types: map[string]TypeCacheConfig{
					"Product": {MaxAge: 30 * time.Second},
					"Brand":   {MaxAge: 0},
				},
			},
			typeNames: []string{
				"Product",
				"Brand",
			},
			want:   EntityCacheConfig{},
			wantOK: false,
		},
		{
			name:      "a fetch without any type name resolves nothing",
			effective: EffectiveSubgraphConfig{Enabled: true, DefaultTTL: time.Minute},
			typeNames: nil,
			want:      EntityCacheConfig{},
			wantOK:    false,
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			entity, ok := row.effective.Entities(row.typeNames)
			assert.Equal(t, row.wantOK, ok)
			assert.Equal(t, row.want, entity)
		})
	}

	t.Run("the whole cascade from the raw configuration: a declaration overrides the global default", func(t *testing.T) {
		caching := &CachingConfiguration{
			Global: GlobalCacheConfig{DefaultTTL: 2 * time.Minute},
			Subgraphs: map[string]SubgraphCacheConfig{
				"products": {
					Types: map[string]TypeCacheConfig{
						"Product": {MaxAge: 15 * time.Second, Scope: CacheScopePrivate},
					},
				},
			},
		}
		entity, ok := caching.Resolve("products").Entities([]string{"Product"})
		assert.True(t, ok)
		assert.Equal(t, EntityCacheConfig{
			TTL:   15 * time.Second,
			Scope: CacheScopePrivate,
		}, entity)

		// Its sibling keeps the global default and stays public.
		entity, ok = caching.Resolve("products").Entities([]string{"User"})
		assert.True(t, ok)
		assert.Equal(t, EntityCacheConfig{TTL: 2 * time.Minute}, entity)
	})
}
