package resolve

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// What the root fetch cache may be asked to key is narrower than what the
// loader sees: one case per condition that narrows it.
func TestRootFetchCacheable(t *testing.T) {
	rootItem := func() *FetchItem { return &FetchItem{} }

	queryFetch := func() *SingleFetch {
		return &SingleFetch{
			FetchConfiguration: FetchConfiguration{
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath:   []string{"data"},
					SelectResponseErrorsPath: []string{"errors"},
				},
			},
			DataSourceIdentifier: []byte("graphql_datasource.Source"),
			Info:                 &FetchInfo{OperationType: ast.OperationTypeQuery},
		}
	}

	t.Run("a root query fetch against a subgraph is cacheable", func(t *testing.T) {
		require.True(t, rootFetchCacheable(rootItem(), queryFetch()))
	})

	t.Run("a nested fetch is not", func(t *testing.T) {
		// Its input carries parent data, which is a different thing to key on
		// than a request the variables determine on their own.
		nested := &FetchItem{FetchPath: []FetchItemPathElement{{Path: []string{"topProducts"}}}}
		require.False(t, rootFetchCacheable(nested, queryFetch()))
	})

	t.Run("a mutation is not", func(t *testing.T) {
		fetch := queryFetch()
		fetch.Info.OperationType = ast.OperationTypeMutation
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})

	t.Run("a subscription is not", func(t *testing.T) {
		fetch := queryFetch()
		fetch.Info.OperationType = ast.OperationTypeSubscription
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})

	t.Run("a fetch answered by anything but a GraphQL subgraph is not", func(t *testing.T) {
		// Introspection, pubsub and gRPC plugins answer without a Cache-Control
		// header, so a lookup for them could only ever cost a round trip.
		fetch := queryFetch()
		fetch.DataSourceIdentifier = []byte("introspection_datasource.Source")
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})

	t.Run("a fetch that reads its data from anywhere but data is not", func(t *testing.T) {
		// A hit rebuilds {"data":...} around the stored bytes, which is only the
		// response this fetch's merge would read back.
		fetch := queryFetch()
		fetch.PostProcessing.SelectResponseDataPath = []string{"data", "_entities"}
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})

	t.Run("a fetch with no info is not", func(t *testing.T) {
		fetch := queryFetch()
		fetch.Info = nil
		require.False(t, rootFetchCacheable(rootItem(), fetch))
	})
}

