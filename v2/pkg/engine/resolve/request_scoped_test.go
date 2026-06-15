package resolve

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestRequestScopedTryInjection(t *testing.T) {
	t.Run("no hints is a no-op", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, true)
		defer release()
		item := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)

		injected := loader.tryRequestScopedInjection(&FetchCacheConfiguration{}, []*astjson.Value{item}, &result{})

		assert.Equal(t, false, injected)
		assert.Equal(t, `{"id":"a1"}`, string(item.MarshalTo(nil)))
	})

	t.Run("missing key leaves items untouched", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, true)
		defer release()
		item := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)

		injected := loader.tryRequestScopedInjection(requestScopedViewerCache("accounts.viewer", requestScopedViewerProvidesData()), []*astjson.Value{item}, &result{})

		assert.Equal(t, false, injected)
		assert.Equal(t, `{"id":"a1"}`, string(item.MarshalTo(nil)))
	})

	t.Run("widening rejects narrow cached values", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, true)
		defer release()
		loader.requestScopedL1["accounts.viewer"] = parseLoaderCacheTransformTestValue(t, loader, `{"id":"v1","name":"Ada"}`)
		item := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)

		injected := loader.tryRequestScopedInjection(requestScopedViewerCache("accounts.viewer", requestScopedViewerProvidesData()), []*astjson.Value{item}, &result{})

		assert.Equal(t, false, injected)
		assert.Equal(t, `{"id":"a1"}`, string(item.MarshalTo(nil)))
	})

	t.Run("all hints must pass before any item is mutated", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, true)
		defer release()
		loader.requestScopedL1["accounts.viewer"] = parseLoaderCacheTransformTestValue(t, loader, `{"id":"v1","name":"Ada","email":"ada@example.com"}`)
		item := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)
		cache := &FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				requestScopedField("currentViewer", "accounts.viewer", requestScopedViewerProvidesData()),
				requestScopedField("viewerProfile", "accounts.profile", requestScopedProfileProvidesData()),
			},
		}

		injected := loader.tryRequestScopedInjection(cache, []*astjson.Value{item}, &result{})

		assert.Equal(t, false, injected)
		assert.Equal(t, `{"id":"a1"}`, string(item.MarshalTo(nil)))
	})

	t.Run("l1 flag gates injection", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, false)
		defer release()
		loader.requestScopedL1["accounts.viewer"] = parseLoaderCacheTransformTestValue(t, loader, `{"id":"v1","name":"Ada","email":"ada@example.com"}`)
		item := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)

		injected := loader.tryRequestScopedInjection(requestScopedViewerCache("accounts.viewer", requestScopedViewerProvidesData()), []*astjson.Value{item}, &result{})

		assert.Equal(t, false, injected)
		assert.Equal(t, `{"id":"a1"}`, string(item.MarshalTo(nil)))
	})

	t.Run("nil provides data fails closed", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, true)
		defer release()
		loader.requestScopedL1["accounts.viewer"] = parseLoaderCacheTransformTestValue(t, loader, `{"id":"v1"}`)
		item := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)

		injected := loader.tryRequestScopedInjection(requestScopedViewerCache("accounts.viewer", nil), []*astjson.Value{item}, &result{})

		assert.Equal(t, false, injected)
		assert.Equal(t, `{"id":"a1"}`, string(item.MarshalTo(nil)))
	})
}

func TestRequestScopedInjectionCopiesAndAliases(t *testing.T) {
	loader, release := newRequestScopedTestLoader(t, true)
	defer release()
	loader.requestScopedL1["accounts.viewer"] = parseLoaderCacheTransformTestValue(t, loader, `{"id":"v1","name":"Ada","email":"ada@example.com"}`)
	first := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)
	second := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a2"}`)

	injected := loader.tryRequestScopedInjection(requestScopedViewerCache("accounts.viewer", requestScopedAliasedViewerProvidesData()), []*astjson.Value{first, second}, &result{})
	require.Equal(t, true, injected)

	first.Get("currentViewer").Set(loader.jsonArena, "displayName", astjson.StringValue(loader.jsonArena, "Grace"))

	assert.Equal(t, `{"id":"a1","currentViewer":{"viewerID":"v1","displayName":"Grace","email":"ada@example.com"}}`, string(first.MarshalTo(nil)))
	assert.Equal(t, `{"id":"a2","currentViewer":{"viewerID":"v1","displayName":"Ada","email":"ada@example.com"}}`, string(second.MarshalTo(nil)))
	assert.Equal(t, `{"id":"v1","name":"Ada","email":"ada@example.com"}`, string(loader.requestScopedL1["accounts.viewer"].MarshalTo(nil)))
}

func TestRequestScopedInjectionSyntheticAlias(t *testing.T) {
	loader, release := newRequestScopedTestLoader(t, true)
	defer release()
	loader.requestScopedL1["accounts.viewer"] = parseLoaderCacheTransformTestValue(t, loader, `{"id":"v1","name":"Ada"}`)
	item := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)

	injected := loader.tryRequestScopedInjection(requestScopedViewerCache("accounts.viewer", requestScopedSyntheticAliasProvidesData()), []*astjson.Value{item}, &result{})
	require.Equal(t, true, injected)

	assert.Equal(t, `{"id":"a1","currentViewer":{"id":"v1","__request_scoped_0":"Ada"}}`, string(item.MarshalTo(nil)))
}

