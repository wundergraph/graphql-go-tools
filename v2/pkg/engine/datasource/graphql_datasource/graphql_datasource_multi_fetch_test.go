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
		WithPostProcessor(postprocess.EnableMultiFetch()),
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
		// Full default pipeline with only the scheduler replaced by the legacy
		// organizers — the fixture pins rendered inputs and legacy wave shapes.
		WithPostProcessor(),
	))
}

func TestGraphQLDataSourceFederation_MultiFetch_ThreeFetchGroup(t *testing.T) {
	definition := `
		type Query {
			employees: [Employee]
			employee: Employee
			contractors: [Employee]
		}
		type Employee {
			id: ID!
			products: [String]
			notes: String
		}`
	accountsSDL := `
		type Query {
			employees: [Employee]
			employee: Employee
			contractors: [Employee]
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
				{TypeName: "Query", FieldNames: []string{"employees", "employee", "contractors"}},
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

	config := plan.Configuration{
		DataSources:                  []plan.DataSource{accounts, products},
		DisableResolveFieldPositions: true,
		EnableMultiFetch:             true,
	}

	employeesListField := func(name string) *resolve.Field {
		return &resolve.Field{
			Name: []byte(name),
			Value: &resolve.Array{
				Path:     []string{name},
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
				Name:                name,
				ExactParentTypeName: "Query",
				ParentTypeNames:     []string{"Query"},
				NamedType:           "Employee",
				Source:              resolve.TypeFieldSource{IDs: []string{"accounts"}, Names: []string{"accounts"}},
			},
		}
	}
	listEntry := func(alias, path, prefix string) resolve.MultiEntityFetchEntry {
		return resolve.MultiEntityFetchEntry{
			Alias: alias,
			Item: &resolve.FetchItem{
				FetchPath: []resolve.FetchItemPathElement{
					{Kind: resolve.FetchItemPathElementKindArray, Path: []string{path}},
				},
				ResponsePath:         path,
				ResponsePathElements: []string{path},
			},
			Info: &resolve.FetchInfo{
				DataSourceID:   "products",
				DataSourceName: "products",
				RootFields:     []resolve.GraphCoordinate{{TypeName: "Employee", FieldName: "products"}},
				OperationType:  ast.OperationTypeQuery,
			},
			PostProcessing: resolve.PostProcessingConfiguration{
				SelectResponseDataPath:   []string{"data", alias},
				SelectResponseErrorsPath: []string{"errors"},
			},
			OriginKind:            resolve.EntityFetchOriginBatch,
			RepresentationsPrefix: []byte(prefix),
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
			IncludePrefix:        []byte(`],"includeF` + alias[1:] + `":`),
			SkipNullItems:        true,
			SkipEmptyObjectItems: true,
			SkipErrItems:         true,
		}
	}
	f1 := listEntry("f1", "employees", `"representations_f1":[`)
	f3 := listEntry("f3", "contractors", `,"representations_f3":[`)

	t.Run("three same-wave entity fetches merge", RunTest(
		definition,
		`{ employees { id products } employee { id notes } contractors { id products } }`,
		"",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						employeesListField("employees"),
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
						employeesListField("contractors"),
					},
				},
				Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: resolve.Sequence(
					&resolve.FetchTreeNode{
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
											Data:        []byte(`{"method":"POST","url":"http://accounts","body":{"query":"{employees {id __typename} employee {id __typename} contractors {id __typename}}"}}`),
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
										{TypeName: "Query", FieldName: "contractors"},
									},
									OperationType: ast.OperationTypeQuery,
								},
							},
						},
					},
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
												Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations_f1: [_Any!]!, $includeF1: Boolean!, $representations_f2: [_Any!]!, $includeF2: Boolean!, $representations_f3: [_Any!]!, $includeF3: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes}} f3: _entities(representations: $representations_f3)@include(if: $includeF3) {... on Employee {__typename products}}}","variables":{`),
											},
										},
									},
									Entries: []resolve.MultiEntityFetchEntry{
										f1,
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
										f3,
									},
									Footer: resolve.InputTemplate{
										Segments: []resolve.TemplateSegment{
											{SegmentType: resolve.StaticSegmentType, Data: []byte(`}}}`)},
										},
									},
								},
								DataSource:           &Source{},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								MergedFetchIDs:       []int{1, 2, 3},
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
		config,
		WithFieldInfo(),
		WithPostProcessor(postprocess.EnableMultiFetch()),
	))
}