// What a subgraph asserts about its own entries decides what an invalidation
// would later remove, so the parser's job is to be unambiguous about what it
// accepts and to keep what it accepts aligned with the values it belongs to.
func TestResponseCacheTags(t *testing.T) {
	parse := func(t *testing.T, doc string) *astjson.Value {
		t.Helper()
		value, err := astjson.Parse(doc)
		require.NoError(t, err)
		return value
	}

	t.Run("one tag list per value, in the order the values came in", func(t *testing.T) {
		response := parse(t, `{"data":{"_entities":[{},{},{}]},"extensions":{"apolloEntityCacheTags":[
			["users","user-42"],
			["users","user-1023"],
			["users","user-7"]
		]}}`)
		require.Equal(t, [][]string{
			{"users", "user-42"},
			{"users", "user-1023"},
			{"users", "user-7"},
		}, responseCacheTags(response, 3, false))
	})

	t.Run("a root fetch declares one flat list under its own key", func(t *testing.T) {
		response := parse(t, `{"extensions":{"apolloCacheTags":["users","homepage"]}}`)
		require.Equal(t, [][]string{{"users", "homepage"}}, responseCacheTags(response, 1, true))
	})

	t.Run("the two extension keys are not interchangeable", func(t *testing.T) {
		// A root fetch caches one entry and an entity fetch one per entity, so
		// each key carries the shape its own fetch needs. Reading either from
		// the other's key would make the same document mean different things
		// depending on a count the subgraph author cannot see.
		entity := parse(t, `{"extensions":{"apolloEntityCacheTags":[["users","homepage"]]}}`)
		require.Empty(t, responseCacheTags(entity, 1, true))

		root := parse(t, `{"extensions":{"apolloCacheTags":["users","homepage"]}}`)
		require.Empty(t, responseCacheTags(root, 1, false))
	})

	t.Run("a flat list is not shorthand for an entity fetch", func(t *testing.T) {
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":["users","homepage"]}}`)
		require.Equal(t, [][]string{nil, nil}, responseCacheTags(response, 2, false))
	})

	t.Run("a root fetch with nothing usable in its list gets no tags", func(t *testing.T) {
		require.Empty(t, responseCacheTags(parse(t, `{"extensions":{"apolloCacheTags":[]}}`), 1, true))
		require.Empty(t, responseCacheTags(parse(t, `{"extensions":{"apolloCacheTags":[""]}}`), 1, true))
		require.Empty(t, responseCacheTags(parse(t, `{"extensions":{"apolloCacheTags":null}}`), 1, true))
		require.Empty(t, responseCacheTags(parse(t, `{"extensions":{}}`), 1, true))
	})

	t.Run("nothing at all is not an error", func(t *testing.T) {
		require.Empty(t, responseCacheTags(parse(t, `{"data":{"_entities":[{}]}}`), 1, false))
		require.Empty(t, responseCacheTags(parse(t, `{"extensions":{}}`), 1, false))
		require.Empty(t, responseCacheTags(parse(t, `{"extensions":{"apolloEntityCacheTags":null}}`), 1, false))
	})

	t.Run("too many tag lists for the values discards all of them", func(t *testing.T) {
		// Zipping as far as the shorter of the two would tag the first entries
		// correctly and say nothing about which of the rest went astray.
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":[["a"],["b"],["c"]]}}`)
		require.Empty(t, responseCacheTags(response, 2, false))
	})

	t.Run("too few tag lists for the values discards all of them", func(t *testing.T) {
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":[["a"]]}}`)
		require.Empty(t, responseCacheTags(response, 3, false))
	})

	t.Run("no values means there is nothing for tags to belong to", func(t *testing.T) {
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":[["a"]]}}`)
		require.Empty(t, responseCacheTags(response, 0, false))
	})

	t.Run("one malformed element costs that value its tags and no other", func(t *testing.T) {
		// The list is still positional, so the elements either side are known to
		// belong where they sit.
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":[["a"],"not-a-list",["c"]]}}`)
		require.Equal(t, [][]string{{"a"}, nil, {"c"}}, responseCacheTags(response, 3, false))
	})

	t.Run("non string tags are skipped, the rest of the list survives", func(t *testing.T) {
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":[["a",7,null,{"x":1},"b"]]}}`)
		require.Equal(t, [][]string{{"a", "b"}}, responseCacheTags(response, 1, false))
	})

	t.Run("an empty tag is dropped", func(t *testing.T) {
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":[["","a"]]}}`)
		require.Equal(t, [][]string{{"a"}}, responseCacheTags(response, 1, false))
	})

	t.Run("a value whose tags are all unusable gets none", func(t *testing.T) {
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":[[""],["a"]]}}`)
		require.Equal(t, [][]string{nil, {"a"}}, responseCacheTags(response, 2, false))
	})

	t.Run("an over long tag is dropped, its neighbours are not", func(t *testing.T) {
		long := strings.Repeat("x", maxResponseCacheTagLength+1)
		atLimit := strings.Repeat("y", maxResponseCacheTagLength)
		response := parse(t, fmt.Sprintf(
			`{"extensions":{"apolloEntityCacheTags":[["a",%q,%q]]}}`, long, atLimit))
		require.Equal(t, [][]string{{"a", atLimit}}, responseCacheTags(response, 1, false))
	})

	tagList := func(n int) string {
		tags := make([]string, 0, n)
		for i := range n {
			tags = append(tags, fmt.Sprintf("%q", fmt.Sprintf("tag-%d", i)))
		}
		return strings.Join(tags, ",")
	}

	t.Run("a value at maxResponseCacheTagsPerValue tags is kept whole", func(t *testing.T) {
		response := parse(t, fmt.Sprintf(
			`{"extensions":{"apolloEntityCacheTags":[[%s]]}}`, tagList(maxResponseCacheTagsPerValue)))

		got := responseCacheTags(response, 1, false)
		require.Len(t, got, 1)
		require.Len(t, got[0], maxResponseCacheTagsPerValue)
	})

	t.Run("a value over the cap is rejected, not truncated", func(t *testing.T) {
		// Truncating would leave the entry uninvalidatable by the tags dropped,
		// with nothing saying so.
		response := parse(t, fmt.Sprintf(
			`{"extensions":{"apolloEntityCacheTags":[[%s]]}}`, tagList(maxResponseCacheTagsPerValue+1)))

		require.Equal(t, [][]string{nil}, responseCacheTags(response, 1, false))
	})

	t.Run("one value over the cap costs that value only", func(t *testing.T) {
		response := parse(t, fmt.Sprintf(
			`{"extensions":{"apolloEntityCacheTags":[[%s],["a"]]}}`, tagList(maxResponseCacheTagsPerValue+1)))

		require.Equal(t, [][]string{nil, {"a"}}, responseCacheTags(response, 2, false))
	})

	t.Run("an object where the array should be is not tags", func(t *testing.T) {
		response := parse(t, `{"extensions":{"apolloEntityCacheTags":{"users":["user-42"]}}}`)
		require.Empty(t, responseCacheTags(response, 1, false))
	})
}

