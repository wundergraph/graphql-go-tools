package resolve

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestNegativeCachingResolveRegression_PreservesParentObjectForNullableField(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := NewFakeLoaderCache()

	// The root fetch discovers the Product identity and creates the parent object that the
	// entity fetch will later extend. It does not provide `name`.
	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"product":{"__typename":"Product","id":"prod-1"}}}`), nil
		}).Times(1)

	// The entity fetch comes back as `null`, which triggers negative caching for this Product key.
	// The regression here was that resolve could lose the already-built parent object and return
	// `product: null` instead of preserving `product.id` and filling the nullable child as `null`.
	entityDS := NewMockDataSource(ctrl)
	entityDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, headers any, input []byte) ([]byte, error) {
			return []byte(`{"data":{"_entities":[null]}}`), nil
		}).Times(1)

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: rootDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{{
						Data:        []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{product {__typename id}}"}}`),
						SegmentType: StaticSegmentType,
					}},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query"),
			SingleWithPath(&SingleFetch{
				// This entity fetch asks only for the nullable `name` field. Negative caching is enabled
				// so the resolver has to merge a negative-cache result back into the existing `product` object.
				FetchConfiguration: FetchConfiguration{
					DataSource: entityDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities", "0"},
					},
					Caching: FetchCacheConfiguration{
						Enabled:          true,
						CacheName:        "default",
						TTL:              30 * time.Second,
						CacheKeyTemplate: newProductCacheKeyTemplate(),
						NegativeCacheTTL: 10 * time.Second,
					},
				},
				InputTemplate: InputTemplate{Segments: newNegativeCacheEntitySegments()},
				Info: &FetchInfo{
					DataSourceID:   "products",
					DataSourceName: "products",
					OperationType:  ast.OperationTypeQuery,
					ProvidesData: &Object{Fields: []*Field{{
						Name:  []byte("name"),
						Value: &String{Path: []string{"name"}, Nullable: true},
					}}},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query.product", ObjectPath("product")),
		),
		Data: &Object{Fields: []*Field{{
			Name: []byte("product"),
			Value: &Object{
				Path:     []string{"product"},
				Nullable: true,
				Fields: []*Field{
					{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: false}},
					// `name` is nullable, so a negative-cache hit should materialize it as `null`
					// while still preserving the parent object and its non-null `id`.
					{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: true}},
				},
			},
		}}},
	}

	loader := &Loader{caches: map[string]LoaderCache{"default": cache}}
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = true

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	err = resolvable.Resolve(context.Background(), response.Data, response.Fetches, buf)
	require.NoError(t, err)
	// The parent object must survive the negative entity result. The regression would have
	// dropped the object entirely instead of returning the already-known `id` plus `name: null`.
	assert.Equal(t, `{"data":{"product":{"id":"prod-1","name":null}}}`, buf.String())
}
