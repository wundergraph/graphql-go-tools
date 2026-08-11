package engine

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func newCachingTestConfiguration(t *testing.T) Configuration {
	t.Helper()

	schema := heroWithArgumentSchema(t)
	engineConf := NewConfiguration(schema)
	engineConf.SetDataSources([]plan.DataSource{
		mustGraphqlDataSourceConfiguration(t,
			"heroes-ds",
			mustFactory(t, http.DefaultClient),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"hero", "heroDefault", "heroDefaultRequired", "heroes"}},
				},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "http://localhost:8080/graphql",
					Method: "POST",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, `
					type Query {
						hero(name: String): String
						heroDefault(name: String = "Any"): String
						heroDefaultRequired(name: String! = "AnyRequired"): String
						heroes(names: [String!]!): [String!]
					}`),
			}),
		),
	})
	return engineConf
}

// TestSetCachingWiring pins the engine entry point: SetCaching (its subgraph
// overrides keyed by datasource ID) reaches the planner's caching
// configuration, force-enables FetchInfo, and produces the postprocess
// EnableCaching option; without SetCaching none of that is constructed.
func TestSetCachingWiring(t *testing.T) {
	t.Run("without SetCaching nothing is wired", func(t *testing.T) {
		engineConf := newCachingTestConfiguration(t)
		engine, err := NewExecutionEngine(context.Background(), abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{MaxConcurrency: 1})
		require.NoError(t, err)
		assert.Nil(t, engine.config.plannerConfig.Caching)
		assert.Nil(t, engine.postProcessorOptions)
	})

	t.Run("SetCaching wires the planner configuration, FetchInfo, and the postprocess option", func(t *testing.T) {
		engineConf := newCachingTestConfiguration(t)
		engineConf.plannerConfig.DisableIncludeInfo = true // must be force-overridden by caching
		ttl := time.Minute
		engineConf.SetCaching(cacheconfig.CachingConfiguration{
			Global: cacheconfig.GlobalCacheConfig{DefaultTTL: 30 * time.Second},
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
				"heroes-ds": {
					DefaultTTL: &ttl,
					RootFields: []cacheconfig.RootFieldCacheConfig{
						{TypeName: "Query", FieldName: "hero", TTL: time.Minute},
					},
				},
			},
		})
		engine, err := NewExecutionEngine(context.Background(), abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{MaxConcurrency: 1})
		require.NoError(t, err)

		caching := engine.config.plannerConfig.Caching
		require.NotNil(t, caching)
		assert.Equal(t, cacheconfig.EffectiveSubgraphConfig{
			Enabled:    true,
			DefaultTTL: time.Minute,
			RootFields: []cacheconfig.RootFieldCacheConfig{
				{TypeName: "Query", FieldName: "hero", TTL: time.Minute},
			},
		}, caching.Resolve("heroes-ds"))

		assert.False(t, engine.config.plannerConfig.DisableIncludeInfo)
		assert.Len(t, engine.postProcessorOptions, 1)
	})

	t.Run("SetCaching for an unknown datasource id fails", func(t *testing.T) {
		engineConf := newCachingTestConfiguration(t)
		engineConf.SetCaching(cacheconfig.CachingConfiguration{
			Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{"no-such-ds": {}},
		})
		_, err := NewExecutionEngine(context.Background(), abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{MaxConcurrency: 1})
		assert.EqualError(t, err, "caching configured for unknown datasource id: no-such-ds")
	})
}
