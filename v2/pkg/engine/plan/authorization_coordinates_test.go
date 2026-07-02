package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// TestPlanner_CollectsAuthorizationCoordinates demonstrates the effect of the
// CollectAuthorizationCoordinates step that Planner.Plan runs at the end of planning: the planned
// response carries, on its Info, the {DataSourceID, coordinate} of every selected field that has an
// authorization rule. Pre-fetch field authorization later decides all of these up front.
func TestPlanner_CollectsAuthorizationCoordinates(t *testing.T) {
	const schema = `
		scalar JSON

		schema { query: Query }

		type Query {
			hero: Character!
		}

		type Character {
			info: JSON!
			infos: [JSON!]!
		}
	`

	dsConfig := dsb().
		Id("hero-service").
		Schema(schema).
		RootNode("Query", "hero").
		ChildNode("Character", "info", "infos").
		DS()

	planResponse := func(t *testing.T, operation string, fields FieldConfigurations) *resolve.GraphQLResponse {
		t.Helper()

		def := unsafeparser.ParseGraphqlDocumentString(schema)
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

		var report operationreport.Report
		astnormalization.NewNormalizer(true, true).NormalizeOperation(&op, &def, &report)
		astvalidation.DefaultOperationValidator().Validate(&op, &def, &report)
		require.False(t, report.HasErrors(), report.Error())

		p, err := NewPlanner(Configuration{
			DisableResolveFieldPositions: true,
			Fields:                       fields,
			DataSources:                  []DataSource{dsConfig},
		})
		require.NoError(t, err)

		plan := p.Plan(&op, &def, "", &report)
		require.False(t, report.HasErrors(), report.Error())

		return plan.(*SynchronousResponsePlan).Response
	}

	t.Run("no authorization rules leaves the coordinates empty", func(t *testing.T) {
		response := planResponse(t, `{ hero { info } }`, nil)
		assert.Nil(t, response.Info.AuthorizationCoordinates)
	})

	t.Run("protected root field is collected from the fetch", func(t *testing.T) {
		response := planResponse(t, `{ hero { info } }`, FieldConfigurations{
			{TypeName: "Query", FieldName: "hero", HasAuthorizationRule: true},
		})
		assert.Equal(t, []resolve.AuthorizationCoordinate{
			{DataSourceID: "hero-service", Coordinate: resolve.GraphCoordinate{TypeName: "Query", FieldName: "hero"}},
		}, response.Info.AuthorizationCoordinates)
	})

	t.Run("protected nested field is collected from the data tree", func(t *testing.T) {
		response := planResponse(t, `{ hero { info } }`, FieldConfigurations{
			{TypeName: "Character", FieldName: "info", HasAuthorizationRule: true},
		})
		assert.Equal(t, []resolve.AuthorizationCoordinate{
			{DataSourceID: "hero-service", Coordinate: resolve.GraphCoordinate{TypeName: "Character", FieldName: "info"}},
		}, response.Info.AuthorizationCoordinates)
	})

	t.Run("multiple protected fields are collected and deterministically sorted", func(t *testing.T) {
		response := planResponse(t, `{ hero { info infos } }`, FieldConfigurations{
			{TypeName: "Query", FieldName: "hero", HasAuthorizationRule: true},
			{TypeName: "Character", FieldName: "info", HasAuthorizationRule: true},
		})
		assert.Equal(t, []resolve.AuthorizationCoordinate{
			{DataSourceID: "hero-service", Coordinate: resolve.GraphCoordinate{TypeName: "Character", FieldName: "info"}},
			{DataSourceID: "hero-service", Coordinate: resolve.GraphCoordinate{TypeName: "Query", FieldName: "hero"}},
		}, response.Info.AuthorizationCoordinates)
	})
}
