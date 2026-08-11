package plan

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// TestCacheProvidesDataVisitorGating pins the planner no-op gate: the visitor
// is constructed only when a caching configuration is set.
func TestCacheProvidesDataVisitorGating(t *testing.T) {
	t.Run("not constructed without a caching configuration", func(t *testing.T) {
		p, err := NewPlanner(Configuration{
			DataSources: []DataSource{testDefinitionDSConfiguration},
		})
		require.NoError(t, err)
		assert.Nil(t, p.cacheProvidesData)
	})

	t.Run("constructed with a caching configuration", func(t *testing.T) {
		p, err := NewPlanner(Configuration{
			DataSources: []DataSource{testDefinitionDSConfiguration},
			Caching:     &cacheconfig.CachingConfiguration{},
		})
		require.NoError(t, err)
		assert.NotNil(t, p.cacheProvidesData)
	})
}

// TestCacheProvidesDataSecondWalkKeepsPlanIdentical proves the second walk
// never re-runs the planning visitor and never rebuilds the plan: with the
// visitor enabled by an empty caching configuration, the produced plan is
// identical to the plan produced without any caching configuration.
func TestCacheProvidesDataSecondWalkKeepsPlanIdentical(t *testing.T) {
	planOnce := func(t *testing.T, config Configuration) Plan {
		t.Helper()

		def := unsafeparser.ParseGraphqlDocumentString(testDefinition)
		op := unsafeparser.ParseGraphqlDocumentString(`
			query SearchResults {
				searchResults {
					... on Character {
						name
					}
					... on Vehicle {
						length
					}
				}
			}
		`)
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		require.NoError(t, err)
		var report operationreport.Report
		norm := astnormalization.NewNormalizer(true, true)
		norm.NormalizeOperation(&op, &def, &report)
		valid := astvalidation.DefaultOperationValidator()
		valid.Validate(&op, &def, &report)
		require.False(t, report.HasErrors(), "unexpected report errors: %s", report.Error())

		p, err := NewPlanner(config)
		require.NoError(t, err)
		result := p.Plan(&op, &def, "SearchResults", &report)
		require.False(t, report.HasErrors(), "unexpected report errors: %s", report.Error())
		return result
	}

	baseConfig := func() Configuration {
		return Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{testDefinitionDSConfiguration},
		}
	}

	withoutCaching := planOnce(t, baseConfig())

	cachingConfig := baseConfig()
	cachingConfig.Caching = &cacheconfig.CachingConfiguration{}
	withCaching := planOnce(t, cachingConfig)

	formatterConfig := map[reflect.Type]any{
		reflect.TypeFor[[]byte](): func(b []byte) string { return fmt.Sprintf(`"%s"`, string(b)) },
		reflect.TypeOf(map[string]struct{}{}): func(m map[string]struct{}) string {
			keys := slices.Sorted(maps.Keys(m))
			keysPrinted, _ := json.Marshal(keys)
			return string(keysPrinted)
		},
	}
	prettyCfg := &pretty.Config{
		Diffable:          true,
		IncludeUnexported: false,
		Formatter:         formatterConfig,
	}

	assert.Equal(t, prettyCfg.Sprint(withoutCaching), prettyCfg.Sprint(withCaching))
}
