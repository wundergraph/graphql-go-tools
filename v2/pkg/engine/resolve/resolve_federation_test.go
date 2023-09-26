package resolve

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestingTB interface {
	Errorf(format string, args ...interface{})
	Helper()
	FailNow()
}

func mockedDS(t TestingTB, ctrl *gomock.Controller, expectedInput, responseData string) *MockDataSource {
	t.Helper()

	service := NewMockDataSource(ctrl)
	service.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
		DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
			actual := string(input)
			expected := expectedInput

			require.Equal(t, expected, actual)

			pair := NewBufPair()
			pair.Data.WriteString(responseData)

			return writeGraphqlResponse(pair, w, false)
		}).AnyTimes()
	return service
}

func TestResolveGraphQLResponse_Federation(t *testing.T) {
	t.Run("federation: composed keys, requires, provides, shareable", func(t *testing.T) {
		t.Run("composed keys", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			expectedAccountsQuery := `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":[{"__typename":"Account","id":"1234","info":{"a":"foo","b":"bar"}}]}}}`

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						DataSource: mockedDS(
							t, ctrl,
							`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
							`{"user":{"account":{"__typename":"Account","id":"1234","info":{"a":"foo","b":"bar"}}}}`,
						),
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
						Input:                `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("account"),
										Value: &Object{
											Path:     []string{"account"},
											Nullable: true,
											Fields: []*Field{
												{
													Name: []byte("name"),
													Value: &String{
														Path: []string{"name"},
													},
												},
												{
													Name: []byte("shippingInfo"),
													Value: &Object{
														Path:     []string{"shippingInfo"},
														Nullable: true,
														Fields: []*Field{
															{
																Name: []byte("zip"),
																Value: &String{
																	Path: []string{"zip"},
																},
															},
														},
													},
												},
											},
											Fetch: &SingleFetch{
												DataSource: mockedDS(
													t, ctrl,
													expectedAccountsQuery,
													`{"_entities":[{"__typename":"Account","name":"John Doe","shippingInfo":{"zip":"12345"}}]}`,
												),
												Input: `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":$$0$$}}}`,
												InputTemplate: InputTemplate{
													Segments: []TemplateSegment{
														{
															Data:        []byte(`{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":[`),
															SegmentType: StaticSegmentType,
														},
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Fields: []*Field{
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																	{
																		Name: []byte("id"),
																		Value: &String{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("info"),
																		Value: &Object{
																			Path:     []string{"info"},
																			Nullable: true,
																			Fields: []*Field{
																				{
																					Name: []byte("a"),
																					Value: &String{
																						Path: []string{"a"},
																					},
																				},
																				{
																					Name: []byte("b"),
																					Value: &String{
																						Path: []string{"b"},
																					},
																				},
																			},
																		},
																	},
																},
															}),
														},
														{
															Data:        []byte(`]}}}`),
															SegmentType: StaticSegmentType,
														},
													},
												},
												DataSourceIdentifier: []byte("graphql_datasource.Source"),
												PostProcessing: PostProcessingConfiguration{
													SelectResponseDataPath: []string{"data", "_entities", "[0]"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background()}, `{"data":{"user":{"account":{"name":"John Doe","shippingInfo":{"zip":"12345"}}}}}`
		}))

		t.Run("federation with shareable", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			firstService := NewMockDataSource(ctrl)
			firstService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"me": {"__typename": "User", "id": "1234", "details": {"forename": "John", "middlename": "A"}}}`)
					return writeGraphqlResponse(pair, w, false)
				})

			secondService := NewMockDataSource(ctrl)
			secondService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":[{"__typename":"User","id":"1234"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities": [{"__typename": "User", "details": {"surname": "Smith"}}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			thirdService := NewMockDataSource(ctrl)
			thirdService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w *bytes.Buffer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[{"__typename":"User","id":"1234"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities": [{"__typename": "User", "details": {"age": 21}}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						DataSource:           firstService,
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`),
								},
							},
						},
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("me"),
							Value: &Object{
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("details"),
										Value: &Object{
											Path: []string{"details"},
											Fields: []*Field{
												{
													Name: []byte("forename"),
													Value: &String{
														Path: []string{"forename"},
													},
												},
												{
													Name: []byte("surname"),
													Value: &String{
														Path: []string{"surname"},
													},
												},
												{
													Name: []byte("middlename"),
													Value: &String{
														Path: []string{"middlename"},
													},
												},
												{
													Name: []byte("age"),
													Value: &Integer{
														Path: []string{"age"},
													},
												},
											},
										},
									},
								},
								Fetch: &ParallelFetch{
									Fetches: []Fetch{
										&SingleFetch{
											SetTemplateOutputToNullOnVariableNull: true,
											DataSource:                            secondService,
											DataSourceIdentifier:                  []byte("graphql_datasource.Source"),
											InputTemplate: InputTemplate{
												Segments: []TemplateSegment{
													{
														SegmentType: StaticSegmentType,
														Data:        []byte(`{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":[`),
													},
													{
														SegmentType:  VariableSegmentType,
														VariableKind: ResolvableObjectVariableKind,
														Renderer: NewGraphQLVariableResolveRenderer(&Object{
															Fields: []*Field{
																{
																	Name: []byte("__typename"),
																	Value: &String{
																		Path: []string{"__typename"},
																	},
																},
																{
																	Name: []byte("id"),
																	Value: &String{
																		Path: []string{"id"},
																	},
																},
															},
														}),
													},
													{
														SegmentType: StaticSegmentType,
														Data:        []byte(`]}}}`),
													},
												},
											},
											PostProcessing: PostProcessingConfiguration{
												SelectResponseDataPath: []string{"data", "_entities", "[0]"},
											},
										},
										&SingleFetch{
											SetTemplateOutputToNullOnVariableNull: true,
											DataSource:                            thirdService,
											DataSourceIdentifier:                  []byte("graphql_datasource.Source"),
											InputTemplate: InputTemplate{
												Segments: []TemplateSegment{
													{
														SegmentType: StaticSegmentType,
														Data:        []byte(`{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[`),
													},
													{
														SegmentType:  VariableSegmentType,
														VariableKind: ResolvableObjectVariableKind,
														Renderer: NewGraphQLVariableResolveRenderer(&Object{
															Fields: []*Field{
																{
																	Name: []byte("__typename"),
																	Value: &String{
																		Path: []string{"__typename"},
																	},
																},
																{
																	Name: []byte("id"),
																	Value: &String{
																		Path: []string{"id"},
																	},
																},
															},
														}),
													},
													{
														SegmentType: StaticSegmentType,
														Data:        []byte(`]}}}`),
													},
												},
											},
											PostProcessing: PostProcessingConfiguration{
												SelectResponseDataPath: []string{"data", "_entities", "[0]"},
											},
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"details":{"forename":"John","surname":"Smith","middlename":"A","age":21}}}}`
		}))
	})

	t.Run("federation: response renderer", func(t *testing.T) {
		t.Run("multiple entities with response renderer", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename} address {id __typename} } }"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"user":{"name":"Bill","info":{"id":11,"__typename":"Info"},"address":{"id": 55,"__typename":"Address"}}`)
					return writeGraphqlResponse(pair, w, false)
				})

			infoService := NewMockDataSource(ctrl)
			infoService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"},{"id":55,"__typename":"Address"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"age":21,"__typename":"Info"},{"line1":"Munich","__typename":"Address"}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename} address {id __typename} } }"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path: []string{"user"},
								Fetch: &SingleFetch{
									InputTemplate: InputTemplate{
										Segments: []TemplateSegment{
											{
												Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:  VariableSegmentType,
												VariableKind: ResolvableObjectVariableKind,
												Renderer: NewGraphQLVariableResolveRenderer(&Object{
													Path: []string{"info"},
													Fields: []*Field{
														{
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
														},
													},
												}),
											},
											{
												Data:        []byte(`,`),
												SegmentType: StaticSegmentType,
											},
											{
												SegmentType:  VariableSegmentType,
												VariableKind: ResolvableObjectVariableKind,
												Renderer: NewGraphQLVariableResolveRenderer(&Object{
													Path: []string{"address"},
													Fields: []*Field{
														{
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
														},
													},
												}),
											},
											{
												Data:        []byte(`]}}}`),
												SegmentType: StaticSegmentType,
											},
										},
									},
									DataSource: infoService,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseDataPath: []string{"data", "_entities"},
										ResponseTemplate: &InputTemplate{
											Segments: []TemplateSegment{
												{
													SegmentType:  VariableSegmentType,
													VariableKind: ResolvableObjectVariableKind,
													Renderer: NewGraphQLVariableResolveRenderer(&Object{
														Fields: []*Field{
															{
																Name: []byte("info"),
																Value: &Object{
																	Fields: []*Field{
																		{
																			Name: []byte("age"),
																			Value: &Integer{
																				Path: []string{"[0]", "age"},
																			},
																		},
																	},
																},
															},
															{
																Name: []byte("address"),
																Value: &Object{
																	Fields: []*Field{
																		{
																			Name: []byte("line1"),
																			Value: &String{
																				Path: []string{"[1]", "line1"},
																			},
																		},
																	},
																},
															},
														},
													}),
												},
											},
										},
									},
								},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
									{
										Name: []byte("info"),
										Value: &Object{
											Path: []string{"info"},
											Fields: []*Field{
												{
													Name: []byte("age"),
													Value: &Integer{
														Path: []string{"age"},
													},
												},
											},
										},
									},
									{
										Name: []byte("address"),
										Value: &Object{
											Path: []string{"address"},
											Fields: []*Field{
												{
													Name: []byte("line1"),
													Value: &String{
														Path: []string{"line1"},
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
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"user":{"name":"Bill","info":{"age":21},"address":{"line1":"Munich"}}}}`
		}))

		t.Run("multiple entities with response renderer and batching", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"users":[{"name":"Bill","info":{"id":11,"__typename":"Info"},"address":{"id": 55,"__typename":"Address"}},{"name":"John","info":{"id":12,"__typename":"Info"},"address":{"id": 56,"__typename":"Address"}},{"name":"Jane","info":{"id":13,"__typename":"Info"},"address":{"id": 57,"__typename":"Address"}}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			infoService := NewMockDataSource(ctrl)
			infoService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"},{"id":55,"__typename":"Address"},{"id":12,"__typename":"Info"},{"id":56,"__typename":"Address"},{"id":13,"__typename":"Info"},{"id":57,"__typename":"Address"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"age":21,"__typename":"Info"},{"line1":"Munich","__typename":"Address"},{"age":22,"__typename":"Info"},{"line1":"Berlin","__typename":"Address"},{"age":23,"__typename":"Info"},{"line1":"Hamburg","__typename":"Address"}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("users"),
							Value: &Array{
								Path: []string{"users"},
								Item: &Object{
									Fetch: &BatchFetch{
										Input: BatchInput{
											Header: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Items: []InputTemplate{
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Path: []string{"info"},
																Fields: []*Field{
																	{
																		Name: []byte("id"),
																		Value: &Integer{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																},
															}),
														},
													},
												},
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Path: []string{"address"},
																Fields: []*Field{
																	{
																		Name: []byte("id"),
																		Value: &Integer{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																},
															}),
														},
													},
												},
											},
											Separator: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`,`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Footer: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`]}}}`),
														SegmentType: StaticSegmentType,
													},
												},
											},
										},
										DataSource: infoService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities"},
											ResponseTemplate: &InputTemplate{
												Segments: []TemplateSegment{
													{
														SegmentType:  VariableSegmentType,
														VariableKind: ResolvableObjectVariableKind,
														Renderer: NewGraphQLVariableResolveRenderer(&Object{
															Fields: []*Field{
																{
																	Name: []byte("info"),
																	Value: &Object{
																		Fields: []*Field{
																			{
																				Name: []byte("age"),
																				Value: &Integer{
																					Path: []string{"[0]", "age"},
																				},
																			},
																		},
																	},
																},
																{
																	Name: []byte("address"),
																	Value: &Object{
																		Fields: []*Field{
																			{
																				Name: []byte("line1"),
																				Value: &String{
																					Path: []string{"[1]", "line1"},
																				},
																			},
																		},
																	},
																},
															},
														}),
													},
												},
											},
										},
									},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("info"),
											Value: &Object{
												Path: []string{"info"},
												Fields: []*Field{
													{
														Name: []byte("age"),
														Value: &Integer{
															Path: []string{"age"},
														},
													},
												},
											},
										},
										{
											Name: []byte("address"),
											Value: &Object{
												Path: []string{"address"},
												Fields: []*Field{
													{
														Name: []byte("line1"),
														Value: &String{
															Path: []string{"line1"},
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
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"users":[{"name":"Bill","info":{"age":21},"address":{"line1":"Munich"}},{"name":"John","info":{"age":22},"address":{"line1":"Berlin"}},{"name":"Jane","info":{"age":23},"address":{"line1":"Hamburg"}}]}}`
		}))

		t.Run("multiple entities with response renderer and batching, duplicates", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"users":[{"name":"Bill","info":{"id":11,"__typename":"Info"},"address":{"id": 55,"__typename":"Address"}},{"name":"John","info":{"id":12,"__typename":"Info"},"address":{"id": 55,"__typename":"Address"}},{"name":"Jane","info":{"id":13,"__typename":"Info"},"address":{"id": 55,"__typename":"Address"}}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			infoService := NewMockDataSource(ctrl)
			infoService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"},{"id":55,"__typename":"Address"},{"id":12,"__typename":"Info"},{"id":13,"__typename":"Info"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"age":21,"__typename":"Info"},{"line1":"Munich","__typename":"Address"},{"age":22,"__typename":"Info"},{"age":23,"__typename":"Info"}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("users"),
							Value: &Array{
								Path: []string{"users"},
								Item: &Object{
									Fetch: &BatchFetch{
										Input: BatchInput{
											Header: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Items: []InputTemplate{
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Path: []string{"info"},
																Fields: []*Field{
																	{
																		Name: []byte("id"),
																		Value: &Integer{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																},
															}),
														},
													},
												},
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Path: []string{"address"},
																Fields: []*Field{
																	{
																		Name: []byte("id"),
																		Value: &Integer{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																},
															}),
														},
													},
												},
											},
											Separator: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`,`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Footer: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`]}}}`),
														SegmentType: StaticSegmentType,
													},
												},
											},
										},
										DataSource: infoService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities"},
											ResponseTemplate: &InputTemplate{
												Segments: []TemplateSegment{
													{
														SegmentType:  VariableSegmentType,
														VariableKind: ResolvableObjectVariableKind,
														Renderer: NewGraphQLVariableResolveRenderer(&Object{
															Fields: []*Field{
																{
																	Name: []byte("info"),
																	Value: &Object{
																		Fields: []*Field{
																			{
																				Name: []byte("age"),
																				Value: &Integer{
																					Path: []string{"[0]", "age"},
																				},
																			},
																		},
																	},
																},
																{
																	Name: []byte("address"),
																	Value: &Object{
																		Fields: []*Field{
																			{
																				Name: []byte("line1"),
																				Value: &String{
																					Path: []string{"[1]", "line1"},
																				},
																			},
																		},
																	},
																},
															},
														}),
													},
												},
											},
										},
									},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("info"),
											Value: &Object{
												Path: []string{"info"},
												Fields: []*Field{
													{
														Name: []byte("age"),
														Value: &Integer{
															Path: []string{"age"},
														},
													},
												},
											},
										},
										{
											Name: []byte("address"),
											Value: &Object{
												Path: []string{"address"},
												Fields: []*Field{
													{
														Name: []byte("line1"),
														Value: &String{
															Path: []string{"line1"},
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
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"users":[{"name":"Bill","info":{"age":21},"address":{"line1":"Munich"}},{"name":"John","info":{"age":22},"address":{"line1":"Munich"}},{"name":"Jane","info":{"age":23},"address":{"line1":"Munich"}}]}}`
		}))

		t.Run("multiple entities with response renderer and batching, one null", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"users":[{"name":"Bill","info":{"id":11,"__typename":"Info"},"address":{"id": 55,"__typename":"Address"}},{"name":"John","address":{"id": 56,"__typename":"Address"}},{"name":"Jane","info":{"id":13,"__typename":"Info"},"address":{"id": 57,"__typename":"Address"}}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			infoService := NewMockDataSource(ctrl)
			infoService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"},{"id":55,"__typename":"Address"},{"id":56,"__typename":"Address"},{"id":13,"__typename":"Info"},{"id":57,"__typename":"Address"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"age":21,"__typename":"Info"},{"line1":"Munich","__typename":"Address"},{"line1":"Berlin","__typename":"Address"},{"age":23,"__typename":"Info"},{"line1":"Hamburg","__typename":"Address"}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("users"),
							Value: &Array{
								Path: []string{"users"},
								Item: &Object{
									Fetch: &BatchFetch{
										Input: BatchInput{
											Header: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											SkipNullItems: true,
											Items: []InputTemplate{
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Path:     []string{"info"},
																Nullable: true,
																Fields: []*Field{
																	{
																		Name: []byte("id"),
																		Value: &Integer{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																},
															}),
														},
													},
												},
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Path:     []string{"address"},
																Nullable: true,
																Fields: []*Field{
																	{
																		Name: []byte("id"),
																		Value: &Integer{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																},
															}),
														},
													},
												},
											},
											Separator: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`,`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Footer: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`]}}}`),
														SegmentType: StaticSegmentType,
													},
												},
											},
										},
										DataSource: infoService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities"},
											ResponseTemplate: &InputTemplate{
												Segments: []TemplateSegment{
													{
														SegmentType:  VariableSegmentType,
														VariableKind: ResolvableObjectVariableKind,
														Renderer: NewGraphQLVariableResolveRenderer(&Object{
															Fields: []*Field{
																{
																	Name: []byte("info"),
																	Value: &Object{
																		Nullable: true,
																		Fields: []*Field{
																			{
																				Name: []byte("age"),
																				Value: &Integer{
																					Path: []string{"[0]", "age"},
																				},
																			},
																		},
																	},
																},
																{
																	Name: []byte("address"),
																	Value: &Object{
																		Nullable: true,
																		Fields: []*Field{
																			{
																				Name: []byte("line1"),
																				Value: &String{
																					Path: []string{"[1]", "line1"},
																				},
																			},
																		},
																	},
																},
															},
														}),
													},
												},
											},
										},
									},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("info"),
											Value: &Object{
												Nullable: true,
												Path:     []string{"info"},
												Fields: []*Field{
													{
														Name: []byte("age"),
														Value: &Integer{
															Path: []string{"age"},
														},
													},
												},
											},
										},
										{
											Name: []byte("address"),
											Value: &Object{
												Nullable: true,
												Path:     []string{"address"},
												Fields: []*Field{
													{
														Name: []byte("line1"),
														Value: &String{
															Path: []string{"line1"},
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
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"users":[{"name":"Bill","info":{"age":21},"address":{"line1":"Munich"}},{"name":"John","info":null,"address":{"line1":"Berlin"}},{"name":"Jane","info":{"age":23},"address":{"line1":"Hamburg"}}]}}`
		}))

		t.Run("multiple entities with response renderer and batching, one render err", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			userService := NewMockDataSource(ctrl)
			userService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"users":[{"name":"Bill","info":{"id":11,"__typename":"Info"},"address":{"id":true,"__typename":"Address"}},{"name":"John","info":{"id":12,"__typename":"Info"},"address":{"id": 56,"__typename":"Address"}},{"name":"Jane","info":{"id":13,"__typename":"Info"},"address":{"id": 57,"__typename":"Address"}}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			infoService := NewMockDataSource(ctrl)
			infoService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
				DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"},{"id":12,"__typename":"Info"},{"id":56,"__typename":"Address"},{"id":13,"__typename":"Info"},{"id":57,"__typename":"Address"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"_entities":[{"age":21,"__typename":"Info"},{"age":22,"__typename":"Info"},{"line1":"Berlin","__typename":"Address"},{"age":23,"__typename":"Info"},{"line1":"Hamburg","__typename":"Address"}]}`)
					return writeGraphqlResponse(pair, w, false)
				})

			return &GraphQLResponse{
				Data: &Object{
					Fetch: &SingleFetch{
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename} address {id __typename} } }"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
						DataSource: userService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					Fields: []*Field{
						{
							Name: []byte("users"),
							Value: &Array{
								Path: []string{"users"},
								Item: &Object{
									Fetch: &BatchFetch{
										Input: BatchInput{
											Header: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											SkipErrItems: true,
											Items: []InputTemplate{
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Path: []string{"info"},
																Fields: []*Field{
																	{
																		Name: []byte("id"),
																		Value: &Integer{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																},
															}),
														},
													},
												},
												{
													Segments: []TemplateSegment{
														{
															SegmentType:  VariableSegmentType,
															VariableKind: ResolvableObjectVariableKind,
															Renderer: NewGraphQLVariableResolveRenderer(&Object{
																Path: []string{"address"},
																Fields: []*Field{
																	{
																		Name: []byte("id"),
																		Value: &Integer{
																			Path: []string{"id"},
																		},
																	},
																	{
																		Name: []byte("__typename"),
																		Value: &String{
																			Path: []string{"__typename"},
																		},
																	},
																},
															}),
														},
													},
												},
											},
											Separator: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`,`),
														SegmentType: StaticSegmentType,
													},
												},
											},
											Footer: InputTemplate{
												Segments: []TemplateSegment{
													{
														Data:        []byte(`]}}}`),
														SegmentType: StaticSegmentType,
													},
												},
											},
										},
										DataSource: infoService,
										PostProcessing: PostProcessingConfiguration{
											SelectResponseDataPath: []string{"data", "_entities"},
											ResponseTemplate: &InputTemplate{
												Segments: []TemplateSegment{
													{
														SegmentType:  VariableSegmentType,
														VariableKind: ResolvableObjectVariableKind,
														Renderer: NewGraphQLVariableResolveRenderer(&Object{
															Fields: []*Field{
																{
																	Name: []byte("info"),
																	Value: &Object{
																		Nullable: true,
																		Fields: []*Field{
																			{
																				Name: []byte("age"),
																				Value: &Integer{
																					Path: []string{"[0]", "age"},
																				},
																			},
																		},
																	},
																},
																{
																	Name: []byte("address"),
																	Value: &Object{
																		Nullable: true,
																		Fields: []*Field{
																			{
																				Name: []byte("line1"),
																				Value: &String{
																					Path: []string{"[1]", "line1"},
																				},
																			},
																		},
																	},
																},
															},
														}),
													},
												},
											},
										},
									},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("info"),
											Value: &Object{
												Nullable: true,
												Path:     []string{"info"},
												Fields: []*Field{
													{
														Name: []byte("age"),
														Value: &Integer{
															Path: []string{"age"},
														},
													},
												},
											},
										},
										{
											Name: []byte("address"),
											Value: &Object{
												Nullable: true,
												Path:     []string{"address"},
												Fields: []*Field{
													{
														Name: []byte("line1"),
														Value: &String{
															Path: []string{"line1"},
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
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"users":[{"name":"Bill","info":{"age":21},"address":null},{"name":"John","info":{"age":22},"address":{"line1":"Berlin"}},{"name":"Jane","info":{"age":23},"address":{"line1":"Hamburg"}}]}}`
		}))
	})

	t.Run("serial fetch", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		user := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {__typename id line1 line2}}}}"}}`,
			`{"user":{"account":{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}}}}`)

		addressEnricher := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {country city}}}","variables":{"representations":[{"__typename":"Address","id":"address-1"}]}}}`,
			`{"__typename":"Address","country":"country-1","city":"city-1"}`)

		address := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {line3(test: "BOOM") zip}}}","variables":{"representations":[{"__typename":"Address","id":"address-1","country":"country-1","city":"city-1"}]}}}`,
			`{"__typename": "Address", "line3": "line3-1", "zip": "zip-1"}`)

		account := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {fullAddress}}}","variables":{"representations":[{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}]}}}`,
			`{"__typename":"Address","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1"}`)

		return &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								SegmentType: StaticSegmentType,
								Data:        []byte(`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {__typename id line1 line2}}}}"}}`),
							},
						},
					},
					DataSource:           user,
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("account"),
									Value: &Object{
										Path:     []string{"account"},
										Nullable: true,
										Fields: []*Field{
											{
												Name: []byte("address"),
												Value: &Object{
													Path:     []string{"address"},
													Nullable: true,
													Fields: []*Field{
														{
															Name: []byte("fullAddress"),
															Value: &String{
																Path: []string{"fullAddress"},
															},
														},
													},
													Fetch: &SerialFetch{
														Fetches: []Fetch{
															&SingleFetch{
																Input:                `{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {country city}}}","variables":{"representations":$$0$$}}}`,
																DataSource:           addressEnricher,
																DataSourceIdentifier: []byte("graphql_datasource.Source"),
																PostProcessing: PostProcessingConfiguration{
																	SelectResponseDataPath: []string{"data"},
																},
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			SegmentType: StaticSegmentType,
																			Data:        []byte(`{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {country city}}}","variables":{"representations":[`),
																		},
																		{
																			SegmentType:  VariableSegmentType,
																			VariableKind: ResolvableObjectVariableKind,
																			Renderer: NewGraphQLVariableResolveRenderer(&Object{
																				Fields: []*Field{
																					{
																						Name: []byte("__typename"),
																						Value: &String{
																							Path: []string{"__typename"},
																						},
																					},
																					{
																						Name: []byte("id"),
																						Value: &String{
																							Path: []string{"id"},
																						},
																					},
																				},
																			}),
																		},
																		{
																			SegmentType: StaticSegmentType,
																			Data:        []byte(`]}}}`),
																		},
																	},
																},
															},
															&SingleFetch{
																Input:                `{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {line3(test: "BOOM") zip}}}","variables":{"representations":$$0$$}}}`,
																DataSource:           address,
																DataSourceIdentifier: []byte("graphql_datasource.Source"),
																PostProcessing: PostProcessingConfiguration{
																	SelectResponseDataPath: []string{"data"},
																},
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			SegmentType: StaticSegmentType,
																			Data:        []byte(`{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {line3(test: "BOOM") zip}}}","variables":{"representations":[`),
																		},
																		{
																			SegmentType:  VariableSegmentType,
																			VariableKind: ResolvableObjectVariableKind,
																			Renderer: NewGraphQLVariableResolveRenderer(&Object{
																				Fields: []*Field{
																					{
																						Name: []byte("__typename"),
																						Value: &String{
																							Path: []string{"__typename"},
																						},
																					},
																					{
																						Name: []byte("id"),
																						Value: &String{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("country"),
																						Value: &String{
																							Path: []string{"country"},
																						},
																					},
																					{
																						Name: []byte("city"),
																						Value: &String{
																							Path: []string{"city"},
																						},
																					},
																				},
																			}),
																		},
																		{
																			SegmentType: StaticSegmentType,
																			Data:        []byte(`]}}}`),
																		},
																	},
																},
															},
															&SingleFetch{
																Input:                `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {fullAddress}}}","variables":{"representations":$$0$$}}}`,
																DataSource:           account,
																DataSourceIdentifier: []byte("graphql_datasource.Source"),
																PostProcessing: PostProcessingConfiguration{
																	SelectResponseDataPath: []string{"data"},
																},
																InputTemplate: InputTemplate{
																	Segments: []TemplateSegment{
																		{
																			SegmentType: StaticSegmentType,
																			Data:        []byte(`{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {fullAddress}}}","variables":{"representations":[`),
																		},
																		{
																			SegmentType:  VariableSegmentType,
																			VariableKind: ResolvableObjectVariableKind,
																			Renderer: NewGraphQLVariableResolveRenderer(&Object{
																				Fields: []*Field{
																					{
																						Name: []byte("__typename"),
																						Value: &String{
																							Path: []string{"__typename"},
																						},
																					},
																					{
																						Name: []byte("id"),
																						Value: &String{
																							Path: []string{"id"},
																						},
																					},
																					{
																						Name: []byte("line1"),
																						Value: &String{
																							Path: []string{"line1"},
																						},
																					},
																					{
																						Name: []byte("line2"),
																						Value: &String{
																							Path: []string{"line2"},
																						},
																					},
																					{
																						Name: []byte("line3"),
																						Value: &String{
																							Path: []string{"line3"},
																						},
																					},
																					{
																						Name: []byte("zip"),
																						Value: &String{
																							Path: []string{"zip"},
																						},
																					},
																				},
																			}),
																		},
																		{
																			SegmentType: StaticSegmentType,
																			Data:        []byte(`]}}}`),
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
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background()}, `{"data":{"user":{"account":{"address":{"fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1"}}}}}`
	}))

	t.Run("nested batching", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		productsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`,
			`{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}`)

		reviewsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
			`{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}`)

		stockService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
			`{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}`)

		usersService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}}`,
			`{"_entities":[{"name":"user-1"},{"name":"user-2"}]}`)

		return &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSource: productsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Fields: []*Field{
					{
						Name: []byte("topProducts"),
						Value: &Array{
							Path: []string{"topProducts"},
							Item: &Object{
								Fetch: &ParallelFetch{
									Fetches: []Fetch{
										&BatchFetch{
											Input: BatchInput{
												Header: InputTemplate{
													Segments: []TemplateSegment{
														{
															Data:        []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[`),
															SegmentType: StaticSegmentType,
														},
													},
												},
												Items: []InputTemplate{
													{
														Segments: []TemplateSegment{
															{
																SegmentType:  VariableSegmentType,
																VariableKind: ResolvableObjectVariableKind,
																Renderer: NewGraphQLVariableResolveRenderer(&Object{
																	Fields: []*Field{
																		{
																			Name: []byte("__typename"),
																			Value: &String{
																				Path: []string{"__typename"},
																			},
																		},
																		{
																			Name: []byte("upc"),
																			Value: &String{
																				Path: []string{"upc"},
																			},
																		},
																	},
																}),
															},
														},
													},
												},
												Separator: InputTemplate{
													Segments: []TemplateSegment{
														{
															Data:        []byte(`,`),
															SegmentType: StaticSegmentType,
														},
													},
												},
												Footer: InputTemplate{
													Segments: []TemplateSegment{
														{
															Data:        []byte(`]}}}`),
															SegmentType: StaticSegmentType,
														},
													},
												},
											},
											DataSource: reviewsService,
											PostProcessing: PostProcessingConfiguration{
												SelectResponseDataPath: []string{"data", "_entities"},
											},
										},
										&BatchFetch{
											Input: BatchInput{
												Header: InputTemplate{
													Segments: []TemplateSegment{
														{
															Data:        []byte(`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[`),
															SegmentType: StaticSegmentType,
														},
													},
												},
												Items: []InputTemplate{
													{
														Segments: []TemplateSegment{
															{
																SegmentType:  VariableSegmentType,
																VariableKind: ResolvableObjectVariableKind,
																Renderer: NewGraphQLVariableResolveRenderer(&Object{
																	Fields: []*Field{
																		{
																			Name: []byte("__typename"),
																			Value: &String{
																				Path: []string{"__typename"},
																			},
																		},
																		{
																			Name: []byte("upc"),
																			Value: &String{
																				Path: []string{"upc"},
																			},
																		},
																	},
																}),
															},
														},
													},
												},
												Separator: InputTemplate{
													Segments: []TemplateSegment{
														{
															Data:        []byte(`,`),
															SegmentType: StaticSegmentType,
														},
													},
												},
												Footer: InputTemplate{
													Segments: []TemplateSegment{
														{
															Data:        []byte(`]}}}`),
															SegmentType: StaticSegmentType,
														},
													},
												},
											},
											DataSource: stockService,
											PostProcessing: PostProcessingConfiguration{
												SelectResponseDataPath: []string{"data", "_entities"},
											},
										},
									},
								},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
									{
										Name: []byte("stock"),
										Value: &Integer{
											Path: []string{"stock"},
										},
									},
									{
										Name: []byte("reviews"),
										Value: &Array{
											Path: []string{"reviews"},
											Item: &Object{
												Fields: []*Field{
													{
														Name: []byte("body"),
														Value: &String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("author"),
														Value: &Object{
															Path: []string{"author"},
															Fetch: &BatchFetch{
																Input: BatchInput{
																	Header: InputTemplate{
																		Segments: []TemplateSegment{
																			{
																				Data:        []byte(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[`),
																				SegmentType: StaticSegmentType,
																			},
																		},
																	},
																	Items: []InputTemplate{
																		{
																			Segments: []TemplateSegment{
																				{
																					SegmentType:  VariableSegmentType,
																					VariableKind: ResolvableObjectVariableKind,
																					Renderer: NewGraphQLVariableResolveRenderer(&Object{
																						Fields: []*Field{
																							{
																								Name: []byte("__typename"),
																								Value: &String{
																									Path: []string{"__typename"},
																								},
																							},
																							{
																								Name: []byte("id"),
																								Value: &String{
																									Path: []string{"id"},
																								},
																							},
																						},
																					}),
																				},
																			},
																		},
																	},
																	Separator: InputTemplate{
																		Segments: []TemplateSegment{
																			{
																				Data:        []byte(`,`),
																				SegmentType: StaticSegmentType,
																			},
																		},
																	},
																	Footer: InputTemplate{
																		Segments: []TemplateSegment{
																			{
																				Data:        []byte(`]}}}`),
																				SegmentType: StaticSegmentType,
																			},
																		},
																	},
																},
																DataSource: usersService,
																PostProcessing: PostProcessingConfiguration{
																	SelectResponseDataPath: []string{"data", "_entities"},
																},
															},
															Fields: []*Field{
																{
																	Name: []byte("name"),
																	Value: &String{
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
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}},{"body":"Prefer other Table.","author":{"name":"user-2"}}]},{"name":"Couch","stock":2,"reviews":[{"body":"Couch Too expensive.","author":{"name":"user-1"}}]},{"name":"Chair","stock":5,"reviews":[{"body":"Chair Could be better.","author":{"name":"user-2"}}]}]}}`
	}))

	t.Run("nested batching of direct array childs", testFn(true, func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		accountsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://accounts","body":{"query":"{accounts{__typename ... on User {__typename id} ... on Moderator {__typename moderatorID} ... on Admin {__typename adminID}}}"}}`,
			`{"accounts":[{"__typename":"User","id":"3"},{"__typename":"Admin","adminID":"2"},{"__typename":"Moderator","moderatorID":"1"}]}`)

		namesService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://names","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name} ... on Moderator {subject} ... on Admin {type}}}","variables":{"representations":[{"__typename":"User","id":"3"},{"__typename":"Admin","adminID":"2"},{"__typename":"Moderator","moderatorID":"1"}]}}}`,
			`{"_entities":[{"__typename":"User","name":"User"},{"__typename":"Admin","type":"super"},{"__typename":"Moderator","subject":"posts"}]}`)

		return &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://accounts","body":{"query":"{accounts{__typename ... on User {__typename id} ... on Moderator {__typename moderatorID} ... on Admin {__typename adminID}}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSource: accountsService,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Fields: []*Field{
					{
						Name: []byte("accounts"),
						Value: &Array{
							Path: []string{"accounts"},
							Item: &Object{
								Fetch: &BatchFetch{
									Input: BatchInput{
										Header: InputTemplate{
											Segments: []TemplateSegment{
												{
													Data:        []byte(`{"method":"POST","url":"http://names","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name} ... on Moderator {subject} ... on Admin {type}}}","variables":{"representations":[`),
													SegmentType: StaticSegmentType,
												},
											},
										},
										Items: []InputTemplate{
											{
												Segments: []TemplateSegment{
													{
														SegmentType:  VariableSegmentType,
														VariableKind: ResolvableObjectVariableKind,
														Renderer: NewGraphQLVariableResolveRenderer(&Object{
															Fields: []*Field{
																{
																	Name: []byte("__typename"),
																	Value: &String{
																		Path: []string{"__typename"},
																	},
																},
																{
																	Name: []byte("id"),
																	Value: &String{
																		Path: []string{"id"},
																	},
																	OnTypeNames: [][]byte{[]byte("User")},
																},
																{
																	Name: []byte("adminID"),
																	Value: &String{
																		Path: []string{"adminID"},
																	},
																	OnTypeNames: [][]byte{[]byte("Admin")},
																},
																{
																	Name: []byte("moderatorID"),
																	Value: &String{
																		Path: []string{"moderatorID"},
																	},
																	OnTypeNames: [][]byte{[]byte("Moderator")},
																},
															},
														}),
													},
												},
											},
										},
										Separator: InputTemplate{
											Segments: []TemplateSegment{
												{
													Data:        []byte(`,`),
													SegmentType: StaticSegmentType,
												},
											},
										},
										Footer: InputTemplate{
											Segments: []TemplateSegment{
												{
													Data:        []byte(`]}}}`),
													SegmentType: StaticSegmentType,
												},
											},
										},
									},
									DataSource: namesService,
									PostProcessing: PostProcessingConfiguration{
										SelectResponseDataPath: []string{"data", "_entities"},
									},
								},
								Fields: []*Field{
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
										OnTypeNames: [][]byte{[]byte("User")},
									},
									{
										Name: []byte("type"),
										Value: &String{
											Path: []string{"type"},
										},
										OnTypeNames: [][]byte{[]byte("Admin")},
									},
									{
										Name: []byte("subject"),
										Value: &String{
											Path: []string{"subject"},
										},
										OnTypeNames: [][]byte{[]byte("Moderator")},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"accounts":[{"name":"User"},{"type":"super"},{"subject":"posts"}]}}`
	}))
}
