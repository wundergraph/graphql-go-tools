package plan

import (
	"encoding/json"
	"strings"
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

func TestPlanSubscriptionFilter(t *testing.T) {

	testLogic := func(t *testing.T, definition, operation, operationName string, config Configuration, report *operationreport.Report) Plan {
		t.Helper()

		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		if err != nil {
			t.Fatal(err)
		}
		norm := astnormalization.NewNormalizer(true, true)
		norm.NormalizeOperation(&op, &def, report)
		valid := astvalidation.DefaultOperationValidator()
		valid.Validate(&op, &def, report)

		p, err := NewPlanner(config)
		require.NoError(t, err)

		pp := p.Plan(&op, &def, operationName, report)

		return pp
	}

	test := func(definition, operation, operationName string, expectedPlan Plan, config Configuration) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			var report operationreport.Report
			plan := testLogic(t, definition, operation, operationName, config, &report)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			assert.Equal(t, expectedPlan, plan)

			toJson := func(v interface{}) string {
				b := &strings.Builder{}
				e := json.NewEncoder(b)
				e.SetIndent("", " ")
				_ = e.Encode(v)
				return b.String()
			}

			assert.Equal(t, toJson(expectedPlan), toJson(plan))

		}
	}

	schema := `
			schema {
				query: Query
				subscription: Subscription
			}
			
			type Query {
				heroByID(id: ID!): Hero
			}

			type Subscription {
				heroByID(id: ID!): Hero
				heroByIDInput(input: Input!): Hero
				heroByIDMultipleArgs(one: Int!, two: Float!): Hero
				heroByIDs(ids: [ID!]!): Hero
			}
			
			input Input {
				one: Int!
				two: Boolean!
			}

			type Hero {
				id: ID!
				name: String!
			}
		`

	dsConfig := dsb().Schema(schema).
		RootNode("Query", "heroByID").
		RootNode("Subscription", "heroByID").
		RootNode("Subscription", "heroByIDs").
		RootNode("Subscription", "heroByIDInput").
		RootNode("Subscription", "heroByIDMultipleArgs").
		ChildNode("Hero", "id", "name").
		DS()

	t.Run("subscription with IN field filter", test(
		schema, `
				subscription { heroByID(id: "1") { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					In: &resolve.SubscriptionFieldFilter{
						FieldPath: []string{"id"},
						Values: []resolve.InputTemplate{
							{
								Segments: []resolve.TemplateSegment{
									{
										SegmentType:        resolve.VariableSegmentType,
										VariableKind:       resolve.ContextVariableKind,
										VariableSourcePath: []string{"a"},
										Renderer:           resolve.NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByID"),
								Value: &resolve.Object{
									Path:          []string{"heroByID"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByID",
					Path:      []string{"heroByID"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"id"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						In: &SubscriptionFieldCondition{
							FieldPath: []string{"id"},
							Values:    []string{"{{ args.id }}"},
						},
					},
				},
			},
		},
	))

	t.Run("subscription with nested IN field filter", test(
		schema, `
				subscription { heroByIDInput(input: { one: 1, two: true }) { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					In: &resolve.SubscriptionFieldFilter{
						FieldPath: []string{"input", "one"},
						Values: []resolve.InputTemplate{
							{
								Segments: []resolve.TemplateSegment{
									{
										SegmentType:        resolve.VariableSegmentType,
										VariableKind:       resolve.ContextVariableKind,
										VariableSourcePath: []string{"a", "one"},
										Renderer:           resolve.NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByIDInput"),
								Value: &resolve.Object{
									Path:          []string{"heroByIDInput"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByIDInput",
					Path:      []string{"heroByIDInput"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "input",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"input"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						In: &SubscriptionFieldCondition{
							FieldPath: []string{"input", "one"},
							Values:    []string{"{{ args.input.one }}"},
						},
					},
				},
			},
		},
	))

	t.Run("subscription with IN field invalid filter multiple templates", test(
		schema, `
				subscription { heroByID(id: "1") { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByID"),
								Value: &resolve.Object{
									Path:          []string{"heroByID"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByID",
					Path:      []string{"heroByID"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"id"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						In: &SubscriptionFieldCondition{
							FieldPath: []string{"id"},
							Values:    []string{"{{ args.a }}.{{ args.b }}"},
						},
					},
				},
			},
		},
	))

	t.Run("subscription with IN field filter with prefix", test(
		schema, `
				subscription { heroByID(id: "1") { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					In: &resolve.SubscriptionFieldFilter{
						FieldPath: []string{"id"},
						Values: []resolve.InputTemplate{
							{
								Segments: []resolve.TemplateSegment{
									{
										SegmentType: resolve.StaticSegmentType,
										Data:        []byte("prefix."),
									},
									{
										SegmentType:        resolve.VariableSegmentType,
										VariableKind:       resolve.ContextVariableKind,
										VariableSourcePath: []string{"a"},
										Renderer:           resolve.NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByID"),
								Value: &resolve.Object{
									Path:          []string{"heroByID"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByID",
					Path:      []string{"heroByID"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"id"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						In: &SubscriptionFieldCondition{
							FieldPath: []string{"id"},
							Values:    []string{"prefix.{{ args.id }}"},
						},
					},
				},
			},
		},
	))

	t.Run("subscription with IN field filter with suffix", test(
		schema, `
				subscription { heroByID(id: "1") { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					In: &resolve.SubscriptionFieldFilter{
						FieldPath: []string{"id"},
						Values: []resolve.InputTemplate{
							{
								Segments: []resolve.TemplateSegment{
									{
										SegmentType:        resolve.VariableSegmentType,
										VariableKind:       resolve.ContextVariableKind,
										VariableSourcePath: []string{"a"},
										Renderer:           resolve.NewPlainVariableRenderer(),
									},
									{
										SegmentType: resolve.StaticSegmentType,
										Data:        []byte(".suffix"),
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByID"),
								Value: &resolve.Object{
									Path:          []string{"heroByID"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByID",
					Path:      []string{"heroByID"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"id"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						In: &SubscriptionFieldCondition{
							FieldPath: []string{"id"},
							Values:    []string{"{{ args.id }}.suffix"},
						},
					},
				},
			},
		},
	))

	t.Run("subscription with IN field filter with prefix and suffix", test(
		schema, `
				subscription { heroByID(id: "1") { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					In: &resolve.SubscriptionFieldFilter{
						FieldPath: []string{"id"},
						Values: []resolve.InputTemplate{
							{
								Segments: []resolve.TemplateSegment{
									{
										SegmentType: resolve.StaticSegmentType,
										Data:        []byte("prefix."),
									},
									{
										SegmentType:        resolve.VariableSegmentType,
										VariableKind:       resolve.ContextVariableKind,
										VariableSourcePath: []string{"a"},
										Renderer:           resolve.NewPlainVariableRenderer(),
									},
									{
										SegmentType: resolve.StaticSegmentType,
										Data:        []byte(".suffix"),
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByID"),
								Value: &resolve.Object{
									Path:          []string{"heroByID"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByID",
					Path:      []string{"heroByID"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"id"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						In: &SubscriptionFieldCondition{
							FieldPath: []string{"id"},
							Values:    []string{"prefix.{{ args.id }}.suffix"},
						},
					},
				},
			},
		},
	))

	t.Run("subscription with AND field filter", test(
		schema, `
				subscription { heroByIDInput(input: { one: 3, two: false }) { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					And: []resolve.SubscriptionFilter{
						{
							In: &resolve.SubscriptionFieldFilter{
								FieldPath: []string{"input", "one"},
								Values: []resolve.InputTemplate{
									{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType:        resolve.VariableSegmentType,
												VariableKind:       resolve.ContextVariableKind,
												VariableSourcePath: []string{"a", "one"},
												Renderer:           resolve.NewPlainVariableRenderer(),
											},
										},
									},
								},
							},
						},
						{
							In: &resolve.SubscriptionFieldFilter{
								FieldPath: []string{"input", "two"},
								Values: []resolve.InputTemplate{
									{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType:        resolve.VariableSegmentType,
												VariableKind:       resolve.ContextVariableKind,
												VariableSourcePath: []string{"a", "two"},
												Renderer:           resolve.NewPlainVariableRenderer(),
											},
										},
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByIDInput"),
								Value: &resolve.Object{
									Path:          []string{"heroByIDInput"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByIDInput",
					Path:      []string{"heroByIDInput"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "input",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"input"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						And: []SubscriptionFilterCondition{
							{
								In: &SubscriptionFieldCondition{
									FieldPath: []string{"input", "one"},
									Values:    []string{"{{ args.input.one }}"},
								},
							},
							{
								In: &SubscriptionFieldCondition{
									FieldPath: []string{"input", "two"},
									Values:    []string{"{{ args.input.two }}"},
								},
							},
						},
					},
				},
			},
		},
	))

	t.Run("subscription with OR field filter", test(
		schema, `
				subscription { heroByIDMultipleArgs(one: 10, two: 5.8) { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					Or: []resolve.SubscriptionFilter{
						{
							In: &resolve.SubscriptionFieldFilter{
								FieldPath: []string{"one"},
								Values: []resolve.InputTemplate{
									{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType:        resolve.VariableSegmentType,
												VariableKind:       resolve.ContextVariableKind,
												VariableSourcePath: []string{"a"},
												Renderer:           resolve.NewPlainVariableRenderer(),
											},
										},
									},
								},
							},
						},
						{
							In: &resolve.SubscriptionFieldFilter{
								FieldPath: []string{"two"},
								Values: []resolve.InputTemplate{
									{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType:        resolve.VariableSegmentType,
												VariableKind:       resolve.ContextVariableKind,
												VariableSourcePath: []string{"b"},
												Renderer:           resolve.NewPlainVariableRenderer(),
											},
										},
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByIDMultipleArgs"),
								Value: &resolve.Object{
									Path:          []string{"heroByIDMultipleArgs"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByIDMultipleArgs",
					Path:      []string{"heroByIDMultipleArgs"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "one",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"one"},
						},
						{
							Name:       "two",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"two"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						Or: []SubscriptionFilterCondition{
							{
								In: &SubscriptionFieldCondition{
									FieldPath: []string{"one"},
									Values:    []string{"{{ args.one }}"},
								},
							},
							{
								In: &SubscriptionFieldCondition{
									FieldPath: []string{"two"},
									Values:    []string{"{{ args.two }}"},
								},
							},
						},
					},
				},
			},
		},
	))

	t.Run("subscription with NOT OR field filter", test(
		schema, `
				subscription { heroByIDMultipleArgs(one: 42, two: 3.7) { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					Not: &resolve.SubscriptionFilter{
						Or: []resolve.SubscriptionFilter{
							{
								In: &resolve.SubscriptionFieldFilter{
									FieldPath: []string{"one"},
									Values: []resolve.InputTemplate{
										{
											Segments: []resolve.TemplateSegment{
												{
													SegmentType:        resolve.VariableSegmentType,
													VariableKind:       resolve.ContextVariableKind,
													VariableSourcePath: []string{"a"},
													Renderer:           resolve.NewPlainVariableRenderer(),
												},
											},
										},
									},
								},
							},
							{
								In: &resolve.SubscriptionFieldFilter{
									FieldPath: []string{"two"},
									Values: []resolve.InputTemplate{
										{
											Segments: []resolve.TemplateSegment{
												{
													SegmentType:        resolve.VariableSegmentType,
													VariableKind:       resolve.ContextVariableKind,
													VariableSourcePath: []string{"b"},
													Renderer:           resolve.NewPlainVariableRenderer(),
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByIDMultipleArgs"),
								Value: &resolve.Object{
									Path:          []string{"heroByIDMultipleArgs"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByIDMultipleArgs",
					Path:      []string{"heroByIDMultipleArgs"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "one",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"one"},
						},
						{
							Name:       "two",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"two"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						Not: &SubscriptionFilterCondition{
							Or: []SubscriptionFilterCondition{
								{
									In: &SubscriptionFieldCondition{
										FieldPath: []string{"one"},
										Values:    []string{"{{ args.one }}"},
									},
								},
								{
									In: &SubscriptionFieldCondition{
										FieldPath: []string{"two"},
										Values:    []string{"{{ args.two }}"},
									},
								},
							},
						},
					},
				},
			},
		},
	))
	t.Run("subscription with IN condition filter and list argument", test(
		schema, `
				subscription { heroByIDs(ids: ["1", "3", "5"]) { id name } }
			`, "",
		&SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte{},
				},
				Filter: &resolve.SubscriptionFilter{
					In: &resolve.SubscriptionFieldFilter{
						FieldPath: []string{"id"},
						Values: []resolve.InputTemplate{
							{
								Segments: []resolve.TemplateSegment{
									{
										SegmentType:        resolve.VariableSegmentType,
										VariableKind:       resolve.ContextVariableKind,
										VariableSourcePath: []string{"a"},
										Renderer:           resolve.NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByIDs"),
								Value: &resolve.Object{
									Path:          []string{"heroByIDs"},
									Nullable:      true,
									TypeName:      "Hero",
									PossibleTypes: map[string]struct{}{"Hero": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.Scalar{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path: []string{"name"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{dsConfig},
			Fields: []FieldConfiguration{
				{
					TypeName:  "Subscription",
					FieldName: "heroByIDs",
					Path:      []string{"heroByIDs"},
					Arguments: []ArgumentConfiguration{
						{
							Name:       "ids",
							SourceType: FieldArgumentSource,
							SourcePath: []string{"ids"},
						},
					},
					SubscriptionFilterCondition: &SubscriptionFilterCondition{
						In: &SubscriptionFieldCondition{
							FieldPath: []string{"id"},
							Values:    []string{"{{ args.ids }}"},
						},
					},
				},
			},
		},
	))
}
