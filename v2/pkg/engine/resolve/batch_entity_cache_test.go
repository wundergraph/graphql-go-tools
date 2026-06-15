package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestBatchEntityCacheAllMissThenAllHit(t *testing.T) {
	users := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"},{"__typename":"User","id":"2","name":"Grace"}]}}`),
		},
	}
	cache := newMemoryLoaderCache()
	response := cacheTestBatchEntityResponse(users, entities, batchUserNameCacheConfig(false))
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out1 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)
	out2 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)

	assert.Equal(t, `{"data":{"users":[{"id":"1","name":"Ada"},{"id":"2","name":"Grace"}]}}`, out1)
	assert.Equal(t, out1, out2)
	assert.Equal(t, 2, users.CallCount())
	assert.Equal(t, 1, entities.CallCount())
	assert.Equal(t, map[string]string{
		`{"__typename":"User","key":{"id":"1"}}`: `{"__typename":"User","id":"1","name":"Ada"}`,
		`{"__typename":"User","key":{"id":"2"}}`: `{"__typename":"User","id":"2","name":"Grace"}`,
	}, cache.Snapshot())
}

func TestBatchEntityCachePartialHitFetchesOnlyMissingEntities(t *testing.T) {
	users := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"},{"__typename":"User","id":"3"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"2","name":"Grace"}]}}`),
		},
	}
	cache := newMemoryLoaderCache()
	assert.NoError(t, cache.Set(nil, []*CacheEntry{
		{
			Key:   `{"__typename":"User","key":{"id":"1"}}`,
			Value: []byte(`{"__typename":"User","id":"1","name":"Ada"}`),
		},
		{
			Key:   `{"__typename":"User","key":{"id":"3"}}`,
			Value: []byte(`{"__typename":"User","id":"3","name":"Lin"}`),
		},
	}))
	response := cacheTestBatchEntityResponse(users, entities, batchUserNameCacheConfig(true))
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)

	assert.Equal(t, `{"data":{"users":[{"id":"1","name":"Ada"},{"id":"2","name":"Grace"},{"id":"3","name":"Lin"}]}}`, out)
	assert.Equal(t, 1, entities.CallCount())
	assert.Equal(t, []string{
		`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename id name}}}","variables":{"representations":[{"__typename":"User","id":"2"}]}}}`,
	}, entities.Inputs())
	assert.Equal(t, map[string]string{
		`{"__typename":"User","key":{"id":"1"}}`: `{"__typename":"User","id":"1","name":"Ada"}`,
		`{"__typename":"User","key":{"id":"2"}}`: `{"__typename":"User","id":"2","name":"Grace"}`,
		`{"__typename":"User","key":{"id":"3"}}`: `{"__typename":"User","id":"3","name":"Lin"}`,
	}, cache.Snapshot())
}

func TestBatchEntityCacheMultiCandidateMerge(t *testing.T) {
	users := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"},{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"},{"__typename":"User","id":"2","name":"Grace"}]}}`),
		},
	}
	cache := newMemoryLoaderCache()
	response := cacheTestBatchEntityResponse(users, entities, batchUserNameCacheConfig(false))
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out1 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)
	out2 := resolveCacheTestGraphQLResponse(t, response, options, enableL2Cache)

	assert.Equal(t, `{"data":{"users":[{"id":"1","name":"Ada"},{"id":"1","name":"Ada"},{"id":"2","name":"Grace"}]}}`, out1)
	assert.Equal(t, out1, out2)
	assert.Equal(t, []string{
		`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename id name}}}","variables":{"representations":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}}`,
	}, entities.Inputs())
	assert.Equal(t, map[string]string{
		`{"__typename":"User","key":{"id":"1"}}`: `{"__typename":"User","id":"1","name":"Ada"}`,
		`{"__typename":"User","key":{"id":"2"}}`: `{"__typename":"User","id":"2","name":"Grace"}`,
	}, cache.Snapshot())
}

func TestBatchEntityCacheL2DisabledBehavesAsNormalBatchLoad(t *testing.T) {
	users := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"},{"__typename":"User","id":"2","name":"Grace"}]}}`),
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"},{"__typename":"User","id":"2","name":"Grace"}]}}`),
		},
	}
	cache := newMemoryLoaderCache()
	response := cacheTestBatchEntityResponse(users, entities, batchUserNameCacheConfig(false))
	options := ResolverOptions{
		Caches: map[string]LoaderCache{
			"default": cache,
		},
	}

	out1 := resolveCacheTestGraphQLResponse(t, response, options, nil)
	out2 := resolveCacheTestGraphQLResponse(t, response, options, nil)

	assert.Equal(t, `{"data":{"users":[{"id":"1","name":"Ada"},{"id":"2","name":"Grace"}]}}`, out1)
	assert.Equal(t, out1, out2)
	assert.Equal(t, 2, entities.CallCount())
	assert.Equal(t, map[string]string{}, cache.Snapshot())
}

func enableL2Cache(ctx *Context) {
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
}

func batchUserNameCacheConfig(enablePartial bool) *FetchCacheConfiguration {
	config := userNameCacheConfig(false)
	config.CacheName = "default"
	config.EnableL2Cache = true
	config.EnablePartialCacheLoad = enablePartial
	config.TTL = time.Minute
	return config
}

func cacheTestBatchEntityResponse(rootSource DataSource, entitySource DataSource, cache *FetchCacheConfiguration) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"query{users{__typename id}}"}}`),
				FetchConfiguration: FetchConfiguration{
					DataSource: rootSource,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
			SingleWithPath(cacheTestBatchEntityFetch(entitySource, cache), "query.users", ArrayPath("users")),
		),
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("users"),
					Value: &Array{
						Path: []string{"users"},
						Item: &Object{
							Fields: []*Field{
								{
									Name:  []byte("id"),
									Value: &String{Path: []string{"id"}},
								},
								{
									Name:  []byte("name"),
									Value: &String{Path: []string{"name"}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func cacheTestBatchEntityFetch(source DataSource, cache *FetchCacheConfiguration) *BatchEntityFetch {
	return &BatchEntityFetch{
		Input: BatchInput{
			Header: cacheTestStaticInput(`{"method":"POST","url":"http://users","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename id name}}}","variables":{"representations":[`),
			Items: []InputTemplate{
				{
					Segments: []TemplateSegment{
						{
							SegmentType:  VariableSegmentType,
							VariableKind: ResolvableObjectVariableKind,
							Renderer: NewGraphQLVariableResolveRenderer(&Object{
								Fields: []*Field{
									{
										Name:  []byte("__typename"),
										Value: &String{Path: []string{"__typename"}},
									},
									{
										Name:  []byte("id"),
										Value: &String{Path: []string{"id"}},
									},
								},
							}),
						},
					},
				},
			},
			Separator: cacheTestStaticInput(`,`),
			Footer:    cacheTestStaticInput(`]}}}`),
		},
		DataSource: source,
		PostProcessing: PostProcessingConfiguration{
			SelectResponseDataPath: []string{"data", "_entities"},
		},
		Cache: cache,
		Info: &FetchInfo{
			DataSourceID:   "users",
			DataSourceName: "users",
			OperationType:  ast.OperationTypeQuery,
		},
	}
}
