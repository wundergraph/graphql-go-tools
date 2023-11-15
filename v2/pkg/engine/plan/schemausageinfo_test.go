package plan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astjson"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/asttransform"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

const schemaUsageInfoTestSchema = `

directive @defer on FIELD

directive @flushInterval(milliSeconds: Int!) on QUERY | SUBSCRIPTION

directive @stream(initialBatchSize: Int) on FIELD

union SearchResult = Human | Droid | Starship

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
    search(name: String!): SearchResult
	searchResults(name: String!, filter: SearchFilter, filter2: SearchFilter, enumValue: Episode enumList: [Episode] enumList2: [Episode] filterList: [SearchFilter]): [SearchResult]
}

input SearchFilter {
	excludeName: String
	enumField: Episode
}

type Mutation {
    createReview(episode: Episode!, review: ReviewInput!): Review
}

type Subscription {
    remainingJedis: Int!
	newReviews: Review
}

input ReviewInput {
    stars: Int!
    commentary: String
}

type Review {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character {
    name: String!
    friends: [Character]
}

type Human implements Character {
    name: String!
    height: String!
    friends: [Character]
	inlineName(name: String!): String!
}

type Droid implements Character {
    name: String!
    primaryFunction: String!
    friends: [Character]
	favoriteEpisode: Episode
}

interface Vehicle {
	length: Float!
}

type Starship implements Vehicle {
    name: String!
    length: Float!
}
`

