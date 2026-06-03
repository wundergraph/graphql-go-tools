package plan

import (
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

type requestScopedUnionTestDataSource struct {
	*DataSourceMetadata

	id   string
	name string
	hash DSHash
}

func newRequestScopedUnionTestDataSource() *requestScopedUnionTestDataSource {
	metadata := &DataSourceMetadata{
		ChildNodes: TypeFields{
			{
				TypeName:   "Viewer",
				FieldNames: []string{"name", "email", "handle", "posts"},
			},
		},
	}
	metadata.InitNodesIndex()

	return &requestScopedUnionTestDataSource{
		DataSourceMetadata: metadata,
		id:                 "viewer",
		name:               "viewer",
		hash:               DSHash(1),
	}
}

func (*requestScopedUnionTestDataSource) UpstreamSchema() (*ast.Document, bool) {
	return nil, false
}

func (*requestScopedUnionTestDataSource) PlanningBehavior() DataSourcePlanningBehavior {
	return DataSourcePlanningBehavior{}
}

func (d *requestScopedUnionTestDataSource) Id() string {
	return d.id
}

func (d *requestScopedUnionTestDataSource) Name() string {
	return d.name
}

func (d *requestScopedUnionTestDataSource) Hash() DSHash {
	return d.hash
}

func (d *requestScopedUnionTestDataSource) FederationConfiguration() FederationMetaData {
	return d.FederationMetaData
}

func (*requestScopedUnionTestDataSource) CreatePlannerConfiguration(abstractlogger.Logger, *objectFetchConfiguration, *plannerPathsConfiguration, *Configuration) PlannerConfiguration {
	return nil
}

func (*requestScopedUnionTestDataSource) GetCostConfig() *DataSourceCostConfig {
	return nil
}

func TestRequestScopedSelectionUnion_DirectiveConflictsUseSyntheticAliases(t *testing.T) {
	t.Parallel()

	definition := unsafeparser.ParseGraphqlDocumentString(`
		directive @tag(name: String!) on FIELD

		type Query {
			currentViewer: Viewer
			article: Article
		}

		type Article {
			currentViewer: Viewer
		}

		type Viewer {
			name: String!
		}
	`)
	operation := unsafeparser.ParseGraphqlDocumentString(`
		query Widening {
			currentViewer {
				name @tag(name: "root")
			}
			article {
				currentViewer {
					name @tag(name: "child")
				}
			}
		}
	`)

	operationDefinitionRef := operation.RootNodes[0].Ref
	rootSelectionSetRef := operation.OperationDefinitions[operationDefinitionRef].SelectionSet
	rootFieldRefs := operation.SelectionSetFieldRefs(rootSelectionSetRef)
	require.Len(t, rootFieldRefs, 2)

	rootViewerSelectionSetRef, ok := operation.FieldSelectionSet(rootFieldRefs[0])
	require.True(t, ok)

	articleSelectionSetRef, ok := operation.FieldSelectionSet(rootFieldRefs[1])
	require.True(t, ok)
	articleFieldRefs := operation.SelectionSetFieldRefs(articleSelectionSetRef)
	require.Len(t, articleFieldRefs, 1)

	childViewerSelectionSetRef, ok := operation.FieldSelectionSet(articleFieldRefs[0])
	require.True(t, ok)

	viewerTypeNode, ok := definition.Index.FirstNodeByNameStr("Viewer")
	require.True(t, ok)

	ds := newRequestScopedUnionTestDataSource()
	union := newRequestScopedSelectionUnion()

	require.True(t, union.mergeSelectionSet(&operation, &definition, rootViewerSelectionSetRef, viewerTypeNode, ds))
	require.True(t, union.mergeSelectionSet(&operation, &definition, childViewerSelectionSetRef, viewerTypeNode, ds))

	assert.Equal(t,
		`__request_scoped__name_0: name @tag(name: "child")`,
		union.renderMissingFragment(&operation, &definition, rootViewerSelectionSetRef, viewerTypeNode, ds),
	)
	assert.Equal(t,
		`__request_scoped__name_1: name @tag(name: "root")`,
		union.renderMissingFragment(&operation, &definition, childViewerSelectionSetRef, viewerTypeNode, ds),
	)
}
