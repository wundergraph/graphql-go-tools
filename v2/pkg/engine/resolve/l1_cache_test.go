package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoaderCacheL1DeduplicatesSameEntityWithinRequest(t *testing.T) {
	userEntities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]}}`),
		},
	}
	response := cacheTestL1Response(userEntities, userNameCacheConfig(true))
	out := resolveCacheTestGraphQLResponse(t, response, ResolverOptions{}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
	})

	assert.Equal(t, `{"data":{"first":{"id":"1","name":"Ada"},"second":{"id":"1","name":"Ada"}}}`, out)
	assert.Equal(t, 1, userEntities.CallCount())
}

func TestLoaderCacheL1FieldWideningGuardMissesNarrowEntry(t *testing.T) {
	userEntities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]}}`),
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada","email":"ada@example.com"}]}}`),
		},
	}
	response := cacheTestL1WideningResponse(userEntities)
	out := resolveCacheTestGraphQLResponse(t, response, ResolverOptions{}, func(ctx *Context) {
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
	})

	assert.Equal(t, `{"data":{"first":{"id":"1","name":"Ada"},"second":{"id":"1","name":"Ada","email":"ada@example.com"}}}`, out)
	assert.Equal(t, 2, userEntities.CallCount())
}

func TestLoaderCacheL1DisabledDoesNotCache(t *testing.T) {
	t.Run("request flag disabled", func(t *testing.T) {
		userEntities := &countingCacheTestDataSource{
			responses: [][]byte{
				[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]}}`),
				[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]}}`),
			},
		}
		response := cacheTestL1Response(userEntities, userNameCacheConfig(true))
		out := resolveCacheTestGraphQLResponse(t, response, ResolverOptions{}, nil)

		assert.Equal(t, `{"data":{"first":{"id":"1","name":"Ada"},"second":{"id":"1","name":"Ada"}}}`, out)
		assert.Equal(t, 2, userEntities.CallCount())
	})

	t.Run("fetch flag disabled", func(t *testing.T) {
		userEntities := &countingCacheTestDataSource{
			responses: [][]byte{
				[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]}}`),
				[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"}]}}`),
			},
		}
		response := cacheTestL1Response(userEntities, userNameCacheConfig(false))
		out := resolveCacheTestGraphQLResponse(t, response, ResolverOptions{}, func(ctx *Context) {
			ctx.ExecutionOptions.Caching.EnableL1Cache = true
		})

		assert.Equal(t, `{"data":{"first":{"id":"1","name":"Ada"},"second":{"id":"1","name":"Ada"}}}`, out)
		assert.Equal(t, 2, userEntities.CallCount())
	})
}

func cacheTestL1WideningResponse(source DataSource) *GraphQLResponse {
	narrow := userNameCacheConfig(true)
	wide := userNameEmailCacheConfig()
	response := cacheTestL1Response(source, narrow)
	response.Fetches.ChildNodes[2].Item.Fetch = cacheTestEntityFetch(source, wide)
	response.Data.Fields[1].Value.(*Object).Fields = append(response.Data.Fields[1].Value.(*Object).Fields, &Field{
		Name:  []byte("email"),
		Value: &String{Path: []string{"email"}},
	})
	return response
}

func userNameEmailCacheConfig() *FetchCacheConfiguration {
	config := userNameCacheConfig(true)
	config.ProvidesData = &Object{
		Fields: []*Field{
			{
				Name:  []byte("__typename"),
				Value: &String{},
			},
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
	return config
}
