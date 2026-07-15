package graphql_datasource

import (
	"testing"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func multiFetchDefinition() string {
	return `
		type Query {
			employees: [Employee]
			employee: Employee
		}
		type Employee {
			id: ID!
			products: [String]
			notes: String
		}`
}

func multiFetchPlanConfig(t *testing.T, enableMultiFetch bool) plan.Configuration {
	accountsSDL := `
		type Query {
			employees: [Employee]
			employee: Employee
		}
		type Employee @key(fields: "id") {
			id: ID!
		}`
	productsSDL := `
		type Employee @key(fields: "id") {
			id: ID!
			products: [String]
			notes: String
		}`

	accounts := mustDataSourceConfiguration(t, "accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"employees", "employee"}},
				{TypeName: "Employee", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Employee", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://accounts"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: accountsSDL}, accountsSDL),
		}))

	products := mustDataSourceConfiguration(t, "products",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{{TypeName: "Employee", FieldNames: []string{"id", "products", "notes"}}},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Employee", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://products"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: productsSDL}, productsSDL),
		}))

	return plan.Configuration{
		DataSources:                  []plan.DataSource{accounts, products},
		DisableResolveFieldPositions: true,
		EnableMultiFetch:             enableMultiFetch,
	}
}

func multiFetchResponseData() *resolve.Object {
	return &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name: []byte("employees"),
				Value: &resolve.Array{
					Path:     []string{"employees"},
					Nullable: true,
					Item: &resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name:  []byte("id"),
								Value: &resolve.Scalar{Path: []string{"id"}},
								Info: &resolve.FieldInfo{
									Name:                "id",
									ExactParentTypeName: "Employee",
									ParentTypeNames:     []string{"Employee"},
									NamedType:           "ID",
									Source:              resolve.TypeFieldSource{IDs: []string{"accounts"}, Names: []string{"accounts"}},
								},
							},
							{
								Name: []byte("products"),
								Value: &resolve.Array{
									Path:     []string{"products"},
									Nullable: true,
									Item:     &resolve.String{Nullable: true},
									SkipItem: func(ctx *resolve.Context, value *astjson.Value) bool { return false },
								},
								Info: &resolve.FieldInfo{
									Name:                "products",
									ExactParentTypeName: "Employee",
									ParentTypeNames:     []string{"Employee"},
									NamedType:           "String",
									Source:              resolve.TypeFieldSource{IDs: []string{"products"}, Names: []string{"products"}},
								},
							},
						},
						PossibleTypes: map[string]struct{}{"Employee": {}},
						SourceName:    "accounts",
						TypeName:      "Employee",
					},
					SkipItem: func(ctx *resolve.Context, value *astjson.Value) bool { return false },
				},
				Info: &resolve.FieldInfo{
					Name:                "employees",
					ExactParentTypeName: "Query",
					ParentTypeNames:     []string{"Query"},
					NamedType:           "Employee",
					Source:              resolve.TypeFieldSource{IDs: []string{"accounts"}, Names: []string{"accounts"}},
				},
			},
			{
				Name: []byte("employee"),
				Value: &resolve.Object{
					Nullable: true,
					Path:     []string{"employee"},
					Fields: []*resolve.Field{
						{
							Name:  []byte("id"),
							Value: &resolve.Scalar{Path: []string{"id"}},
							Info: &resolve.FieldInfo{
								Name:                "id",
								ExactParentTypeName: "Employee",
								ParentTypeNames:     []string{"Employee"},
								NamedType:           "ID",
								Source:              resolve.TypeFieldSource{IDs: []string{"accounts"}, Names: []string{"accounts"}},
							},
						},
						{
							Name:  []byte("notes"),
							Value: &resolve.String{Path: []string{"notes"}, Nullable: true},
							Info: &resolve.FieldInfo{
								Name:                "notes",
								ExactParentTypeName: "Employee",
								ParentTypeNames:     []string{"Employee"},
								NamedType:           "String",
								Source:              resolve.TypeFieldSource{IDs: []string{"products"}, Names: []string{"products"}},
							},
						},
					},
					PossibleTypes: map[string]struct{}{"Employee": {}},
					SourceName:    "accounts",
					TypeName:      "Employee",
				},
				Info: &resolve.FieldInfo{
					Name:                "employee",
					ExactParentTypeName: "Query",
					ParentTypeNames:     []string{"Query"},
					NamedType:           "Employee",
					Source:              resolve.TypeFieldSource{IDs: []string{"accounts"}, Names: []string{"accounts"}},
				},
			},
		},
	}
}

func multiFetchRepresentationsRenderer() *resolve.GraphQLVariableResolveRenderer {
	return resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
		Nullable: true,
		Fields: []*resolve.Field{
			{
				Name:        []byte("__typename"),
				Value:       &resolve.String{Path: []string{"__typename"}},
				OnTypeNames: [][]byte{[]byte("Employee")},
			},
			{
				Name:        []byte("id"),
				Value:       &resolve.Scalar{Path: []string{"id"}},
				OnTypeNames: [][]byte{[]byte("Employee")},
			},
		},
	})
}