func multiFetchArgDefinition() string {
	return `
		type Query {
			employees: [Employee]
			employee: Employee
		}
		type Employee {
			id: ID!
			products(first: Int): [String]
			notes: String
		}`
}

func multiFetchArgPlanConfig(t *testing.T) plan.Configuration {
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
			products(first: Int): [String]
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
		EnableMultiFetch:             true,
		Fields: plan.FieldConfigurations{
			{
				TypeName:  "Employee",
				FieldName: "products",
				Arguments: plan.ArgumentsConfigurations{
					{Name: "first", SourceType: plan.FieldArgumentSource},
				},
			},
		},
	}
}

// multiFetchEmployeeProductsData is the response object for
// `{ employees { id products(...) } employee { id products(...) } }`.
func multiFetchEmployeeProductsData() *resolve.Object {
	employeeProducts := func(path string) *resolve.Field {
		return &resolve.Field{
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
		}
	}
	idField := &resolve.Field{
		Name:  []byte("id"),
		Value: &resolve.Scalar{Path: []string{"id"}},
		Info: &resolve.FieldInfo{
			Name:                "id",
			ExactParentTypeName: "Employee",
			ParentTypeNames:     []string{"Employee"},
			NamedType:           "ID",
			Source:              resolve.TypeFieldSource{IDs: []string{"accounts"}, Names: []string{"accounts"}},
		},
	}
	return &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name: []byte("employees"),
				Value: &resolve.Array{
					Path:     []string{"employees"},
					Nullable: true,
					Item: &resolve.Object{
						Nullable:      true,
						Fields:        []*resolve.Field{idField, employeeProducts("employees")},
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
					Nullable:      true,
					Path:          []string{"employee"},
					Fields:        []*resolve.Field{idField, employeeProducts("employee")},
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

// firstArgVariable is the recorded non-representations variable `first`, renamed
// per member to `first_f<k>`, rendered as a context variable template.
func firstArgVariable(alias string) resolve.MultiEntityFetchVariable {
	return resolve.MultiEntityFetchVariable{
		KeyPrefix: []byte(`,"first_` + alias + `":`),
		Value: resolve.InputTemplate{
			Segments: []resolve.TemplateSegment{
				{SegmentType: resolve.StaticSegmentType},
				{
					SegmentType:        resolve.VariableSegmentType,
					VariableKind:       resolve.ContextVariableKind,
					VariableSourcePath: []string{"first"},
					Renderer:           resolve.NewJSONVariableRenderer(),
				},
				{SegmentType: resolve.StaticSegmentType},
			},
		},
	}
}

func multiFetchProductsEntry(alias, kind, path string, first bool) resolve.MultiEntityFetchEntry {
	var fetchPath []resolve.FetchItemPathElement
	if kind == "array" {
		fetchPath = []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindArray, Path: []string{path}}}
	} else {
		fetchPath = []resolve.FetchItemPathElement{{Kind: resolve.FetchItemPathElementKindObject, Path: []string{path}}}
	}
	originKind := resolve.EntityFetchOriginBatch
	if kind == "object" {
		originKind = resolve.EntityFetchOriginSingle
	}
	repPrefix := `"representations_` + alias + `":[`
	if alias != "f1" {
		repPrefix = "," + repPrefix
	}
	entry := resolve.MultiEntityFetchEntry{
		Alias: alias,
		Item: &resolve.FetchItem{
			FetchPath:            fetchPath,
			ResponsePath:         path,
			ResponsePathElements: []string{path},
		},
		Info: &resolve.FetchInfo{
			DataSourceID:   "products",
			DataSourceName: "products",
			RootFields:     []resolve.GraphCoordinate{{TypeName: "Employee", FieldName: "products"}},
			OperationType:  ast.OperationTypeQuery,
		},
		PostProcessing: resolve.PostProcessingConfiguration{
			SelectResponseDataPath:   []string{"data", alias},
			SelectResponseErrorsPath: []string{"errors"},
		},
		OriginKind:            originKind,
		RepresentationsPrefix: []byte(repPrefix),
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
		IncludePrefix:        []byte(`],"include` + "F" + alias[1:] + `":`),
		SkipNullItems:        true,
		SkipEmptyObjectItems: true,
		SkipErrItems:         true,
	}
	if first {
		entry.Variables = []resolve.MultiEntityFetchVariable{firstArgVariable(alias)}
	}
	return entry
}

func TestGraphQLDataSourceFederation_MultiFetch_AdditionalVariables(t *testing.T) {
	t.Run("client variable through one member", RunTest(
		multiFetchArgDefinition(),
		`query($first: Int){ employees { id products(first: $first) } employee { id notes } }`,
		"",
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
												Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $first_f1)}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes}}}","variables":{`),
											},
										},
									},
									Entries: []resolve.MultiEntityFetchEntry{
										multiFetchProductsEntry("f1", "array", "employees", true),
										multiFetchNotesEntry(),
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
		multiFetchArgPlanConfig(t),
		WithFieldInfo(),
		WithPostProcessor(postprocess.EnableMultiFetch()),
	))

	t.Run("same client variable through both members", RunTest(
		multiFetchArgDefinition(),
		`query($first: Int){ employees { id products(first: $first) } employee { id products(first: $first) } }`,
		"",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: multiFetchEmployeeProductsData(),
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
												Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $first_f1)}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename products(first: $first_f2)}}}","variables":{`),
											},
										},
									},
									Entries: []resolve.MultiEntityFetchEntry{
										multiFetchProductsEntry("f1", "array", "employees", true),
										multiFetchProductsEntry("f2", "object", "employee", true),
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
									// Duplicate root fields (both members resolve
									// Employee.products) are deduplicated in the merged info.
									RootFields: []resolve.GraphCoordinate{
										{TypeName: "Employee", FieldName: "products"},
									},
									OperationType: ast.OperationTypeQuery,
								},
							},
						},
					},
				),
			},
		},
		multiFetchArgPlanConfig(t),
		WithFieldInfo(),
		WithPostProcessor(postprocess.EnableMultiFetch()),
	))

	// A default-valued client variable ($first: Int = 10) merges without error.
	// The default is applied during client-operation normalization, so the
	// context variable always sends a concrete value; the base federation
	// planner deliberately does NOT propagate the default into the subgraph
	// operation (it imports only the argument type — see
	// configureFieldArgumentSource / AddVariableDefinitionToOperationDefinition
	// in graphql_datasource.go). The rename-import preserves whatever the base
	// operation had, so the merged definition is `$first_f1: Int` (no default),
	// identical to the non-merged path — a transport-transparent merge.
	t.Run("client variable with default value", RunTest(
		multiFetchArgDefinition(),
		`query($first: Int = 10){ employees { id products(first: $first) } employee { id notes } }`,
		"",
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
												Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $first_f1)}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes}}}","variables":{`),
											},
										},
									},
									Entries: []resolve.MultiEntityFetchEntry{
										multiFetchProductsEntry("f1", "array", "employees", true),
										multiFetchNotesEntry(),
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
		multiFetchArgPlanConfig(t),
		WithFieldInfo(),
		WithPostProcessor(postprocess.EnableMultiFetch()),
	))
}

