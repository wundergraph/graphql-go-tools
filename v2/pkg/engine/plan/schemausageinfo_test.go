package plan

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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
						FieldNames: []string{"searchResults"},
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
				FieldName: "searchResults",
				TypeNames: []string{"Query"},
				Path:      []string{"searchResults"},
				NamedType: "SearchResult",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "__typename"},
				TypeNames: []string{"SearchResult"},
				FieldName: "__typename",
				NamedType: "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "name"},
				TypeNames: []string{"Human"},
				FieldName: "name",
				NamedType: "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "inlineName"},
				TypeNames: []string{"Human"},
				FieldName: "inlineName",
				NamedType: "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "name"},
				TypeNames: []string{"Droid"},
				FieldName: "name",
				NamedType: "String",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "length"},
				TypeNames: []string{"Starship"},
				NamedType: "Float",
				FieldName: "length",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
		},
		Arguments: []ArgumentUsageInfo{
			{
				NamedType:        "Query",
				FieldName:        "searchResults",
				ArgumentName:     "name",
				ArgumentTypeName: "String",
			},
			{
				NamedType:        "Query",
				FieldName:        "searchResults",
				ArgumentName:     "filter",
				ArgumentTypeName: "SearchFilter",
			},
			{
				NamedType:        "Query",
				FieldName:        "searchResults",
				ArgumentName:     "filter2",
				ArgumentTypeName: "SearchFilter",
			},
			{
				NamedType:        "Query",
				FieldName:        "searchResults",
				ArgumentName:     "enumValue",
				ArgumentTypeName: "Episode",
			},
			{
				NamedType:        "Query",
				FieldName:        "searchResults",
				ArgumentName:     "enumList",
				ArgumentTypeName: "Episode",
			},
			{
				NamedType:        "Query",
				FieldName:        "searchResults",
				ArgumentName:     "enumList2",
				ArgumentTypeName: "Episode",
			},
			{
				NamedType:        "Query",
				FieldName:        "searchResults",
				ArgumentName:     "filterList",
				ArgumentTypeName: "SearchFilter",
			},
			{
				NamedType:        "Human",
				FieldName:        "inlineName",
				ArgumentName:     "name",
				ArgumentTypeName: "String",
			},
		},
		InputTypeFields: []TypeFieldUsageInfo{
			{
				Count:     2,
				NamedType: "String",
			},
			{
				Count:      1,
				FieldName:  "enumField",
				NamedType:  "Episode",
				TypeNames:  []string{"SearchFilter"},
				EnumValues: []string{"NEWHOPE"},
			},
			{
				Count:     5,
				NamedType: "SearchFilter",
			},
			{
				Count:      1,
				NamedType:  "Episode",
				EnumValues: []string{"EMPIRE"},
			},
			{
				Count:      1,
				NamedType:  "Episode",
				EnumValues: []string{"JEDI", "EMPIRE", "NEWHOPE"},
			},
			{
				Count:     3,
				FieldName: "excludeName",
				NamedType: "String",
				TypeNames: []string{"SearchFilter"},
			},
			{
				Count:      1,
				FieldName:  "enumField",
				NamedType:  "Episode",
				TypeNames:  []string{"SearchFilter"},
				EnumValues: []string{"JEDI"},
			},
			{
				Count:      1,
				NamedType:  "Episode",
				EnumValues: []string{"JEDI", "EMPIRE"},
			},
		},
	}
	assert.Equal(t, expected.OperationType, syncUsage.OperationType)
	assert.Equal(t, len(expected.TypeFields), len(syncUsage.TypeFields))
	for i := range expected.TypeFields {
		assert.Equal(t, expected.TypeFields[i].FieldName, syncUsage.TypeFields[i].FieldName, "Field %d", i)
		assert.Equal(t, expected.TypeFields[i].TypeNames, syncUsage.TypeFields[i].TypeNames, "Field %d", i)
		assert.Equal(t, expected.TypeFields[i].Path, syncUsage.TypeFields[i].Path, "Field %d", i)
		assert.Equal(t, expected.TypeFields[i].NamedType, syncUsage.TypeFields[i].NamedType, "Field %d", i)
		assert.Equal(t, expected.TypeFields[i].Source.IDs, syncUsage.TypeFields[i].Source.IDs, "Field %d", i)
	}
	assert.Equal(t, len(expected.Arguments), len(syncUsage.Arguments))
	for i := range expected.Arguments {
		assert.Equal(t, expected.Arguments[i].FieldName, syncUsage.Arguments[i].FieldName, "Argument %d", i)
		assert.Equal(t, expected.Arguments[i].NamedType, syncUsage.Arguments[i].NamedType, "Argument %d", i)
		assert.Equal(t, expected.Arguments[i].ArgumentName, syncUsage.Arguments[i].ArgumentName, "Argument %d", i)
		assert.Equal(t, expected.Arguments[i].ArgumentTypeName, syncUsage.Arguments[i].ArgumentTypeName, "Argument %d", i)
	}
	assert.Equal(t, len(expected.InputTypeFields), len(syncUsage.InputTypeFields))
	for i := range expected.InputTypeFields {
		assert.Equal(t, expected.InputTypeFields[i].Count, syncUsage.InputTypeFields[i].Count, "InputTypeField %d", i)
		assert.Equal(t, expected.InputTypeFields[i].FieldName, syncUsage.InputTypeFields[i].FieldName, "InputTypeField %d", i)
		assert.Equal(t, expected.InputTypeFields[i].NamedType, syncUsage.InputTypeFields[i].NamedType, "InputTypeField %d", i)
		assert.Equal(t, expected.InputTypeFields[i].TypeNames, syncUsage.InputTypeFields[i].TypeNames, "InputTypeField %d", i)
	}
	assert.Equal(t, expected, syncUsage)
	assert.Equal(t, expected, subscriptionUsage)
}
