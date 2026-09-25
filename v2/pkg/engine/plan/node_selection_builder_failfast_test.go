package plan

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// TestUnresolvableFieldFailsFast plans an operation with a field which exists in the
// federated graph definition but on no datasource (a config mismatch). SelectNodes must
// report the unresolved field right away once fallback key jumps were tried, instead of
// spinning the selection loop to its 100-iteration bound without making progress.
func TestUnresolvableFieldFailsFast(t *testing.T) {
	def := `
type Query { user: User }
type User { id: ID! name: String }
`
	subgraphSchema := `
type Query { user: User }
type User @key(fields: "id") { id: ID! }
`
	definition := unsafeparser.ParseGraphqlDocumentString(def)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))

	var sb strings.Builder
	sb.WriteString("query { user { name ")
	for i := range 500 {
		fmt.Fprintf(&sb, "a%d: id ", i)
	}
	sb.WriteString("} }")
	operation := unsafeparser.ParseGraphqlDocumentString(sb.String())

	report := &operationreport.Report{}
	astnormalization.NewNormalizer(true, true).NormalizeOperation(&operation, &definition, report)
	require.False(t, report.HasErrors(), report.Error())
	astvalidation.DefaultOperationValidator().Validate(&operation, &definition, report)
	require.False(t, report.HasErrors(), report.Error())

	ds := dsb().Hash(11).Id("users").
		RootNode("Query", "user").
		RootNode("User", "id").
		KeysMetadata(FederationFieldConfigurations{{TypeName: "User", SelectionSet: "id"}}).
		SchemaMergedWithBase(subgraphSchema).DS()

	planner, err := NewPlanner(Configuration{DataSources: []DataSource{ds}, DisableIncludeInfo: true, DisableIncludeFieldDependencies: true})
	require.NoError(t, err)

	start := time.Now()
	planner.Plan(&operation, &definition, "", report)
	t.Logf("plan duration: %s, has errors: %v", time.Since(start), report.HasErrors())
	require.True(t, report.HasErrors())
	require.Contains(t, report.Error(), "could not resolve a field")
}