// multiFetchNotesEntry is the employee->notes member (no extra variables).
func multiFetchNotesEntry() resolve.MultiEntityFetchEntry {
	return resolve.MultiEntityFetchEntry{
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
	}
}

func TestGraphQLDataSourceFederation_MultiFetch_WaveSeparation(t *testing.T) {
	// products is fetched twice: Employee.upc via key id in wave one, and
	// Manager.title via key mid in wave two (the manager key comes from the org
	// subgraph). Different waves must not merge, even with the flag on.
	accountsSDL := `
		type Query { employee: Employee }
		type Employee @key(fields: "id") { id: ID! }`
	orgSDL := `
		type Employee @key(fields: "id") { id: ID! manager: Manager }
		type Manager @key(fields: "mid") { mid: ID! }`
	productsSDL := `
		type Employee @key(fields: "id") { id: ID! upc: String }
		type Manager @key(fields: "mid") { mid: ID! title: String }`
	definition := `
		type Query { employee: Employee }
		type Employee { id: ID! upc: String manager: Manager }
		type Manager { mid: ID! title: String }`

	accounts := mustDataSourceConfiguration(t, "accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"employee"}},
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
	org := mustDataSourceConfiguration(t, "org",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Employee", FieldNames: []string{"id", "manager"}},
				{TypeName: "Manager", FieldNames: []string{"mid"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{
					{TypeName: "Employee", SelectionSet: "id"},
					{TypeName: "Manager", SelectionSet: "mid"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://org"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: orgSDL}, orgSDL),
		}))
	products := mustDataSourceConfiguration(t, "products",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Employee", FieldNames: []string{"id", "upc"}},
				{TypeName: "Manager", FieldNames: []string{"mid", "title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{
					{TypeName: "Employee", SelectionSet: "id"},
					{TypeName: "Manager", SelectionSet: "mid"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://products"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: productsSDL}, productsSDL),
		}))

	config := plan.Configuration{
		DataSources:                  []plan.DataSource{accounts, org, products},
		DisableResolveFieldPositions: true,
		EnableMultiFetch:             true,
	}

	managerRepresentationsRenderer := resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
		Nullable: true,
		Fields: []*resolve.Field{
			{
				Name:        []byte("__typename"),
				Value:       &resolve.String{Path: []string{"__typename"}},
				OnTypeNames: [][]byte{[]byte("Manager")},
			},
			{
				Name:        []byte("mid"),
				Value:       &resolve.Scalar{Path: []string{"mid"}},
				OnTypeNames: [][]byte{[]byte("Manager")},
			},
		},
	})
	entityFetch := func(id int, dependsOn int, url, query string, renderer *resolve.GraphQLVariableResolveRenderer, info *resolve.FetchInfo) *resolve.EntityFetch {
		return &resolve.EntityFetch{
			FetchDependencies: resolve.FetchDependencies{
				FetchID:           id,
				DependsOnFetchIDs: []int{dependsOn},
			},
			Input: resolve.EntityInput{
				Header: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							SegmentType: resolve.StaticSegmentType,
							Data:        []byte(`{"method":"POST","url":"` + url + `","body":{"query":"` + query + `","variables":{"representations":[`),
						},
					},
					SetTemplateOutputToNullOnVariableNull: true,
				},
				Item: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							SegmentType:  resolve.VariableSegmentType,
							VariableKind: resolve.ResolvableObjectVariableKind,
							Renderer:     renderer,
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
			Info: info,
		}
	}

	t.Run("different waves do not merge", RunTest(
		definition,
		`{ employee { upc manager { title } } }`,
		"",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("employee"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"employee"},
								Fields: []*resolve.Field{
									{
										Name:  []byte("upc"),
										Value: &resolve.String{Path: []string{"upc"}, Nullable: true},
										Info: &resolve.FieldInfo{
											Name:                "upc",
											ExactParentTypeName: "Employee",
											ParentTypeNames:     []string{"Employee"},
											NamedType:           "String",
											Source:              resolve.TypeFieldSource{IDs: []string{"products"}, Names: []string{"products"}},
										},
									},
									{
										Name: []byte("manager"),
										Value: &resolve.Object{
											Nullable: true,
											Path:     []string{"manager"},
											Fields: []*resolve.Field{
												{
													Name:  []byte("title"),
													Value: &resolve.String{Path: []string{"title"}, Nullable: true},
													Info: &resolve.FieldInfo{
														Name:                "title",
														ExactParentTypeName: "Manager",
														ParentTypeNames:     []string{"Manager"},
														NamedType:           "String",
														Source:              resolve.TypeFieldSource{IDs: []string{"products"}, Names: []string{"products"}},
													},
												},
											},
											PossibleTypes: map[string]struct{}{"Manager": {}},
											SourceName:    "org",
											TypeName:      "Manager",
										},
										Info: &resolve.FieldInfo{
											Name:                "manager",
											ExactParentTypeName: "Employee",
											ParentTypeNames:     []string{"Employee"},
											NamedType:           "Manager",
											Source:              resolve.TypeFieldSource{IDs: []string{"org"}, Names: []string{"org"}},
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
				},
				Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
				Fetches: resolve.Sequence(
					&resolve.FetchTreeNode{
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
											Data:        []byte(`{"method":"POST","url":"http://accounts","body":{"query":"{employee {__typename id}}"}}`),
										},
									},
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
								Info: &resolve.FetchInfo{
									DataSourceID:   "accounts",
									DataSourceName: "accounts",
									RootFields:     []resolve.GraphCoordinate{{TypeName: "Query", FieldName: "employee"}},
									OperationType:  ast.OperationTypeQuery,
								},
							},
						},
					},
					resolve.Parallel(
						&resolve.FetchTreeNode{
							Kind: resolve.FetchTreeNodeKindSingle,
							Item: &resolve.FetchItem{
								Fetch: entityFetch(1, 0, "http://products",
									`query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename upc}}}`,
									multiFetchRepresentationsRenderer(),
									&resolve.FetchInfo{
										DataSourceID:   "products",
										DataSourceName: "products",
										RootFields:     []resolve.GraphCoordinate{{TypeName: "Employee", FieldName: "upc"}},
										OperationType:  ast.OperationTypeQuery,
									}),
								FetchPath: []resolve.FetchItemPathElement{
									{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"employee"}},
								},
								ResponsePath:         "employee",
								ResponsePathElements: []string{"employee"},
							},
						},
						&resolve.FetchTreeNode{
							Kind: resolve.FetchTreeNodeKindSingle,
							Item: &resolve.FetchItem{
								Fetch: entityFetch(2, 0, "http://org",
									`query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename manager {__typename mid}}}}`,
									multiFetchRepresentationsRenderer(),
									&resolve.FetchInfo{
										DataSourceID:   "org",
										DataSourceName: "org",
										RootFields:     []resolve.GraphCoordinate{{TypeName: "Employee", FieldName: "manager"}},
										OperationType:  ast.OperationTypeQuery,
									}),
								FetchPath: []resolve.FetchItemPathElement{
									{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"employee"}},
								},
								ResponsePath:         "employee",
								ResponsePathElements: []string{"employee"},
							},
						},
					),
					&resolve.FetchTreeNode{
						Kind: resolve.FetchTreeNodeKindSingle,
						Item: &resolve.FetchItem{
							Fetch: entityFetch(3, 2, "http://products",
								`query($representations: [_Any!]!){_entities(representations: $representations){... on Manager {__typename title}}}`,
								managerRepresentationsRenderer,
								&resolve.FetchInfo{
									DataSourceID:   "products",
									DataSourceName: "products",
									RootFields:     []resolve.GraphCoordinate{{TypeName: "Manager", FieldName: "title"}},
									OperationType:  ast.OperationTypeQuery,
								}),
							FetchPath: []resolve.FetchItemPathElement{
								{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"employee"}},
								{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"manager"}},
							},
							ResponsePath:         "employee.manager",
							ResponsePathElements: []string{"employee", "manager"},
						},
					},
				),
			},
		},
		config,
		WithFieldInfo(),
		WithPostProcessor(postprocess.EnableMultiFetch()),
	))
}