// An entry is indexed under three separate kinds of thing, and which kind a tag
// came from has to survive into the index or one could be mistaken for another.
func TestResponseCacheTagIdentities(t *testing.T) {
	parse := func(t *testing.T, doc string) *astjson.Value {
		t.Helper()
		value, err := astjson.Parse(doc)
		require.NoError(t, err)
		return value
	}

	all := ResponseCacheInvalidationOptions{CacheTag: true, Subgraph: true, Type: true}
	entity := func(t *testing.T) *astjson.Value {
		return parse(t, `{"__typename":"User","id":42}`)
	}

	t.Run("everything an entry is about, each under where it came from", func(t *testing.T) {
		got := responseCacheTagIdentities(responseCacheTagInput{
			declared: []string{"users", "user-42"},
			value:    entity(t),
			subgraph: "accounts",
			opts:     all,
		})
		require.Equal(t, []string{
			"declared:accounts:users", "declared:accounts:user-42",
			"subgraph:accounts",
			"type:accounts:User",
		}, got)
	})

	t.Run("a subgraph cannot declare its way into the router's own indexes", func(t *testing.T) {
		// The namespace is applied to what the subgraph said, not taken from
		// it, so a tag spelled like a derived one lands beside them and not
		// among them.
		got := responseCacheTagIdentities(responseCacheTagInput{
			declared: []string{"subgraph:evil", "type:Admin"},
			value:    entity(t),
			subgraph: "accounts",
			opts:     all,
		})
		require.Equal(t, []string{
			"declared:accounts:subgraph:evil", "declared:accounts:type:Admin",
			"subgraph:accounts",
			"type:accounts:User",
		}, got)
	})

	t.Run("the by-subgraph index can be turned off on its own", func(t *testing.T) {
		opts := all
		opts.Subgraph = false
		require.Equal(t, []string{"declared:accounts:users", "type:accounts:User"},
			responseCacheTagIdentities(responseCacheTagInput{
				declared: []string{"users"},
				value:    entity(t),
				subgraph: "accounts",
				opts:     opts,
			}))
	})

	t.Run("the by-type index can be turned off on its own", func(t *testing.T) {
		opts := all
		opts.Type = false
		require.Equal(t, []string{"declared:accounts:users", "subgraph:accounts"},
			responseCacheTagIdentities(responseCacheTagInput{
				declared: []string{"users"},
				value:    entity(t),
				subgraph: "accounts",
				opts:     opts,
			}))
	})

	t.Run("both derived indexes off leaves only what the subgraph declared", func(t *testing.T) {
		opts := ResponseCacheInvalidationOptions{CacheTag: true}
		require.Equal(t, []string{"declared:accounts:users"},
			responseCacheTagIdentities(responseCacheTagInput{
				declared: []string{"users"},
				value:    entity(t),
				subgraph: "accounts",
				opts:     opts,
			}))
	})

	t.Run("the derived indexes stand on their own when nothing was declared", func(t *testing.T) {
		// The point of deriving them: an entry is findable without its subgraph
		// having said anything at all.
		require.Equal(t, []string{"subgraph:accounts", "type:accounts:User"},
			responseCacheTagIdentities(responseCacheTagInput{
				value:    entity(t),
				subgraph: "accounts",
				opts:     all,
			}))
	})

	t.Run("a value that does not say what it is is not indexed by type", func(t *testing.T) {
		// A root fetch's data object is a selection set, not an entity, and an
		// entity fetch that did not select __typename did not return one.
		require.Equal(t, []string{"subgraph:accounts"},
			responseCacheTagIdentities(responseCacheTagInput{
				value:    parse(t, `{"id":42}`),
				subgraph: "accounts",
				opts:     all,
			}))
		require.Equal(t, []string{"subgraph:accounts"},
			responseCacheTagIdentities(responseCacheTagInput{
				value:    parse(t, `{"__typename":null}`),
				subgraph: "accounts",
				opts:     all,
			}))
		require.Equal(t, []string{"subgraph:accounts"},
			responseCacheTagIdentities(responseCacheTagInput{
				value:    parse(t, `{"__typename":""}`),
				subgraph: "accounts",
				opts:     all,
			}))
	})

	t.Run("a root fetch is not indexed by type", func(t *testing.T) {
		require.Equal(t, []string{"subgraph:accounts"},
			responseCacheTagIdentities(responseCacheTagInput{
				value:       parse(t, `{"__typename":"Query"}`),
				subgraph:    "accounts",
				isRootFetch: true,
				opts:        all,
			}))
	})

	t.Run("an unnamed subgraph is not indexed at all", func(t *testing.T) {
		// Every identity is scoped by the subgraph that answered, so without a
		// name there is no scope to file the entry under. Indexing it unscoped
		// would put it where another subgraph's invalidation could reach it,
		// which is worse than not indexing it: the entry still expires on its
		// own TTL either way.
		require.Empty(t, responseCacheTagIdentities(responseCacheTagInput{
			declared: []string{"users"},
			value:    entity(t),
			subgraph: "",
			opts:     all,
		}))
	})

	t.Run("nothing to index at all yields no tags", func(t *testing.T) {
		opts := ResponseCacheInvalidationOptions{CacheTag: true}
		require.Empty(t, responseCacheTagIdentities(responseCacheTagInput{
			value:    entity(t),
			subgraph: "accounts",
			opts:     opts,
		}))
	})

	t.Run("the derived indexes are not charged to the subgraph's cap", func(t *testing.T) {
		// The cap bounds what a subgraph asserts. The two the router adds for
		// itself are one entry each and are not the subgraph's to spend.
		got := responseCacheTagIdentities(responseCacheTagInput{
			declared: []string{"a", "b"},
			value:    entity(t),
			subgraph: "accounts",
			opts:     all,
		})
		require.Len(t, got, 4)
	})
}

