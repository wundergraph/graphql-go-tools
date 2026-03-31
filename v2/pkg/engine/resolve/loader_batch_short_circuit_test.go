package resolve

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

func TestLoader_BatchEntityKeyEmptyListShortCircuit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ds := NewMockDataSource(ctrl)
	ds.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Path: []string{"products"},
						Item: &Object{
							Fields: []*Field{
								{
									Name:  []byte("upc"),
									Value: &String{Path: []string{"upc"}},
								},
							},
						},
					},
				},
			},
		},
		Fetches: Sequence(
			Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: ds,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
					Caching: FetchCacheConfiguration{
						BatchEntityKeyArgumentPathHint: []string{"upcs"},
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://products"}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				Info: &FetchInfo{
					DataSourceName: "products",
					OperationType:  ast.OperationTypeQuery,
					RootFields: []GraphCoordinate{
						{TypeName: "Query", FieldName: "products"},
					},
				},
			}),
		),
	}

	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParse(`{"upcs":[]}`)

	resolvable := NewResolvable(nil, ResolvableOptions{})
	loader := &Loader{}

	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	assert.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	assert.NoError(t, err)

	assert.Equal(t, `{"data":{"products":[]}}`, fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors))
}