func TestGraphQLDataSourceFederation_MultiFetch_Subscription(t *testing.T) {
	// The subscription trigger is not part of the response fetch tree; the two
	// products entity fetches merge because they share the in-tree hub fetch
	// that resolves the Update payload's employees/employee.
	accountsSDL := `
		type Query { _dummy: String }
		type Subscription { update: Update }
		type Update @key(fields: "id") { id: ID! }`
	hubSDL := `
		type Update @key(fields: "id") { id: ID! employees: [Employee] employee: Employee }
		type Employee @key(fields: "id") { id: ID! }`
	productsSDL := `
		type Employee @key(fields: "id") {
			id: ID!
			products: [String]
			notes: String
		}`
	definition := `
		type Query { _dummy: String }
		type Subscription { update: Update }
		type Update { id: ID! employees: [Employee] employee: Employee }
		type Employee { id: ID! products: [String] notes: String }`

	accounts := mustDataSourceConfiguration(t, "accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"_dummy"}},
				{TypeName: "Subscription", FieldNames: []string{"update"}},
				{TypeName: "Update", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{{TypeName: "Update", SelectionSet: "id"}},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://accounts"},
			Subscription:        &SubscriptionConfiguration{URL: "ws://accounts"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: accountsSDL}, accountsSDL),
		}))
	hub := mustDataSourceConfiguration(t, "hub",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Update", FieldNames: []string{"id", "employees", "employee"}},
				{TypeName: "Employee", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: []plan.FederationFieldConfiguration{
					{TypeName: "Update", SelectionSet: "id"},
					{TypeName: "Employee", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch:               &FetchConfiguration{URL: "http://hub"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: hubSDL}, hubSDL),
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

	config := plan.Configuration{
		DataSources:                  []plan.DataSource{accounts, hub, products},
		DisableResolveFieldPositions: true,
		EnableMultiFetch:             true,
	}

	updateRepresentationsRenderer := resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
		Nullable: true,
		Fields: []*resolve.Field{
			{
				Name:        []byte("__typename"),
				Value:       &resolve.String{Path: []string{"__typename"}},
				OnTypeNames: [][]byte{[]byte("Update")},
			},
			{
				Name:        []byte("id"),
				Value:       &resolve.Scalar{Path: []string{"id"}},
				OnTypeNames: [][]byte{[]byte("Update")},
			},
		},
	})
	t.Run("subscription response tree merges same-wave entity fetches", RunTest(
		definition,
		`subscription { update { employees { id products } employee { id notes } } }`,
		"",
		&plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					InputTemplate: resolve.InputTemplate{
						Segments: []resolve.TemplateSegment{
							{
								SegmentType: resolve.StaticSegmentType,
								Data:        []byte(`{"url":"ws://accounts","body":{"query":"subscription{update {__typename id}}"}}`),
							},
						},
					},
					Source: &SubscriptionSource{},
					PostProcessing: resolve.PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
					SourceName: "accounts",
					SourceID:   "accounts",
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("update"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"update"},
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
																Source:              resolve.TypeFieldSource{IDs: []string{"hub"}, Names: []string{"hub"}},
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
													SourceName:    "hub",
													TypeName:      "Employee",
												},
												SkipItem: func(ctx *resolve.Context, value *astjson.Value) bool { return false },
											},
											Info: &resolve.FieldInfo{
												Name:                "employees",
												ExactParentTypeName: "Update",
												ParentTypeNames:     []string{"Update"},
												NamedType:           "Employee",
												Source:              resolve.TypeFieldSource{IDs: []string{"hub"}, Names: []string{"hub"}},
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
															Source:              resolve.TypeFieldSource{IDs: []string{"hub"}, Names: []string{"hub"}},
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
												SourceName:    "hub",
												TypeName:      "Employee",
											},
											Info: &resolve.FieldInfo{
												Name:                "employee",
												ExactParentTypeName: "Update",
												ParentTypeNames:     []string{"Update"},
												NamedType:           "Employee",
												Source:              resolve.TypeFieldSource{IDs: []string{"hub"}, Names: []string{"hub"}},
											},
										},
									},
									PossibleTypes: map[string]struct{}{"Update": {}},
									SourceName:    "accounts",
									TypeName:      "Update",
								},
								Info: &resolve.FieldInfo{
									Name:                "update",
									ExactParentTypeName: "Subscription",
									ParentTypeNames:     []string{"Subscription"},
									NamedType:           "Update",
									Source:              resolve.TypeFieldSource{IDs: []string{"accounts"}, Names: []string{"accounts"}},
								},
							},
						},
					},
					Info: &resolve.GraphQLResponseInfo{OperationType: ast.OperationTypeSubscription},
					Fetches: &resolve.FetchTreeNode{
						Kind: resolve.FetchTreeNodeKindSequence,
						Trigger: &resolve.FetchTreeNode{
							Kind: resolve.FetchTreeNodeKindTrigger,
							Item: &resolve.FetchItem{
								Fetch: &resolve.SingleFetch{
									Info: &resolve.FetchInfo{
										DataSourceID:   "accounts",
										DataSourceName: "accounts",
									},
								},
								ResponsePath: "update",
							},
						},
						ChildNodes: []*resolve.FetchTreeNode{
							{
								Kind: resolve.FetchTreeNodeKindSingle,
								Item: &resolve.FetchItem{
									Fetch: &resolve.EntityFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										Input: resolve.EntityInput{
											Header: resolve.InputTemplate{
												Segments: []resolve.TemplateSegment{
													{
														SegmentType: resolve.StaticSegmentType,
														Data:        []byte(`{"method":"POST","url":"http://hub","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Update {__typename employees {id __typename} employee {id __typename}}}}","variables":{"representations":[`),
													},
												},
												SetTemplateOutputToNullOnVariableNull: true,
											},
											Item: resolve.InputTemplate{
												Segments: []resolve.TemplateSegment{
													{
														SegmentType:  resolve.VariableSegmentType,
														VariableKind: resolve.ResolvableObjectVariableKind,
														Renderer:     updateRepresentationsRenderer,
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
											DataSourceID:   "hub",
											DataSourceName: "hub",
											RootFields: []resolve.GraphCoordinate{
												{TypeName: "Update", FieldName: "employees"},
												{TypeName: "Update", FieldName: "employee"},
											},
											OperationType: ast.OperationTypeQuery,
										},
									},
									FetchPath: []resolve.FetchItemPathElement{
										{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"update"}},
									},
									ResponsePath:         "update",
									ResponsePathElements: []string{"update"},
								},
							},
							{
								Kind: resolve.FetchTreeNodeKindSingle,
								Item: &resolve.FetchItem{
									Fetch: &resolve.MultiEntityFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           2,
											DependsOnFetchIDs: []int{1},
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
															{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"update"}},
															{Kind: resolve.FetchItemPathElementKindArray, Path: []string{"employees"}},
														},
														ResponsePath:         "update.employees",
														ResponsePathElements: []string{"update", "employees"},
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
															{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"update"}},
															{Kind: resolve.FetchItemPathElementKindObject, Path: []string{"employee"}},
														},
														ResponsePath:         "update.employee",
														ResponsePathElements: []string{"update", "employee"},
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
										MergedFetchIDs:       []int{2, 3},
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
						},
					},
				},
			},
		},
		config,
		WithFieldInfo(),
		WithPostProcessor(postprocess.EnableMultiFetch()),
	))
}