func TestRequestScopedExport(t *testing.T) {
	t.Run("copy on export normalizes aliases", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, true)
		defer release()
		item := parseLoaderCacheTransformTestValue(t, loader, `{"viewerAlias":{"viewerID":"v1","displayName":"Ada","email":"ada@example.com"}}`)
		cache := &FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "viewerAlias",
					FieldPath:    []string{"viewerAlias"},
					L1Key:        "accounts.viewer",
					ProvidesData: requestScopedAliasedViewerProvidesData(),
				},
			},
		}

		loader.exportRequestScopedFields(cache, []*astjson.Value{item})
		item.Get("viewerAlias").Set(loader.jsonArena, "displayName", astjson.StringValue(loader.jsonArena, "Grace"))

		assert.Equal(t, `{"id":"v1","name":"Ada","email":"ada@example.com"}`, string(loader.requestScopedL1["accounts.viewer"].MarshalTo(nil)))
		assert.Equal(t, `{"viewerAlias":{"viewerID":"v1","displayName":"Grace","email":"ada@example.com"}}`, string(item.MarshalTo(nil)))
	})

	t.Run("existing entries are merged through an independent working copy", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, true)
		defer release()
		loader.requestScopedL1["accounts.viewer"] = parseLoaderCacheTransformTestValue(t, loader, `{"id":"v1","profile":{"name":"Ada"}}`)
		item := parseLoaderCacheTransformTestValue(t, loader, `{"currentViewer":{"profile":{"email":"ada@example.com"}}}`)
		cache := &FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				requestScopedField("currentViewer", "accounts.viewer", requestScopedProfileEmailProvidesData()),
			},
		}

		loader.exportRequestScopedFields(cache, []*astjson.Value{item})
		item.Get("currentViewer", "profile").Set(loader.jsonArena, "email", astjson.StringValue(loader.jsonArena, "grace@example.com"))

		assert.Equal(t, `{"id":"v1","profile":{"name":"Ada","email":"ada@example.com"}}`, string(loader.requestScopedL1["accounts.viewer"].MarshalTo(nil)))
		assert.Equal(t, `{"currentViewer":{"profile":{"email":"grace@example.com"}}}`, string(item.MarshalTo(nil)))
	})

	t.Run("l1 flag gates export", func(t *testing.T) {
		loader, release := newRequestScopedTestLoader(t, false)
		defer release()
		item := parseLoaderCacheTransformTestValue(t, loader, `{"currentViewer":{"id":"v1","name":"Ada","email":"ada@example.com"}}`)

		loader.exportRequestScopedFields(requestScopedViewerCache("accounts.viewer", requestScopedViewerProvidesData()), []*astjson.Value{item})

		assert.Equal(t, 0, len(loader.requestScopedL1))
	})
}

