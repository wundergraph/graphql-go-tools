package graphql_datasource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestGraphQLDataSourceFederationMultipleInterfaceObjects covers a concrete type
// which is a part of more than one interface object within the same subgraph:
// the interface object to jump to has to be selected by the requested field,
// and a selection spanning multiple interface objects has to be split into separate entity fetches.
func TestGraphQLDataSourceFederationMultipleInterfaceObjects(t *testing.T) {
	const definition = `
		interface Alpha {
			id: ID!
			alphaField: String!
		}

		interface Beta {
			id: ID!
			betaField: String!
		}

		type Thing implements Alpha & Beta {
			id: ID!
			name: String!
			alphaField: String!
			betaField: String!
		}

		type Query {
			thing: Thing!
		}`

	// the interface objects are configured in the order of the type declarations in the subgraph SDL
	alphaFirst := []string{"Alpha", "Beta"}
	betaFirst := []string{"Beta", "Alpha"}

	testCases := []struct {
		name             string
		interfaceObjects []string
		fields           []string
	}{
		{name: "field of the first interface object", interfaceObjects: alphaFirst, fields: []string{"alphaField"}},
		{name: "field of the second interface object", interfaceObjects: alphaFirst, fields: []string{"betaField"}},
		{name: "field of the first interface object when it is declared second", interfaceObjects: betaFirst, fields: []string{"alphaField"}},
		{name: "field of the second interface object when it is declared first", interfaceObjects: betaFirst, fields: []string{"betaField"}},
		{name: "fields of both interface objects are split into separate entity fetches", interfaceObjects: alphaFirst, fields: []string{"alphaField", "betaField"}},
		{name: "fields of both interface objects are split into separate entity fetches in reversed order", interfaceObjects: betaFirst, fields: []string{"betaField", "alphaField"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation := fmt.Sprintf(`query Thing { thing { id %s } }`, strings.Join(tc.fields, " "))

			t.Run("run", RunTest(
				definition,
				operation,
				"Thing",
				multipleInterfaceObjectsExpectedPlan(tc.fields),
				multipleInterfaceObjectsPlanConfiguration(t, tc.interfaceObjects),
				WithDefaultPostProcessor(),
			))
		})
	}
}

// multipleInterfaceObjectsPlanConfiguration builds two subgraphs:
// the first defines the entity interfaces Alpha and Beta and the concrete type Thing,
// the second defines Alpha and Beta as interface objects in the given order.
func multipleInterfaceObjectsPlanConfiguration(t *testing.T, interfaceObjectNames []string) plan.Configuration {
	t.Helper()

	factory := &Factory[Configuration]{}

	newDataSource := func(id, sdl, url string, metadata *plan.DataSourceMetadata) plan.DataSource {
		schema, err := NewSchemaConfiguration(sdl, &FederationConfiguration{Enabled: true, ServiceSDL: sdl})
		require.NoError(t, err)

		custom, err := NewConfiguration(ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: url},
			SchemaConfiguration: schema,
		})
		require.NoError(t, err)

		ds, err := plan.NewDataSourceConfiguration[Configuration](id, factory, metadata, custom)
		require.NoError(t, err)

		return ds
	}

	keys := plan.FederationFieldConfigurations{
		{TypeName: "Alpha", SelectionSet: "id"},
		{TypeName: "Beta", SelectionSet: "id"},
		{TypeName: "Thing", SelectionSet: "id"},
	}

	first := newDataSource("first", `
		type Query { thing: Thing! }
		interface Alpha @key(fields: "id") { id: ID! }
		interface Beta @key(fields: "id") { id: ID! }
		type Thing implements Alpha & Beta @key(fields: "id") { id: ID! name: String! }`,
		"http://localhost:4001/graphql",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"thing"}},
				{TypeName: "Thing", FieldNames: []string{"id", "name"}},
				{TypeName: "Alpha", FieldNames: []string{"id"}},
				{TypeName: "Beta", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				EntityInterfaces: []plan.EntityInterfaceConfiguration{
					{InterfaceTypeName: "Alpha", ConcreteTypeNames: []string{"Thing"}},
					{InterfaceTypeName: "Beta", ConcreteTypeNames: []string{"Thing"}},
				},
				Keys: keys,
			},
		},
	)

	interfaceObjects := make([]plan.EntityInterfaceConfiguration, 0, len(interfaceObjectNames))
	for _, name := range interfaceObjectNames {
		interfaceObjects = append(interfaceObjects, plan.EntityInterfaceConfiguration{
			InterfaceTypeName: name,
			ConcreteTypeNames: []string{"Thing"},
		})
	}

	second := newDataSource("second", `
		type Alpha @key(fields: "id") @interfaceObject { id: ID! alphaField: String! }
		type Beta @key(fields: "id") @interfaceObject { id: ID! betaField: String! }`,
		"http://localhost:4002/graphql",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Alpha", FieldNames: []string{"id", "alphaField"}},
				{TypeName: "Beta", FieldNames: []string{"id", "betaField"}},
				{TypeName: "Thing", FieldNames: []string{"id", "alphaField", "betaField"}},
			},
			FederationMetaData: plan.FederationMetaData{
				InterfaceObjects: interfaceObjects,
				Keys:             keys,
			},
		},
	)

	return plan.Configuration{
		DataSources:                  []plan.DataSource{first, second},
		DisableResolveFieldPositions: true,
	}
}

