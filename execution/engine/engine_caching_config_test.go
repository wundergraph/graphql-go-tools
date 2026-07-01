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

// TestSetCachingWiring pins the engine entry point: SetCaching (keyed by
// datasource ID) reaches the planner's CacheConfigProviders, force-enables
// FetchInfo, and produces the postprocess EnableCaching option; without
// SetCaching none of that is constructed.
func TestSetCachingWiring(t *testing.T) {
	t.Run("without SetCaching nothing is wired", func(t *testing.T) {
		engineConf := newCachingTestConfiguration(t)
		engine, err := NewExecutionEngine(context.Background(), abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{MaxConcurrency: 1})
		require.NoError(t, err)
		assert.Nil(t, engine.config.plannerConfig.CacheConfigProviders)
		assert.Nil(t, engine.postProcessorOptions)
	})

	t.Run("SetCaching wires planner providers, FetchInfo, and postprocess option", func(t *testing.T) {
		engineConf := newCachingTestConfiguration(t)
		engineConf.plannerConfig.DisableIncludeInfo = true // must be force-overridden by caching
		engineConf.SetCaching(map[string]cacheconfig.CachingConfiguration{
			"heroes-ds": {
				RootFields: []cacheconfig.RootFieldCachePolicy{
					{TypeName: "Query", FieldName: "hero", CacheName: "heroes", TTL: time.Minute},
				},
			},
		})
		engine, err := NewExecutionEngine(context.Background(), abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{MaxConcurrency: 1})
		require.NoError(t, err)

		providers := engine.config.plannerConfig.CacheConfigProviders
		require.Len(t, providers, 1)
		policy, ok := providers["heroes-ds"].RootFieldPolicy("Query", "hero")
		assert.True(t, ok)
		assert.Equal(t, cacheconfig.RootFieldCachePolicy{
			TypeName:  "Query",
			FieldName: "hero",
			CacheName: "heroes",
			TTL:       time.Minute,
		}, policy)

		assert.False(t, engine.config.plannerConfig.DisableIncludeInfo)
		assert.Len(t, engine.postProcessorOptions, 1)
	})

	t.Run("SetCaching for an unknown datasource id fails", func(t *testing.T) {
		engineConf := newCachingTestConfiguration(t)
		engineConf.SetCaching(map[string]cacheconfig.CachingConfiguration{
			"no-such-ds": {},
		})
		_, err := NewExecutionEngine(context.Background(), abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{MaxConcurrency: 1})
		assert.EqualError(t, err, "caching configured for unknown datasource id: no-such-ds")
	})
}