// The seam between a subgraph response and the cache: what responseCacheCollect
// hands to SetMany is what the store will be asked to index, so this covers the
// whole path from the extension a subgraph wrote to the tags on an item.
func TestResponseCacheCollectTags(t *testing.T) {
	newLoader := func(t *testing.T, body string, opts ResponseCacheInvalidationOptions) (*Loader, *result) {
		t.Helper()

		ctx := NewContext(context.Background())
		ctx.SetResponseCache(ResponseCacheOptions{
			Store:        newTestCache(),
			DefaultTTL:   time.Minute,
			Invalidation: opts,
		})

		res := &result{
			out:        []byte(body),
			statusCode: http.StatusOK,
			httpResponseContext: &httpclient.ResponseContext{
				Response: &http.Response{
					Header: http.Header{"Cache-Control": []string{"public, max-age=60"}},
				},
			},
		}
		res.init(PostProcessingConfiguration{
			SelectResponseDataPath:   []string{"data"},
			SelectResponseErrorsPath: []string{"errors"},
		}, &FetchInfo{DataSourceName: "accounts"})

		return &Loader{ctx: ctx}, res
	}

	// Declared tags only, so what these assert is not mixed in with what the
	// router derives. The derived indexes get their own cases below.
	declaredOnly := ResponseCacheInvalidationOptions{CacheTag: true}

	collect := func(t *testing.T, body string, keys []string, opts ResponseCacheInvalidationOptions) []caching.Item {
		t.Helper()
		loader, res := newLoader(t, body, opts)
		prepared := &preparedFetch{res: res, responseCacheKeys: keys}
		require.NoError(t, loader.responseCacheCollect(prepared))
		return prepared.responseCacheItems
	}

	const entitiesBody = `{
		"data": {"_entities": [
			{"__typename": "User", "id": 42, "name": "Alice"},
			{"__typename": "User", "id": 1023, "name": "Bob"},
			{"__typename": "User", "id": 7, "name": "Charlie"}
		]},
		"extensions": {"apolloEntityCacheTags": [
			["users", "user-42"],
			["users", "user-1023"],
			["users", "user-7"]
		]}
	}`

	t.Run("each entity is stored with the tags named for its position", func(t *testing.T) {
		items := collect(t, entitiesBody, []string{"k-42", "k-1023", "k-7"}, declaredOnly)

		require.Len(t, items, 3)
		require.Equal(t, []string{"declared:accounts:users", "declared:accounts:user-42"}, items[0].Tags)
		require.Equal(t, []string{"declared:accounts:users", "declared:accounts:user-1023"}, items[1].Tags)
		require.Equal(t, []string{"declared:accounts:users", "declared:accounts:user-7"}, items[2].Tags)

		// The tags ride alongside the value, they do not replace or alter it.
		require.JSONEq(t, `{"__typename":"User","id":42,"name":"Alice"}`, string(items[0].Value))
		require.Equal(t, "k-42", items[0].Key)
		require.Equal(t, time.Minute, items[0].TTL)
	})

	t.Run("the derived indexes reach the item alongside the declared ones", func(t *testing.T) {
		opts := declaredOnly
		opts.Subgraph = true
		opts.Type = true

		items := collect(t, entitiesBody, []string{"k-42", "k-1023", "k-7"}, opts)

		require.Len(t, items, 3)
		require.Equal(t, []string{
			"declared:accounts:users", "declared:accounts:user-42",
			"subgraph:accounts",
			"type:accounts:User",
		}, items[0].Tags)
	})

	t.Run("an untagging subgraph is still indexed by subgraph and type", func(t *testing.T) {
		opts := declaredOnly
		opts.Subgraph = true
		opts.Type = true

		body := `{"data":{"_entities":[{"__typename":"User","id":42}]}}`
		items := collect(t, body, []string{"k-42"}, opts)

		require.Len(t, items, 1)
		require.Equal(t, []string{"subgraph:accounts", "type:accounts:User"}, items[0].Tags)
	})

	t.Run("a null entity keeps the remaining tags on their own entities", func(t *testing.T) {
		// The null is skipped without consuming a tag list, which is the only
		// thing keeping the two aligned past it.
		body := `{
			"data": {"_entities": [{"id": 42}, null, {"id": 7}]},
			"extensions": {"apolloEntityCacheTags": [["user-42"], ["user-1023"], ["user-7"]]}
		}`
		items := collect(t, body, []string{"k-42", "k-1023", "k-7"}, declaredOnly)

		require.Len(t, items, 2)
		require.Equal(t, "k-42", items[0].Key)
		require.Equal(t, []string{"declared:accounts:user-42"}, items[0].Tags)
		require.Equal(t, "k-7", items[1].Key)
		require.Equal(t, []string{"declared:accounts:user-7"}, items[1].Tags)
	})

	t.Run("a response with no tags is cached exactly as it was before", func(t *testing.T) {
		body := `{"data": {"_entities": [{"id": 42}]}}`
		items := collect(t, body, []string{"k-42"}, declaredOnly)

		require.Len(t, items, 1)
		require.Empty(t, items[0].Tags)
		require.Equal(t, time.Minute, items[0].TTL)
	})

	t.Run("tags that do not line up cost the whole response its tags, not its caching", func(t *testing.T) {
		body := `{
			"data": {"_entities": [{"id": 42}, {"id": 7}]},
			"extensions": {"apolloEntityCacheTags": [["user-42"]]}
		}`
		items := collect(t, body, []string{"k-42", "k-7"}, declaredOnly)

		require.Len(t, items, 2, "the entities are still cacheable")
		require.Empty(t, items[0].Tags)
		require.Empty(t, items[1].Tags)
	})

	t.Run("every switch off leaves the entries untagged", func(t *testing.T) {
		items := collect(t, entitiesBody, []string{"k-42", "k-1023", "k-7"},
			ResponseCacheInvalidationOptions{})

		require.Len(t, items, 3)
		for _, item := range items {
			require.Empty(t, item.Tags)
		}
	})

	t.Run("cache_tag off keeps the derived indexes", func(t *testing.T) {
		items := collect(t, entitiesBody, []string{"k-42", "k-1023", "k-7"},
			ResponseCacheInvalidationOptions{Subgraph: true, Type: true})

		require.Len(t, items, 3)
		require.Equal(t, []string{"subgraph:accounts", "type:accounts:User"}, items[0].Tags)
	})

	t.Run("each switch is independent", func(t *testing.T) {
		only := func(opts ResponseCacheInvalidationOptions) []string {
			return collect(t, entitiesBody, []string{"k-42", "k-1023", "k-7"}, opts)[0].Tags
		}

		require.Equal(t, []string{"declared:accounts:users", "declared:accounts:user-42"},
			only(ResponseCacheInvalidationOptions{CacheTag: true}))
		require.Equal(t, []string{"subgraph:accounts"},
			only(ResponseCacheInvalidationOptions{Subgraph: true}))
		require.Equal(t, []string{"type:accounts:User"},
			only(ResponseCacheInvalidationOptions{Type: true}))
	})

	t.Run("a root fetch takes its tags from one flat list", func(t *testing.T) {
		body := `{
			"data": {"employees": [{"id": 1}]},
			"extensions": {"apolloCacheTags": ["employees", "homepage"]}
		}`

		opts := declaredOnly
		opts.Subgraph = true
		opts.Type = true

		loader, res := newLoader(t, body, opts)
		prepared := &preparedFetch{res: res, responseCacheKeys: []string{"root"}, isRootFetchCache: true}
		require.NoError(t, loader.responseCacheCollect(prepared))

		require.Len(t, prepared.responseCacheItems, 1)
		// No type: a root fetch's data object is a selection set rather than an
		// entity, so there is no one typename it could be indexed under.
		require.Equal(t, []string{
			"declared:accounts:employees", "declared:accounts:homepage",
			"subgraph:accounts",
		}, prepared.responseCacheItems[0].Tags)
		require.JSONEq(t, `{"employees":[{"id":1}]}`, string(prepared.responseCacheItems[0].Value))
	})

	t.Run("an errored response is not cached and so is not tagged", func(t *testing.T) {
		body := `{
			"data": {"_entities": [{"id": 42}]},
			"errors": [{"message": "boom"}],
			"extensions": {"apolloEntityCacheTags": [["user-42"]]}
		}`
		require.Empty(t, collect(t, body, []string{"k-42"}, declaredOnly))
	})

	t.Run("a root fetch is never indexed by type", func(t *testing.T) {
		body := `{"data":{"__typename":"Query","employees":[{"id":1}]}}`
		loader, res := newLoader(t, body, ResponseCacheInvalidationOptions{Type: true})
		prepared := &preparedFetch{res: res, responseCacheKeys: []string{"root"}, isRootFetchCache: true}
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Len(t, prepared.responseCacheItems, 1)
		require.Empty(t, prepared.responseCacheItems[0].Tags)
	})
}

func TestRemainingTTL(t *testing.T) {
	item := func(ttl time.Duration) caching.Item { return caching.Item{TTL: ttl} }

	testCases := []struct {
		name     string
		found    map[string]caching.Item
		keys     []string
		expected time.Duration
	}{
		{
			name:     "the shortest of several entries",
			found:    map[string]caching.Item{"a": item(30 * time.Second), "b": item(10 * time.Second)},
			keys:     []string{"a", "b"},
			expected: 10 * time.Second,
		},
		{
			name:     "zero is a lifetime, not an absent one",
			found:    map[string]caching.Item{"a": item(30 * time.Second), "b": item(0)},
			keys:     []string{"a", "b"},
			expected: 0,
		},
		{
			name:     "a lone zero survives",
			found:    map[string]caching.Item{"a": item(0)},
			keys:     []string{"a"},
			expected: 0,
		},
		{
			name:     "a negative TTL is dropped in favour of its neighbours",
			found:    map[string]caching.Item{"a": item(-5 * time.Second), "b": item(10 * time.Second)},
			keys:     []string{"a", "b"},
			expected: 10 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, remainingTTL(tc.found, tc.keys))
		})
	}
}