// multipleInterfaceObjectsExpectedPlan expects a root fetch for the thing
// followed by one entity fetch per requested field, jumping to the interface object defining the field.
func multipleInterfaceObjectsExpectedPlan(fields []string) *plan.SynchronousResponsePlan {
	interfaceObjectByField := map[string]string{
		"alphaField": "Alpha",
		"betaField":  "Beta",
	}

	fetches := []*resolve.FetchTreeNode{
		resolve.Single(&resolve.SingleFetch{
			FetchConfiguration: resolve.FetchConfiguration{
				Input:          `{"method":"POST","url":"http://localhost:4001/graphql","body":{"query":"{thing {id __typename}}"}}`,
				PostProcessing: DefaultPostProcessingConfiguration,
				DataSource:     &Source{},
			},
			DataSourceIdentifier: []byte("graphql_datasource.Source"),
		}),
	}

	responseFields := []*resolve.Field{
		{
			Name:  []byte("id"),
			Value: &resolve.Scalar{Path: []string{"id"}},
		},
	}

	for i, field := range fields {
		interfaceObject := interfaceObjectByField[field]

		fetches = append(fetches, resolve.SingleWithPath(
			multipleInterfaceObjectsEntityFetch(i+1, interfaceObject, field),
			"thing",
			resolve.ObjectPath("thing"),
		))

		responseFields = append(responseFields, &resolve.Field{
			Name:  []byte(field),
			Value: &resolve.String{Path: []string{field}},
		})
	}

	return &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: resolve.Sequence(fetches...),
			Data: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("thing"),
						Value: &resolve.Object{
							Path:          []string{"thing"},
							Fields:        responseFields,
							PossibleTypes: map[string]struct{}{"Thing": {}},
							TypeName:      "Thing",
						},
					},
				},
			},
		},
	}
}

func multipleInterfaceObjectsEntityFetch(fetchID int, interfaceObjectName, fieldName string) *resolve.SingleFetch {
	onTypeNames := [][]byte{[]byte("Thing"), []byte(interfaceObjectName)}

	return &resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetchID,
			DependsOnFetchIDs: []int{0},
		},
		FetchConfiguration: resolve.FetchConfiguration{
			Input: fmt.Sprintf(`{"method":"POST","url":"http://localhost:4002/graphql","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on %s {%s}}}","variables":{"representations":[$$0$$]}}}`, interfaceObjectName, fieldName),
			Variables: []resolve.Variable{
				&resolve.ResolvableObjectVariable{
					Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name: []byte("__typename"),
								Value: &resolve.StaticString{
									Path:  []string{"__typename"},
									Value: interfaceObjectName,
								},
								OnTypeNames: onTypeNames,
							},
							{
								Name:        []byte("id"),
								Value:       &resolve.Scalar{Path: []string{"id"}},
								OnTypeNames: onTypeNames,
							},
						},
					}),
				},
			},
			RequiresEntityFetch:                   true,
			PostProcessing:                        SingleEntityPostProcessingConfiguration,
			DataSource:                            &Source{},
			SetTemplateOutputToNullOnVariableNull: true,
		},
		DataSourceIdentifier: []byte("graphql_datasource.Source"),
	}
}