// TestGraphQLDataSourceFederation_MultiFetch_RepresentationsCollision exercises
// the planner representations-collision fix end-to-end under
// MultiFetch: the client declares AND uses a variable literally named
// $representations. The planner renames the synthetic representations variable
// to $_representations on every member; the merge stage then applies its normal
// per-member suffix, so the synthetic flows through as $_representations_f1 /
// $_representations_f2, while the client's variable becomes $representations_f1.
// The fetches are merged (no longer refused), proving the collision fix removes
// the MultiFetch opt-out.
func TestGraphQLDataSourceFederation_MultiFetch_RepresentationsCollision(t *testing.T) {
	// The recorded client variable is named "representations" and feeds the
	// products(first:) argument; after the per-member rename it is
	// "representations_f1" and reads context path ["representations"].
	clientRepresentationsVar := resolve.MultiEntityFetchVariable{
		KeyPrefix: []byte(`,"representations_f1":`),
		Value: resolve.InputTemplate{
			Segments: []resolve.TemplateSegment{
				{SegmentType: resolve.StaticSegmentType},
				{
					SegmentType:        resolve.VariableSegmentType,
					VariableKind:       resolve.ContextVariableKind,
					VariableSourcePath: []string{"representations"},
					Renderer:           resolve.NewJSONVariableRenderer(),
				},
				{SegmentType: resolve.StaticSegmentType},
			},
		},
	}

	// f1: employees -> products(first: $representations). Reuse the products
	// entry but override the two fields the rename changes: the renamed synthetic
	// representations key and the client variable.
	productsEntry := multiFetchProductsEntry("f1", "array", "employees", false)
	productsEntry.RepresentationsPrefix = []byte(`"_representations_f1":[`)
	productsEntry.Variables = []resolve.MultiEntityFetchVariable{clientRepresentationsVar}

	// f2: employee -> notes. Only the renamed synthetic key changes.
	notesEntry := multiFetchNotesEntry()
	notesEntry.RepresentationsPrefix = []byte(`,"_representations_f2":[`)

	t.Run("client $representations collides, fetch still merges", func(t *testing.T) {
		// The synthetic is renamed to $_representations; both the merged query
		// ($_representations_f1/$_representations_f2) and the input statics
		// (RepresentationsPrefix) must agree on the renamed key.
		RunTest(
			multiFetchArgDefinition(),
			`query($representations: Int){ employees { id products(first: $representations) } employee { id notes } }`,
			"",
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
													Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query($_representations_f1: [_Any!]!, $representations_f1: Int, $includeF1: Boolean!, $_representations_f2: [_Any!]!, $includeF2: Boolean!){f1: _entities(representations: $_representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $representations_f1)}} f2: _entities(representations: $_representations_f2)@include(if: $includeF2) {... on Employee {__typename notes}}}","variables":{`),
												},
											},
										},
										Entries: []resolve.MultiEntityFetchEntry{
											productsEntry,
											notesEntry,
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
			multiFetchArgPlanConfig(t),
			WithFieldInfo(),
			WithPostProcessor(postprocess.EnableMultiFetch()),
		)(t)
	})
}
