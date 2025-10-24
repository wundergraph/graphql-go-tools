package resolve

import (
	"context"
	"net/http"
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
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
			require.Equal(t, expectedInput, string(input))
			return []byte(responseData), nil
		}).Times(1)
	return service
}

func TestResolveGraphQLResponse_Federation(t *testing.T) {
	t.Run("federation: composed keys, requires, provides, shareable", func(t *testing.T) {
		t.Run("composed keys", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			expectedAccountsQuery := `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":[{"__typename":"Account","id":"1234","info":{"a":"foo","b":"bar"}}]}}}`

			return &GraphQLResponse{
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: mockedDS(
								t, ctrl,
								`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
								`{"data":{"user":{"account":{"__typename":"Account","id":"1234","info":{"a":"foo","b":"bar"}}}}}`,
							),
							Input: `{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {__typename id info {a b}}}}"}}`),
									SegmentType: StaticSegmentType,
								},
							},
						},
					}, "query"),
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: mockedDS(
								t, ctrl,
								expectedAccountsQuery,
								`{"data":{"_entities":[{"__typename":"Account","name":"John Doe","shippingInfo":{"zip":"12345"}}]}}`,
							),
							Input: `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Account {name shippingInfo {zip}}}}","variables":{"representations":$$0$$}}}`,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						},
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
					}, "query.user.account", ObjectPath("user"), ObjectPath("account")),
				),
				Data: &Object{
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
										},
									},
								},
							},
						},
					},
				},
			}, Context{ctx: context.Background()}, `{"data":{"user":{"account":{"name":"John Doe","shippingInfo":{"zip":"12345"}}}}}`
		}))

		t.Run("federation with shareable", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
			firstService := NewMockDataSource(ctrl)
			firstService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"data":{"me": {"__typename": "User", "id": "1234", "details": {"forename": "John", "middlename": "A"}}}}`)
					return pair.Data.Bytes(), nil
				})

			secondService := NewMockDataSource(ctrl)
			secondService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://second.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {surname}}}}","variables":{"representations":[{"__typename":"User","id":"1234"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"data":{"_entities": [{"__typename": "User", "details": {"surname": "Smith"}}]}}`)
					return pair.Data.Bytes(), nil
				})

			thirdService := NewMockDataSource(ctrl)
			thirdService.EXPECT().
				Load(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
					actual := string(input)
					expected := `{"method":"POST","url":"http://third.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {details {age}}}}","variables":{"representations":[{"__typename":"User","id":"1234"}]}}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"data":{"_entities": [{"__typename": "User", "details": {"age": 21}}]}}`)
					return pair.Data.Bytes(), nil
				})

			return &GraphQLResponse{
				Fetches: Sequence(
					SingleWithPath(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: firstService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{
								{
									SegmentType: StaticSegmentType,
									Data:        []byte(`{"method":"POST","url":"http://first.service","body":{"query":"{me {details {forename middlename} __typename id}}"}}`),
								},
							},
						},
					}, "query"),
					Parallel(
						SingleWithPath(&SingleFetch{
							FetchConfiguration: FetchConfiguration{
								SetTemplateOutputToNullOnVariableNull: true,
								DataSource:                            secondService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data", "_entities", "0"},
								},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
						}, "query.me", ObjectPath("me")),
						SingleWithPath(&SingleFetch{
							FetchConfiguration: FetchConfiguration{
								SetTemplateOutputToNullOnVariableNull: true,
								DataSource:                            thirdService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data", "_entities", "0"},
								},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
						}, "query.me", ObjectPath("me")),
					),
				),
				Data: &Object{
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
							},
						},
					},
				},
			}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"me":{"details":{"forename":"John","surname":"Smith","middlename":"A","age":21}}}}`
		}))
	})

	t.Run("federation input render", func(t *testing.T) {
		t.Run("batch entity fetch", func(t *testing.T) {
			t.Run("batching on union", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name infoOrAddress { ... on Info {id __typename} ... on Address {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"user":{"name":"Bill","infoOrAddress":[{"id":11,"__typename":"Info"},{"id": 55,"__typename":"Address"}]}}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"},{"id":55,"__typename":"Address"}]}}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"_entities":[{"age":21,"__typename":"Info"},{"line1":"Munich","__typename":"Address"}]}}`)
						return pair.Data.Bytes(), nil
					})

				return &GraphQLResponse{
					Fetches: Sequence(
						SingleWithPath(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name infoOrAddress { ... on Info {id __typename} ... on Address {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}, "query"),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[`),
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
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Info"), []byte("Address")},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("Info"), []byte("Address")},
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
								SkipNullItems:        true,
								SkipEmptyObjectItems: true,
								SkipErrItems:         true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						}, "user.infoOrAddress", ObjectPath("user"), ArrayPath("infoOrAddress")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Path: []string{"user"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("infoOrAddress"),
											Value: &Array{
												Path: []string{"infoOrAddress"},
												Item: &Object{
													Fields: []*Field{
														{
															Name: []byte("age"),
															Value: &Integer{
																Path: []string{"age"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
														},
														{
															Name: []byte("line1"),
															Value: &String{
																Path: []string{"line1"},
															},
															OnTypeNames: [][]byte{[]byte("Address")},
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
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"user":{"name":"Bill","infoOrAddress":[{"age":21},{"line1":"Munich"}]}}}`
			}))

			t.Run("batching on union - all not matching items", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name infoOrAddress { ... on Info {id __typename} ... on Address {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"user":{"name":"Bill","infoOrAddress":[{"id":11,"__typename":"Whatever"},{"id": 55,"__typename":"Whatever"}]}}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				return &GraphQLResponse{
					Fetches: Sequence(
						SingleWithPath(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name infoOrAddress { ... on Info {id __typename} ... on Address {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}, "query"),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age } ... on Address { line1 }}}}}","variables":{"representations":[`),
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
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Info"), []byte("Address")},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("Info"), []byte("Address")},
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
								SkipNullItems:        true,
								SkipEmptyObjectItems: true,
								SkipErrItems:         true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						}, "user.infoOrAddress", ObjectPath("user"), ArrayPath("infoOrAddress")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Path: []string{"user"},
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &String{
												Path: []string{"name"},
											},
										},
										{
											Name: []byte("infoOrAddress"),
											Value: &Array{
												Path: []string{"infoOrAddress"},
												Item: &Object{
													Fields: []*Field{
														{
															Name: []byte("age"),
															Value: &Integer{
																Path: []string{"age"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
														},
														{
															Name: []byte("line1"),
															Value: &String{
																Path: []string{"line1"},
															},
															OnTypeNames: [][]byte{[]byte("Address")},
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
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"user":{"name":"Bill","infoOrAddress":[{},{}]}}}`
			}))

			t.Run("batching on a field", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"users":[{"name":"Bill","info":{"id":11,"__typename":"Info"}},{"name":"John","info":{"id":12,"__typename":"Info"}},{"name":"Jane","info":{"id":13,"__typename":"Info"}}]}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"},{"id":12,"__typename":"Info"},{"id":13,"__typename":"Info"}]}}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"_entities":[{"age":21,"__typename":"Info"},{"age":22,"__typename":"Info"},{"age":23,"__typename":"Info"}]}}`)
						return pair.Data.Bytes(), nil
					})

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
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
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
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
								SkipNullItems:        true,
								SkipEmptyObjectItems: true,
								SkipErrItems:         true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						}, "users.info", ArrayPath("users"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("users"),
								Value: &Array{
									Path: []string{"users"},
									Item: &Object{
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
										},
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"users":[{"name":"Bill","info":{"age":21}},{"name":"John","info":{"age":22}},{"name":"Jane","info":{"age":23}}]}}`
			}))

			t.Run("batching with duplicates", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"users":[{"name":"Bill","info":{"id":11,"__typename":"Info"}},{"name":"John","info":{"id":11,"__typename":"Info"}},{"name":"Jane","info":{"id":11,"__typename":"Info"}}]}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"}]}}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"_entities":[{"age":77,"__typename":"Info"}]}}`)
						return pair.Data.Bytes(), nil
					})

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
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
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
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
							},
						}, "users.info", ArrayPath("users"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("users"),
								Value: &Array{
									Path: []string{"users"},
									Item: &Object{
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
										},
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"users":[{"name":"Bill","info":{"age":77}},{"name":"John","info":{"age":77}},{"name":"Jane","info":{"age":77}}]}}`
			}))

			t.Run("batching with null entry", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"users":[{"name":"Bill","info":{"id":11,"__typename":"Info"}},{"name":"John","info":null},{"name":"Jane","info":{"id":13,"__typename":"Info"}}]}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"},{"id":13,"__typename":"Info"}]}}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"_entities":[{"age":21,"__typename":"Info"},{"age":23,"__typename":"Info"}]}}`)
						return pair.Data.Bytes(), nil
					})

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
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
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
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
								SkipNullItems:        true,
								SkipEmptyObjectItems: true,
								SkipErrItems:         true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						}, "users.info", ArrayPath("users"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("users"),
								Value: &Array{
									Path: []string{"users"},
									Item: &Object{
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
										},
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"users":[{"name":"Bill","info":{"age":21}},{"name":"John","info":null},{"name":"Jane","info":{"age":23}}]}}`
			}))

			t.Run("batching with all null entries", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"users":[{"name":"Bill","info":null},{"name":"John","info":null},{"name":"Jane","info":null}]}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
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
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
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
								SkipNullItems:        true,
								SkipEmptyObjectItems: true,
								SkipErrItems:         true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						}, "users.info", ArrayPath("users"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("users"),
								Value: &Array{
									Path: []string{"users"},
									Item: &Object{
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
										},
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"users":[{"name":"Bill","info":null},{"name":"John","info":null},{"name":"Jane","info":null}]}}`
			}))

			t.Run("batching with render error", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						// render error - first item id is boolean
						pair.Data.WriteString(`{"data":{"users":[{"name":"Bill","info":{"id":true,"__typename":"Info"}},{"name":"John","info":{"id":12,"__typename":"Info"}},{"name":"Jane","info":{"id":13,"__typename":"Info"}}]}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[{"id":12,"__typename":"Info"},{"id":13,"__typename":"Info"}]}}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"_entities":[{"age":21,"__typename":"Info"},{"age":22,"__typename":"Info"}]}}`)
						return pair.Data.Bytes(), nil
					})

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ users { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&BatchEntityFetch{
							Input: BatchInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
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
															Name: []byte("id"),
															Value: &Integer{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
														},
														{
															Name: []byte("__typename"),
															Value: &String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("Info")},
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
								SkipNullItems:        true,
								SkipEmptyObjectItems: true,
								SkipErrItems:         true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities"},
							},
						}, "users.info", ArrayPath("users"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("users"),
								Value: &Array{
									Path: []string{"users"},
									Item: &Object{
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
										},
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.users.info.age'.","path":["users",0,"info","age"]}],"data":null}`
			}))
		})

		t.Run("single entity fetch", func(t *testing.T) {
			t.Run("all data", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"user":{"name":"Bill","info":{"id":11,"__typename":"Info"}}}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[{"id":11,"__typename":"Info"}]}}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"_entities":[{"age":21,"__typename":"Info"}]}}`)
						return pair.Data.Bytes(), nil
					})

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&EntityFetch{
							FetchDependencies: FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							Input: EntityInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
											SegmentType: StaticSegmentType,
										},
									},
								},
								Item: InputTemplate{
									Segments: []TemplateSegment{
										{
											SegmentType:  VariableSegmentType,
											VariableKind: ResolvableObjectVariableKind,
											Renderer: NewGraphQLVariableResolveRenderer(&Object{
												Fields: []*Field{
													{
														Name: []byte("id"),
														Value: &Integer{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
													{
														Name: []byte("__typename"),
														Value: &String{
															Path: []string{"__typename"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
												},
											}),
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
								SkipErrItem: true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						}, "user.info", ObjectPath("user"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Path: []string{"user"},
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
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"user":{"name":"Bill","info":{"age":21}}}}`
			}))

			t.Run("null info data", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"user":{"name":"Bill","info":null}}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&EntityFetch{
							FetchDependencies: FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							Input: EntityInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
											SegmentType: StaticSegmentType,
										},
									},
								},
								Item: InputTemplate{
									Segments: []TemplateSegment{
										{
											SegmentType:  VariableSegmentType,
											VariableKind: ResolvableObjectVariableKind,
											Renderer: NewGraphQLVariableResolveRenderer(&Object{
												Fields: []*Field{
													{
														Name: []byte("id"),
														Value: &Integer{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
													{
														Name: []byte("__typename"),
														Value: &String{
															Path: []string{"__typename"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
												},
											}),
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
								SkipErrItem: true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						}, "user.info", ObjectPath("user"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Path: []string{"user"},
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
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"user":{"name":"Bill","info":null}}}`
			}))

			t.Run("wrong type data", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"user":{"name":"Bill","info":{"id":false,"__typename":"Info"}}}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&EntityFetch{
							FetchDependencies: FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							Input: EntityInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
											SegmentType: StaticSegmentType,
										},
									},
								},
								Item: InputTemplate{
									Segments: []TemplateSegment{
										{
											SegmentType:  VariableSegmentType,
											VariableKind: ResolvableObjectVariableKind,
											Renderer: NewGraphQLVariableResolveRenderer(&Object{
												Fields: []*Field{
													{
														Name: []byte("id"),
														Value: &Integer{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
													{
														Name: []byte("__typename"),
														Value: &String{
															Path: []string{"__typename"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
												},
											}),
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
								SkipErrItem: true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						}, "user.info", ObjectPath("user"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Path: []string{"user"},
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
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.info.age'.","path":["user","info","age"]}],"data":{"user":{"name":"Bill","info":null}}}`
			}))

			t.Run("not matching type data", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {
				userService := NewMockDataSource(ctrl)
				userService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
						actual := string(input)
						expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename}}}}"}}`
						assert.Equal(t, expected, actual)
						pair := NewBufPair()
						pair.Data.WriteString(`{"data":{"user":{"name":"Bill","info":{"id":1,"__typename":"Whatever"}}}}`)
						return pair.Data.Bytes(), nil
					})

				infoService := NewMockDataSource(ctrl)
				infoService.EXPECT().
					Load(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)

				return &GraphQLResponse{
					Fetches: Sequence(
						Single(&SingleFetch{
							InputTemplate: InputTemplate{
								Segments: []TemplateSegment{
									{
										Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{ user { name info {id __typename}}}}"}}`),
										SegmentType: StaticSegmentType,
									},
								},
							},
							FetchConfiguration: FetchConfiguration{
								DataSource: userService,
								PostProcessing: PostProcessingConfiguration{
									SelectResponseDataPath: []string{"data"},
								},
							},
						}),
						SingleWithPath(&EntityFetch{
							FetchDependencies: FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							Input: EntityInput{
								Header: InputTemplate{
									Segments: []TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations) { ... on Info { age }}}}}","variables":{"representations":[`),
											SegmentType: StaticSegmentType,
										},
									},
								},
								Item: InputTemplate{
									Segments: []TemplateSegment{
										{
											SegmentType:  VariableSegmentType,
											VariableKind: ResolvableObjectVariableKind,
											Renderer: NewGraphQLVariableResolveRenderer(&Object{
												Fields: []*Field{
													{
														Name: []byte("id"),
														Value: &Integer{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
													{
														Name: []byte("__typename"),
														Value: &String{
															Path: []string{"__typename"},
														},
														OnTypeNames: [][]byte{[]byte("Info")},
													},
												},
											}),
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
								SkipErrItem: true,
							},
							DataSource: infoService,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data", "_entities", "0"},
							},
						}, "user.info", ObjectPath("user"), ObjectPath("info")),
					),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("user"),
								Value: &Object{
									Path: []string{"user"},
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
														OnTypeNames: [][]byte{[]byte("Info")},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"user":{"name":"Bill","info":{}}}}`
			}))
		})
	})

	t.Run("serial fetch", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		user := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {__typename id line1 line2}}}}"}}`,
			`{"data":{"user":{"account":{"address":{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2"}}}}}`)

		addressEnricher := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {country city}}}","variables":{"representations":[{"__typename":"Address","id":"address-1"}]}}}`,
			`{"data":{"__typename":"Address","country":"country-1","city":"city-1"}}`)

		address := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {line3(test: "BOOM") zip}}}","variables":{"representations":[{"__typename":"Address","id":"address-1","country":"country-1","city":"city-1"}]}}}`,
			`{"data":{"__typename": "Address", "line3": "line3-1", "zip": "zip-1"}}`)

		account := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {fullAddress}}}","variables":{"representations":[{"__typename":"Address","id":"address-1","line1":"line1","line2":"line2","line3":"line3-1","zip":"zip-1"}]}}}`,
			`{"data":{"__typename":"Address","fullAddress":"line1 line2 line3-1 city-1 country-1 zip-1"}}`)

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								SegmentType: StaticSegmentType,
								Data:        []byte(`{"method":"POST","url":"http://user.service","body":{"query":"{user {account {address {__typename id line1 line2}}}}"}}`),
							},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: FetchConfiguration{
						DataSource: user,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}, "query"),
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						Input:      `{"method":"POST","url":"http://address-enricher.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {country city}}}","variables":{"representations":$$0$$}}}`,
						DataSource: addressEnricher,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
				}, "query.user.account.address", ObjectPath("user"), ObjectPath("account"), ObjectPath("address")),
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						Input:      `{"method":"POST","url":"http://address.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {line3(test: "BOOM") zip}}}","variables":{"representations":$$0$$}}}`,
						DataSource: address,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
				}, "query.user.account.address", ObjectPath("user"), ObjectPath("account"), ObjectPath("address")),
				SingleWithPath(&SingleFetch{
					FetchConfiguration: FetchConfiguration{
						Input:      `{"method":"POST","url":"http://account.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Address {fullAddress}}}","variables":{"representations":$$0$$}}}`,
						DataSource: account,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
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
				}, "query.user.account.address", ObjectPath("user"), ObjectPath("account"), ObjectPath("address")),
			),
			Data: &Object{
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

	t.Run("nested batching", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		productsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`,
			`{"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"},{"name":"Couch","__typename":"Product","upc":"2"},{"name":"Chair","__typename":"Product","upc":"3"}]}}`)

		reviewsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
			`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}},{"body":"Prefer other Table.","author":{"__typename":"User","id":"2"}}]},{"__typename":"Product","reviews":[{"body":"Couch Too expensive.","author":{"__typename":"User","id":"1"}}]},{"__typename":"Product","reviews":[{"body":"Chair Could be better.","author":{"__typename":"User","id":"2"}}]}]}}`)

		stockService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
			`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`)

		usersService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}}`,
			`{"data":{"_entities":[{"name":"user-1"},{"name":"user-2"}]}}`)

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						DataSource: productsService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}, "query"),
				Parallel(
					SingleWithPath(&BatchEntityFetch{
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
					}, "query.topProducts", ArrayPath("topProducts")),
					SingleWithPath(&BatchEntityFetch{
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
					}, "query.topProducts", ArrayPath("topProducts")),
				),
				SingleWithPath(&BatchEntityFetch{
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
				}, "query.topProducts.reviews.author", ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("topProducts"),
						Value: &Array{
							Path: []string{"topProducts"},
							Item: &Object{
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

	t.Run("nested batching single root result", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		productsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`,
			`{"data":{"topProducts":[{"name":"Table","__typename":"Product","upc":"1"}]}}`)

		reviewsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {__typename id}}}}}","variables":{"representations":[{"__typename":"Product","upc":"1"}]}}}`,
			`{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Love Table!","author":{"__typename":"User","id":"1"}}]}]}}`)

		stockService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://stock","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {stock}}}","variables":{"representations":[{"__typename":"Product","upc":"1"}]}}}`,
			`{"data":{"_entities":[{"stock":8}]}}`)

		usersService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}}`,
			`{"data":{"_entities":[{"name":"user-1"}]}}`)

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://products","body":{"query":"query{topProducts{name __typename upc}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						DataSource: productsService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}, "query"),
				Parallel(
					SingleWithPath(&BatchEntityFetch{
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
					}, "query.topProducts", ArrayPath("topProducts")),
					SingleWithPath(&BatchEntityFetch{
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
					}, "query.topProducts", ArrayPath("topProducts")),
				),
				SingleWithPath(&BatchEntityFetch{
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
				}, "query.topProducts.reviews.author", ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("topProducts"),
						Value: &Array{
							Path: []string{"topProducts"},
							Item: &Object{
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
		}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"topProducts":[{"name":"Table","stock":8,"reviews":[{"body":"Love Table!","author":{"name":"user-1"}}]}]}}`
	}))

	t.Run("nested batching of direct array children", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		accountsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://accounts","body":{"query":"{accounts{__typename ... on User {__typename id} ... on Moderator {__typename moderatorID} ... on Admin {__typename adminID}}}"}}`,
			`{"data":{"accounts":[{"__typename":"User","id":"3"},{"__typename":"Admin","adminID":"2"},{"__typename":"Moderator","moderatorID":"1"}]}}`)

		namesService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://names","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {name} ... on Moderator {subject} ... on Admin {type}}}","variables":{"representations":[{"__typename":"User","id":"3"},{"__typename":"Admin","adminID":"2"},{"__typename":"Moderator","moderatorID":"1"}]}}}`,
			`{"data":{"_entities":[{"__typename":"User","name":"User"},{"__typename":"Admin","type":"super"},{"__typename":"Moderator","subject":"posts"}]}}`)

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://accounts","body":{"query":"{accounts{__typename ... on User {__typename id} ... on Moderator {__typename moderatorID} ... on Admin {__typename adminID}}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						DataSource: accountsService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
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
				}, "query.accounts", ArrayPath("accounts")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("accounts"),
						Value: &Array{
							Path: []string{"accounts"},
							Item: &Object{
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

	t.Run("nested fetch should apply to only single type", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		accountsService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://accounts","body":{"query":"{accounts {__typename ... on User {some {__typename id}} ... on Admin {some {__typename id}}}}"}}`,
			`{"data":{"accounts":[{"__typename":"User","some":{"__typename":"User","id":"1"}},{"__typename":"Admin","some":{"__typename":"User","id":"2"}},{"__typename":"User","some":{"__typename":"User","id":"3"}}]}}`)

		namesService := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://names","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"3"}]}}}`,
			`{"data":{"_entities":[{"__typename":"User","title":"User1"},{"__typename":"User","title":"User3"}]}}`)

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://accounts","body":{"query":"{accounts {__typename ... on User {some {__typename id}} ... on Admin {some {__typename id}}}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						DataSource: accountsService,
						PostProcessing: PostProcessingConfiguration{
							SelectResponseDataPath: []string{"data"},
						},
					},
				}, "query"),
				SingleWithPath(&BatchEntityFetch{
					Input: BatchInput{
						Header: InputTemplate{
							Segments: []TemplateSegment{
								{
									Data:        []byte(`{"method":"POST","url":"http://names","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[`),
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
													OnTypeNames: [][]byte{[]byte("User")},
												},
												{
													Name: []byte("id"),
													Value: &String{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("User")},
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
				}, "accounts.@.some", ArrayPath("accounts"), PathElementWithTypeNames(ObjectPath("some"), []string{"User"})),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("accounts"),
						Value: &Array{
							Path: []string{"accounts"},
							Item: &Object{
								Nullable: false,
								PossibleTypes: map[string]struct{}{
									"Admin": {},
									"User":  {},
								},
								TypeName: "Node",
								Fields: []*Field{
									{
										Name: []byte("some"),
										Value: &Object{
											Path: []string{"some"},
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*Field{
												{
													Name: []byte("title"),
													Value: &String{
														Path: []string{"title"},
													},
												},
											},
										},
										OnTypeNames: [][]byte{[]byte("User")},
									},
									{
										Name: []byte("some"),
										Value: &Object{
											Path: []string{"some"},
											PossibleTypes: map[string]struct{}{
												"User": {},
											},
											TypeName: "User",
											Fields: []*Field{
												{
													Name: []byte("__typename"),
													Value: &String{
														Path:       []string{"__typename"},
														IsTypeName: true,
													},
												},
												{
													Name: []byte("id"),
													Value: &Scalar{
														Path: []string{"id"},
													},
												},
											},
										},
										OnTypeNames: [][]byte{[]byte("Admin")},
									},
								},
							},
						},
					},
				},
			},
		}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"accounts":[{"some":{"title":"User1"}},{"some":{"__typename":"User","id":"2"}},{"some":{"title":"User3"}}]}}`
	}))
}