func TestGetSchemaUsageInfo(t *testing.T) {
	operation := `
		query Search($name: String!, $filter2: SearchFilter $enumValue: Episode $enumList: [Episode] $filterList: [SearchFilter]) {
			searchResults(name: $name, filter: {excludeName: "Jannik"} filter2: $filter2, enumValue: $enumValue enumList: $enumList, enumList2: [JEDI, EMPIRE] filterList: $filterList ) {
				__typename
				... on Human {
					name
					inlineName(name: "Jannik")
				}
				... on Droid {
					name
				}
				... on Starship {
					length
				}
			}
			hero {
				name
			}
		}
`

	variables := `{"name":"Jannik","filter2":{"enumField":"NEWHOPE"},"enumValue":"EMPIRE","enumList":["JEDI","EMPIRE","NEWHOPE"],"filterList":[{"excludeName":"Jannik"},{"enumField":"JEDI","excludeName":"Jannik"}]}`

	def := unsafeparser.ParseGraphqlDocumentString(schemaUsageInfoTestSchema)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	if err != nil {
		t.Fatal(err)
	}
	report := &operationreport.Report{}
	norm := astnormalization.NewNormalizer(true, true)
	norm.NormalizeOperation(&op, &def, report)
	valid := astvalidation.DefaultOperationValidator()
	valid.Validate(&op, &def, report)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := NewPlanner(ctx, Configuration{
		DisableResolveFieldPositions: true,
		IncludeInfo:                  true,
		DataSources: []DataSourceConfiguration{
			{
				RootNodes: []TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"searchResults", "hero"},
					},
				},
				ChildNodes: []TypeField{
					{
						TypeName:   "Human",
						FieldNames: []string{"name", "inlineName"},
					},
					{
						TypeName:   "Droid",
						FieldNames: []string{"name"},
					},
					{
						TypeName:   "Starship",
						FieldNames: []string{"length"},
					},
					{
						TypeName:   "SearchResult",
						FieldNames: []string{"__typename"},
					},
					{
						TypeName:   "Character",
						FieldNames: []string{"name", "friends"},
					},
				},
				ID: "https://swapi.dev/api",
				Factory: &FakeFactory{
					upstreamSchema: &def,
				},
				Custom: []byte(fmt.Sprintf(`{"UpstreamSchema":"%s"}`, schemaUsageInfoTestSchema)),
			},
		},
	})
	generatedPlan := p.Plan(&op, &def, "Search", report)
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
	vars := &astjson.JSON{}
	err = vars.ParseObject([]byte(variables))
	assert.NoError(t, err)
	extracted, err := vars.AppendObject(op.Input.Variables)
	assert.NoError(t, err)
	vars.MergeNodes(vars.RootNode, extracted)
	mergedVariables := &bytes.Buffer{}
	err = vars.PrintRoot(mergedVariables)
	assert.NoError(t, err)
	syncUsage, err := GetSchemaUsageInfo(generatedPlan, &op, &def, mergedVariables.Bytes())
	assert.NoError(t, err)
	subscriptionUsage, err := GetSchemaUsageInfo(&SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Response: generatedPlan.(*SynchronousResponsePlan).Response,
		},
	}, &op, &def, mergedVariables.Bytes())
	assert.NoError(t, err)
	expected := &SchemaUsageInfo{
		OperationType: ast.OperationTypeQuery,
		TypeFields: []TypeFieldUsageInfo{
			{
				FieldName:          "searchResults",
				EnclosingTypeNames: []string{"Query"},
				Path:               []string{"searchResults"},
				FieldTypeName:      "SearchResult",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:               []string{"searchResults", "__typename"},
				EnclosingTypeNames: []string{"SearchResult"},
				FieldName:          "__typename",
				FieldTypeName:      "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:               []string{"searchResults", "name"},
				EnclosingTypeNames: []string{"Human"},
				FieldName:          "name",
				FieldTypeName:      "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:               []string{"searchResults", "inlineName"},
				EnclosingTypeNames: []string{"Human"},
				FieldName:          "inlineName",
				FieldTypeName:      "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:               []string{"searchResults", "name"},
				EnclosingTypeNames: []string{"Droid"},
				FieldName:          "name",
				FieldTypeName:      "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:               []string{"searchResults", "length"},
				EnclosingTypeNames: []string{"Starship"},
				FieldTypeName:      "Float",
				FieldName:          "length",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				FieldName:          "hero",
				EnclosingTypeNames: []string{"Query"},
				Path:               []string{"hero"},
				FieldTypeName:      "Character",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				FieldName:          "name",
				EnclosingTypeNames: []string{"Character"},
				Path:               []string{"hero", "name"},
				FieldTypeName:      "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
		},
		Arguments: []ArgumentUsageInfo{
			{
				EnclosingTypeName: "Query",
				FieldName:         "searchResults",
				ArgumentName:      "name",
				ArgumentTypeName:  "String",
			},
			{
				EnclosingTypeName: "Query",
				FieldName:         "searchResults",
				ArgumentName:      "filter",
				ArgumentTypeName:  "SearchFilter",
			},
			{
				EnclosingTypeName: "Query",
				FieldName:         "searchResults",
				ArgumentName:      "filter2",
				ArgumentTypeName:  "SearchFilter",
			},
			{
				EnclosingTypeName: "Query",
				FieldName:         "searchResults",
				ArgumentName:      "enumValue",
				ArgumentTypeName:  "Episode",
			},
			{
				EnclosingTypeName: "Query",
				FieldName:         "searchResults",
				ArgumentName:      "enumList",
				ArgumentTypeName:  "Episode",
			},
			{
				EnclosingTypeName: "Query",
				FieldName:         "searchResults",
				ArgumentName:      "enumList2",
				ArgumentTypeName:  "Episode",
			},
			{
				EnclosingTypeName: "Query",
				FieldName:         "searchResults",
				ArgumentName:      "filterList",
				ArgumentTypeName:  "SearchFilter",
			},
			{
				EnclosingTypeName: "Human",
				FieldName:         "inlineName",
				ArgumentName:      "name",
				ArgumentTypeName:  "String",
			},
		},
		InputTypeFields: []InputTypeFieldUsageInfo{
			{
				Count:          2,
				FieldTypeName:  "String",
				IsRootVariable: true,
			},
			{
				Count:              1,
				FieldName:          "enumField",
				FieldTypeName:      "Episode",
				EnclosingTypeNames: []string{"SearchFilter"},
				EnumValues:         []string{"NEWHOPE"},
				IsEnumField:        true,
			},
			{
				Count:          5,
				FieldTypeName:  "SearchFilter",
				IsRootVariable: true,
			},
			{
				Count:          1,
				FieldTypeName:  "Episode",
				EnumValues:     []string{"EMPIRE"},
				IsEnumField:    true,
				IsRootVariable: true,
			},
			{
				Count:          1,
				FieldTypeName:  "Episode",
				EnumValues:     []string{"JEDI", "EMPIRE", "NEWHOPE"},
				IsEnumField:    true,
				IsRootVariable: true,
			},
			{
				Count:              3,
				FieldName:          "excludeName",
				FieldTypeName:      "String",
				EnclosingTypeNames: []string{"SearchFilter"},
			},
			{
				Count:              1,
				FieldName:          "enumField",
				FieldTypeName:      "Episode",
				EnclosingTypeNames: []string{"SearchFilter"},
				EnumValues:         []string{"JEDI"},
				IsEnumField:        true,
			},
			{
				Count:          1,
				FieldTypeName:  "Episode",
				EnumValues:     []string{"JEDI", "EMPIRE"},
				IsEnumField:    true,
				IsRootVariable: true,
			},
		},
	}
	assert.Equal(t, expected.OperationType, syncUsage.OperationType)
	assert.Equal(t, len(expected.TypeFields), len(syncUsage.TypeFields))
	for i := range expected.TypeFields {
		assert.Equal(t, expected.TypeFields[i].FieldName, syncUsage.TypeFields[i].FieldName, "Field %d", i)
		assert.Equal(t, expected.TypeFields[i].EnclosingTypeNames, syncUsage.TypeFields[i].EnclosingTypeNames, "Field %d", i)
		assert.Equal(t, expected.TypeFields[i].Path, syncUsage.TypeFields[i].Path, "Field %d", i)
		assert.Equal(t, expected.TypeFields[i].FieldTypeName, syncUsage.TypeFields[i].FieldTypeName, "Field %d", i)
		assert.Equal(t, expected.TypeFields[i].Source.IDs, syncUsage.TypeFields[i].Source.IDs, "Field %d", i)
	}
	assert.Equal(t, len(expected.Arguments), len(syncUsage.Arguments))
	for i := range expected.Arguments {
		assert.Equal(t, expected.Arguments[i].FieldName, syncUsage.Arguments[i].FieldName, "Argument %d", i)
		assert.Equal(t, expected.Arguments[i].EnclosingTypeName, syncUsage.Arguments[i].EnclosingTypeName, "Argument %d", i)
		assert.Equal(t, expected.Arguments[i].ArgumentName, syncUsage.Arguments[i].ArgumentName, "Argument %d", i)
		assert.Equal(t, expected.Arguments[i].ArgumentTypeName, syncUsage.Arguments[i].ArgumentTypeName, "Argument %d", i)
	}
	assert.Equal(t, len(expected.InputTypeFields), len(syncUsage.InputTypeFields))
	for i := range expected.InputTypeFields {
		assert.Equal(t, expected.InputTypeFields[i].Count, syncUsage.InputTypeFields[i].Count, "InputTypeField %d", i)
		assert.Equal(t, expected.InputTypeFields[i].FieldName, syncUsage.InputTypeFields[i].FieldName, "InputTypeField %d", i)
		assert.Equal(t, expected.InputTypeFields[i].FieldTypeName, syncUsage.InputTypeFields[i].FieldTypeName, "InputTypeField %d", i)
		assert.Equal(t, expected.InputTypeFields[i].EnclosingTypeNames, syncUsage.InputTypeFields[i].EnclosingTypeNames, "InputTypeField %d", i)
	}
	assert.Equal(t, expected, syncUsage)
	assert.Equal(t, expected, subscriptionUsage)
}

