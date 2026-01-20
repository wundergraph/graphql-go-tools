package resolve

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
)

func TestResolveCaching(t *testing.T) {
	t.Run("nested batching single root result", testFn(func(t *testing.T, ctrl *gomock.Controller) (node *GraphQLResponse, ctx Context, expectedOutput string) {

		listingRoot := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://listing","body":{"query":"query{listing{__typename id name}}"}}`,
			`{"data":{"listing":{"__typename":"Listing","id":1,"name":"L1"}}}`)

		nested := mockedDS(t, ctrl,
			`{"method":"POST","url":"http://nested","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Listing { nested { id price listing { __typename id }} }}}","variables":{"representations":[{"__typename":"Listing","id":1}]}}}`,
			`{"data":{"_entities":[{"__typename":"Listing","nested":{"id":1.1,"price":123,"listing":{"__typename":"Listing","id":1}}}]}}`)

		return &GraphQLResponse{
			Fetches: Sequence(
				SingleWithPath(&SingleFetch{
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://listing","body":{"query":"query{listing{__typename id name}}"}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					FetchConfiguration: FetchConfiguration{
						DataSource: listingRoot,
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
									Data:        []byte(`{"method":"POST","url":"http://nested","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Listing { nested { id price listing { __typename id }} }}}","variables":{"representations":[`),
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
													Value: &Integer{
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
					DataSource: nested,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities"},
					},
				}, "query.listing", ObjectPath("listing")),
			),
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("listing"),
						Value: &Object{
							Path: []string{"listing"},
							Fields: []*Field{
								{
									Name: []byte("id"),
									Value: &Integer{
										Path:     []string{"id"},
										Nullable: false,
									},
								},
								{
									Name: []byte("name"),
									Value: &String{
										Path:     []string{"name"},
										Nullable: false,
									},
								},
								{
									Name: []byte("nested"),
									Value: &Object{
										Path: []string{"nested"},
										Fields: []*Field{
											{
												Name: []byte("id"),
												Value: &Float{
													Path: []string{"id"},
												},
											},
											{
												Name: []byte("price"),
												Value: &Integer{
													Path: []string{"price"},
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
		}, Context{ctx: context.Background(), Variables: nil}, `{"data":{"listing":{"id":1,"name":"L1","nested":{"id":1.1,"price":123}}}}`
	}))
}