func multiFetchRootFetch() *resolve.FetchTreeNode {
	return &resolve.FetchTreeNode{
		Kind: resolve.FetchTreeNodeKindSingle,
		Item: &resolve.FetchItem{
			Fetch: &resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					DataSource: &Source{},
					PostProcessing: resolve.PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				InputTemplate: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							SegmentType: resolve.StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"http://accounts","body":{"query":"{employees {id __typename} employee {id __typename}}"}}`),
						},
					},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
				Info: &resolve.FetchInfo{
					DataSourceID:   "accounts",
					DataSourceName: "accounts",
					RootFields: []resolve.GraphCoordinate{
						{TypeName: "Query", FieldName: "employees"},
						{TypeName: "Query", FieldName: "employee"},
					},
					OperationType: ast.OperationTypeQuery,
				},
			},
		},
	}
}

func TestGraphQLDataSourceFederation_MultiFetch(t *testing.T) {
	operation := `{ employees { id products } employee { id notes } }`

	t.Run("two same-wave entity fetches merge", RunTest(
		multiFetchDefinition(), operation, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: multiFetchResponseData(),
				Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: resolve.Sequence(
					multiFetchRootFetch(),
					&resolve.FetchTreeNode{
						Kind: resolve.FetchTreeNodeKindSingle,
						Item: &resolve.FetchItem{
							Fetch: &resolve.MultiEntityFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID:           1,
									DependsOnFetchIDs: []int{0},
								},
								Input: resolve.MultiEntityInput{
									Header: resolve.InputTemplate{
										Segments: []resolve.TemplateSegment{
											{
												SegmentType: resolve.StaticSegmentType,
												Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations_f1: [_Any!]!, $includeF1: Boolean!, $representations_f2: [_Any!]!, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes}}}","variables":{`),
											},
										},
									},
									Entries: []resolve.MultiEntityFetchEntry{
										{
											Alias: "f1",
											Item: &resolve.FetchItem{
												FetchPath: []resolve.FetchItemPathElement{
													{Kind: resolve.FetchItemPathElementKindArray, Path: []string{"employees"}},
												},
												ResponsePath:         "employees",
												ResponsePathElements: []string{"employees"},
											},
											Info: &resolve.FetchInfo{
												DataSourceID:   "products",
												DataSourceName: "products",
												RootFields:     []resolve.GraphCoordinate{{TypeName: "Employee", FieldName: "products"}},
												OperationType:  ast.OperationTypeQuery,
											},
											PostProcessing: resolve.PostProcessingConfiguration{
												SelectResponseDataPath:   []string{"data", "f1"},
												SelectResponseErrorsPath: []string{"errors"},
											},
											OriginKind:            resolve.EntityFetchOriginBatch,
											RepresentationsPrefix: []byte(`"representations_f1":[`),
											Representations: resolve.InputTemplate{
												Segments: []resolve.TemplateSegment{
													{
														SegmentType:  resolve.VariableSegmentType,
														VariableKind: resolve.ResolvableObjectVariableKind,
														Renderer:     multiFetchRepresentationsRenderer(),
													},
												},
												SetTemplateOutputToNullOnVariableNull: true,
											},
											IncludePrefix:        []byte(`],"includeF1":`),
											SkipNullItems:        true,
											SkipEmptyObjectItems: true,
											SkipErrItems:         true,
										},
										{
											Alias: "f2",
											Item: &resolve.FetchItem{
												FetchPath: []resolve.FetchItemPathElement{
													{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"employee"}},
												},
												ResponsePath:         "employee",
												ResponsePathElements: []string{"employee"},
											},
											Info: &resolve.FetchInfo{
												DataSourceID:   "products",
												DataSourceName: "products",
												RootFields:     []resolve.GraphCoordinate{{TypeName: "Employee", FieldName: "notes"}},
												OperationType:  ast.OperationTypeQuery,
											},
											PostProcessing: resolve.PostProcessingConfiguration{
												SelectResponseDataPath:   []string{"data", "f2"},
												SelectResponseErrorsPath: []string{"errors"},
											},
											OriginKind:            resolve.EntityFetchOriginSingle,
											RepresentationsPrefix: []byte(`,"representations_f2":[`),
											Representations: resolve.InputTemplate{
												Segments: []resolve.TemplateSegment{
													{
														SegmentType:  resolve.VariableSegmentType,
														VariableKind: resolve.ResolvableObjectVariableKind,
														Renderer:     multiFetchRepresentationsRenderer(),
													},
												},
												SetTemplateOutputToNullOnVariableNull: true,
											},
											IncludePrefix:        []byte(`],"includeF2":`),
											SkipNullItems:        true,
											SkipEmptyObjectItems: true,
											SkipErrItems:         true,
										},
									},
									Footer: resolve.InputTemplate{
										Segments: []resolve.TemplateSegment{
											{SegmentType: resolve.StaticSegmentType, Data: []byte(`}}}`)},
										},
									},
								},
								DataSource:           &Source{},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								MergedFetchIDs:       []int{1, 2},
								Info: &resolve.FetchInfo{
									DataSourceID:   "products",
									DataSourceName: "products",
									RootFields: []resolve.GraphCoordinate{
										{TypeName: "Employee", FieldName: "products"},
										{TypeName: "Employee", FieldName: "notes"},
									},
									OperationType: ast.OperationTypeQuery,
								},
							},
						},
					},
				),
			},
		},
		multiFetchPlanConfig(t, true),
		WithFieldInfo(),
		WithDefaultCustomPostProcessor(postprocess.EnableMultiFetch()),
	))

	t.Run("flag off keeps two separate entity fetches", RunTest(
		multiFetchDefinition(), operation, "",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: multiFetchResponseData(),
				Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: resolve.Sequence(
					multiFetchRootFetch(),
					resolve.Parallel(
						&resolve.FetchTreeNode{
							Kind: resolve.FetchTreeNodeKindSingle,
							Item: &resolve.FetchItem{
								Fetch: &resolve.BatchEntityFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									Input: resolve.BatchInput{
										Header: resolve.InputTemplate{
											Segments: []resolve.TemplateSegment{
												{
													SegmentType: resolve.StaticSegmentType,
													Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename products}}}","variables":{"representations":[`),
												},
											},
											SetTemplateOutputToNullOnVariableNull: true,
										},
										Items: []resolve.InputTemplate{
											{
												Segments: []resolve.TemplateSegment{
													{
														SegmentType:  resolve.VariableSegmentType,
														VariableKind: resolve.ResolvableObjectVariableKind,
														Renderer:     multiFetchRepresentationsRenderer(),
													},
												},
												SetTemplateOutputToNullOnVariableNull: true,
											},
										},
										SkipNullItems:        true,
										SkipEmptyObjectItems: true,
										SkipErrItems:         true,
										Separator: resolve.InputTemplate{
											Segments: []resolve.TemplateSegment{
												{SegmentType: resolve.StaticSegmentType, Data: []byte(`,`)},
											},
										},
										Footer: resolve.InputTemplate{
											Segments: []resolve.TemplateSegment{
												{SegmentType: resolve.StaticSegmentType, Data: []byte(`]}}}`)},
											},
											SetTemplateOutputToNullOnVariableNull: true,
										},
									},
									DataSource: &Source{},
									PostProcessing: resolve.PostProcessingConfiguration{
										SelectResponseDataPath:   []string{"data", "_entities"},
										SelectResponseErrorsPath: []string{"errors"},
									},
									Info: &resolve.FetchInfo{
										DataSourceID:   "products",
										DataSourceName: "products",
										RootFields:     []resolve.GraphCoordinate{{TypeName: "Employee", FieldName: "products"}},
										OperationType:  ast.OperationTypeQuery,
									},
								},
								FetchPath: []resolve.FetchItemPathElement{
									{Kind: resolve.FetchItemPathElementKindArray, Path: []string{"employees"}},
								},
								ResponsePath:         "employees",
								ResponsePathElements: []string{"employees"},
							},
						},
						&resolve.FetchTreeNode{
							Kind: resolve.FetchTreeNodeKindSingle,
							Item: &resolve.FetchItem{
								Fetch: &resolve.EntityFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           2,
										DependsOnFetchIDs: []int{0},
									},
									Input: resolve.EntityInput{
										Header: resolve.InputTemplate{
											Segments: []resolve.TemplateSegment{
												{
													SegmentType: resolve.StaticSegmentType,
													Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename notes}}}","variables":{"representations":[`),
												},
											},
											SetTemplateOutputToNullOnVariableNull: true,
										},
										Item: resolve.InputTemplate{
											Segments: []resolve.TemplateSegment{
												{
													SegmentType:  resolve.VariableSegmentType,
													VariableKind: resolve.ResolvableObjectVariableKind,
													Renderer:     multiFetchRepresentationsRenderer(),
												},
											},
											SetTemplateOutputToNullOnVariableNull: true,
										},
										SkipErrItem: true,
										Footer: resolve.InputTemplate{
											Segments: []resolve.TemplateSegment{
												{SegmentType: resolve.StaticSegmentType, Data: []byte(`]}}}`)},
											},
											SetTemplateOutputToNullOnVariableNull: true,
										},
									},
									DataSource: &Source{},
									PostProcessing: resolve.PostProcessingConfiguration{
										SelectResponseDataPath:   []string{"data", "_entities", "0"},
										SelectResponseErrorsPath: []string{"errors"},
									},
									Info: &resolve.FetchInfo{
										DataSourceID:   "products",
										DataSourceName: "products",
										RootFields:     []resolve.GraphCoordinate{{TypeName: "Employee", FieldName: "notes"}},
										OperationType:  ast.OperationTypeQuery,
									},
								},
								FetchPath: []resolve.FetchItemPathElement{
									{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"employee"}},
								},
								ResponsePath:         "employee",
								ResponsePathElements: []string{"employee"},
							},
						},
					),
				),
			},
		},
		multiFetchPlanConfig(t, false),
		WithFieldInfo(),
		WithDefaultCustomPostProcessor(),
	))
}