type StatefulSource struct {
}

func (s *StatefulSource) Start(ctx context.Context) {

}

type FakeFactory struct {
	upstreamSchema *ast.Document
}

func (f *FakeFactory) Planner(ctx context.Context) DataSourcePlanner {
	source := &StatefulSource{}
	go source.Start(ctx)
	return &FakePlanner{
		source:         source,
		upstreamSchema: f.upstreamSchema,
	}
}

type FakePlanner struct {
	source         *StatefulSource
	upstreamSchema *ast.Document
}

func (f *FakePlanner) UpstreamSchema(dataSourceConfig DataSourceConfiguration) *ast.Document {
	return f.upstreamSchema
}

func (f *FakePlanner) EnterDocument(operation, definition *ast.Document) {

}

func (f *FakePlanner) Register(visitor *Visitor, _ DataSourceConfiguration, _ DataSourcePlannerConfiguration) error {
	visitor.Walker.RegisterEnterDocumentVisitor(f)
	return nil
}

func (f *FakePlanner) ConfigureFetch() resolve.FetchConfiguration {
	return resolve.FetchConfiguration{
		DataSource: &FakeDataSource{
			source: f.source,
		},
	}
}

func (f *FakePlanner) ConfigureSubscription() SubscriptionConfiguration {
	return SubscriptionConfiguration{}
}

func (f *FakePlanner) DataSourcePlanningBehavior() DataSourcePlanningBehavior {
	return DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (f *FakePlanner) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	return
}

type FakeDataSource struct {
	source *StatefulSource
}

func (f *FakeDataSource) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	return
}