func TestRequestScopedRoundTripSkipsSecondFetch(t *testing.T) {
	source := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"currentViewer":{"id":"v1","name":"Ada","email":"ada@example.com"},"article":{"__typename":"Article","id":"a1"}}}`),
			[]byte(`{"data":{"_entities":[{"currentViewer":{"id":"unexpected","name":"Unexpected","email":"unexpected@example.com"}}]}}`),
		},
	}
	response := requestScopedRoundTripResponse(source)
	out := resolveCacheTestGraphQLResponse(t, response, ResolverOptions{}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
	})

	assert.Equal(t, `{"data":{"currentViewer":{"id":"v1","name":"Ada","email":"ada@example.com"},"article":{"id":"a1","currentViewer":{"id":"v1","name":"Ada","email":"ada@example.com"}}}}`, out)
	assert.Equal(t, 1, source.CallCount())
	assert.Equal(t, []string{
		`{"method":"POST","url":"http://accounts","body":{"query":"query{currentViewer{id name email} article{id __typename}}"}}`,
	}, source.Inputs())
}

func TestRequestScopedGCSurvival(t *testing.T) {
	loader, release := newRequestScopedTestLoader(t, true)
	defer release()
	item := parseLoaderCacheTransformTestValue(t, loader, `{"currentViewer":{"id":"v1","name":"Ada","email":"ada@example.com"}}`)
	loader.exportRequestScopedFields(requestScopedViewerCache("accounts.viewer", requestScopedViewerProvidesData()), []*astjson.Value{item})

	runtime.GC()
	injectedItem := parseLoaderCacheTransformTestValue(t, loader, `{"id":"a1"}`)
	injected := loader.tryRequestScopedInjection(requestScopedViewerCache("accounts.viewer", requestScopedViewerProvidesData()), []*astjson.Value{injectedItem}, &result{})
	require.Equal(t, true, injected)
	runtime.GC()

	assert.Equal(t, `{"id":"v1","name":"Ada","email":"ada@example.com"}`, string(loader.requestScopedL1["accounts.viewer"].MarshalTo(nil)))
	assert.Equal(t, `{"id":"a1","currentViewer":{"id":"v1","name":"Ada","email":"ada@example.com"}}`, string(injectedItem.MarshalTo(nil)))
}

func newRequestScopedTestLoader(t *testing.T, enableL1 bool) (*Loader, func()) {
	t.Helper()

	loader, release := newLoaderCacheTransformTestLoader()
	loader.ctx = NewContext(context.Background())
	loader.ctx.ExecutionOptions.Caching.EnableL1Cache = enableL1
	loader.requestScopedL1 = map[string]*astjson.Value{}
	return loader, release
}

func requestScopedViewerCache(key string, provides *Object) *FetchCacheConfiguration {
	return &FetchCacheConfiguration{
		RequestScopedFields: []RequestScopedField{
			requestScopedField("currentViewer", key, provides),
		},
	}
}

func requestScopedField(fieldName, key string, provides *Object) RequestScopedField {
	return RequestScopedField{
		FieldName:    fieldName,
		FieldPath:    []string{fieldName},
		L1Key:        key,
		ProvidesData: provides,
	}
}

func requestScopedViewerProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name:  []byte("id"),
				Value: &String{},
			},
			{
				Name:  []byte("name"),
				Value: &String{},
			},
			{
				Name:  []byte("email"),
				Value: &String{},
			},
		},
	}
}

func requestScopedAliasedViewerProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name:         []byte("viewerID"),
				OriginalName: []byte("id"),
				Value:        &String{},
			},
			{
				Name:         []byte("displayName"),
				OriginalName: []byte("name"),
				Value:        &String{},
			},
			{
				Name:  []byte("email"),
				Value: &String{},
			},
		},
	}
}

func requestScopedSyntheticAliasProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name:  []byte("id"),
				Value: &String{},
			},
			{
				Name:         []byte("__request_scoped_0"),
				OriginalName: []byte("name"),
				Value:        &String{},
			},
		},
	}
}

func requestScopedProfileProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name: []byte("profile"),
				Value: &Object{
					Fields: []*Field{
						{
							Name:  []byte("name"),
							Value: &String{},
						},
					},
				},
			},
		},
	}
}

func requestScopedProfileEmailProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name: []byte("profile"),
				Value: &Object{
					Fields: []*Field{
						{
							Name:  []byte("email"),
							Value: &String{},
						},
					},
				},
			},
		},
	}
}

func requestScopedRoundTripResponse(source DataSource) *GraphQLResponse {
	return &GraphQLResponse{
		Fetches: Sequence(
			Single(&SingleFetch{
				InputTemplate: cacheTestStaticInput(`{"method":"POST","url":"http://accounts","body":{"query":"query{currentViewer{id name email} article{id __typename}}"}}`),
				FetchConfiguration: FetchConfiguration{
					DataSource: source,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Cache: requestScopedViewerCache("accounts.viewer", requestScopedViewerProvidesData()),
				Info: &FetchInfo{
					DataSourceID:   "accounts",
					DataSourceName: "accounts",
					OperationType:  ast.OperationTypeQuery,
				},
			}),
			SingleWithPath(requestScopedArticleViewerFetch(source), "query.article", ObjectPath("article")),
		),
		Info: &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: &Object{
			Fields: []*Field{
				{
					Name:  []byte("currentViewer"),
					Value: requestScopedViewerResponseObject("currentViewer"),
				},
				{
					Name: []byte("article"),
					Value: &Object{
						Path: []string{"article"},
						Fields: []*Field{
							{
								Name:  []byte("id"),
								Value: &String{Path: []string{"id"}},
							},
							{
								Name:  []byte("currentViewer"),
								Value: requestScopedViewerResponseObject("currentViewer"),
							},
						},
					},
				},
			},
		},
	}
}

func requestScopedArticleViewerFetch(source DataSource) *EntityFetch {
	return &EntityFetch{
		Input: EntityInput{
			Header: cacheTestStaticInput(`{"method":"POST","url":"http://accounts","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Article {currentViewer{id name email}}}}","variables":{"representations":[`),
			Item: InputTemplate{
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
			Footer:      cacheTestStaticInput(`]}}}`),
			SkipErrItem: true,
		},
		DataSource: source,
		PostProcessing: PostProcessingConfiguration{
			SelectResponseDataPath: []string{"data", "_entities", "0"},
		},
		Cache: requestScopedViewerCache("accounts.viewer", requestScopedViewerProvidesData()),
		Info: &FetchInfo{
			DataSourceID:   "accounts",
			DataSourceName: "accounts",
			OperationType:  ast.OperationTypeQuery,
		},
	}
}

func requestScopedViewerResponseObject(path string) *Object {
	return &Object{
		Path: []string{path},
		Fields: []*Field{
			{
				Name:  []byte("id"),
				Value: &String{Path: []string{"id"}},
			},
			{
				Name:  []byte("name"),
				Value: &String{Path: []string{"name"}},
			},
			{
				Name:  []byte("email"),
				Value: &String{Path: []string{"email"}},
			},
		},
	}
}
