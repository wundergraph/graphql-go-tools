package resolve

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
)

func TestResponseCacheVary(t *testing.T) {
	t.Parallel()
	de, fr, en := sentHeaders{"Accept-Language": {"de"}}, sentHeaders{"Accept-Language": {"fr"}}, sentHeaders{"Accept-Language": {"en"}}

	t.Run("collect", func(t *testing.T) {
		t.Parallel()
		const body = `{"data":{"_entities":[{"__typename":"User","id":1},{"__typename":"User","id":2}]}}`
		public := []string{"pub-1", "pub-2"}
		private := []string{"prv-1", "prv-2"}

		collect := func(t *testing.T, loader *Loader, res *result, keys, privateKeys []string) []caching.Item {
			t.Helper()
			prepared := &preparedFetch{res: res, responseCacheKeys: keys, responseCachePrivateKeys: privateKeys}
			require.NoError(t, loader.responseCacheCollect(prepared))
			return prepared.responseCacheItems
		}

		t.Run("a varying body lands under its variant with a record at the base", func(t *testing.T) {
			t.Parallel()
			loader, res := varyLoader(t, newTestCache(), "max-age=60", "Accept-Language", body, "", de)
			// The digest is of the headers the request went out with, not what the builder says now.
			loader.ctx.SubgraphHeadersBuilder = fr
			items := collect(t, loader, res, public, nil)
			require.Len(t, items, 4)

			for i, base := range public {
				record, variant := items[2*i], items[2*i+1]
				require.Equal(t, base, record.Key)
				require.Equal(t, [][]string{{"accept-language"}}, record.Vary)
				require.Empty(t, record.Value)
				require.Empty(t, record.SurrogateKeys)
				require.Equal(t, caching.VariantKey(base, caching.VaryDigest([]string{"accept-language"}, http.Header(de))), variant.Key)
				require.Empty(t, variant.Vary)
				require.JSONEq(t, fmt.Sprintf(`{"__typename":"User","id":%d}`, i+1), string(variant.Value))
				require.Equal(t, variant.Tags, record.Tags, "invalidation drops both")
				require.Equal(t, variant.TTL, record.TTL)
			}
		})

		t.Run("Vary: * or too many names is not stored", func(t *testing.T) {
			t.Parallel()
			loader, res := varyLoader(t, newTestCache(), "max-age=60", "Accept-Language, *", body, "", de)
			require.Empty(t, collect(t, loader, res, public, nil))

			names := make([]string, caching.MaxVaryHeaders+1)
			for i := range names {
				names[i] = fmt.Sprintf("x-%d", i)
			}
			loader, res = varyLoader(t, newTestCache(), "max-age=60", strings.Join(names, ", "), body, "", de)
			require.Empty(t, collect(t, loader, res, public, nil))

			loader, res = varyLoader(t, newTestCache(), "max-age=60", strings.Join(names[1:], ", "), body, "", de)
			require.Len(t, collect(t, loader, res, public, nil), 4, "the cap itself is fine")
		})

		t.Run("a private response varies under the user's key", func(t *testing.T) {
			t.Parallel()
			loader, res := varyLoader(t, newTestCache(), "private, max-age=60", "Accept-Language", body, "u1", de)
			items := collect(t, loader, res, public, private)
			require.Len(t, items, 4)
			require.Equal(t, "prv-2", items[2].Key)
			require.Equal(t, caching.VariantKey("prv-2", caching.VaryDigest([]string{"accept-language"}, http.Header(de))), items[3].Key)
		})
	})

	t.Run("lookup", func(t *testing.T) {
		t.Parallel()
		public := []string{"pub-1", "pub-2"}
		private := []string{"prv-1", "prv-2"}
		langVary := [][]string{{"accept-language"}}

		lookup := func(t *testing.T, store caching.Cache, sent sentHeaders, privateID string, keys, privateKeys []string) (bool, *result) {
			t.Helper()
			loader, res := varyLoader(t, store, "", "", "", privateID, sent)
			prepared := &preparedFetch{res: res, responseCacheKeys: keys, responseCachePrivateKeys: privateKeys}
			return loader.responseCacheLookup(prepared), res
		}

		t.Run("a record sends a second lookup for the variant", func(t *testing.T) {
			t.Parallel()
			store := newSpyCache()
			require.NoError(t, store.SetMany(context.Background(), []caching.Item{
				{Key: "pub-1", Vary: langVary, TTL: time.Minute},
				{Key: caching.VariantKey("pub-1", caching.VaryDigest([]string{"accept-language"}, http.Header(de))), Value: []byte(`{"id":1,"lang":"de"}`), TTL: 30 * time.Second},
				{Key: caching.VariantKey("pub-1", caching.VaryDigest([]string{"accept-language"}, http.Header(fr))), Value: []byte(`{"id":1,"lang":"fr"}`), TTL: 30 * time.Second},
			}))

			hit, res := lookup(t, store, de, "", public[:1], nil)
			require.True(t, hit)
			require.JSONEq(t, `{"data":{"_entities":[{"id":1,"lang":"de"}]}}`, string(res.out))
			require.Equal(t, 30*time.Second, res.responseCacheTTL, "the variant's life, not the record's")
			require.Equal(t, [][]string{{"pub-1"}, {caching.VariantKey("pub-1", caching.VaryDigest([]string{"accept-language"}, http.Header(de)))}}, store.lookups)

			hit, res = lookup(t, store, fr, "", public[:1], nil)
			require.True(t, hit)
			require.JSONEq(t, `{"data":{"_entities":[{"id":1,"lang":"fr"}]}}`, string(res.out))
		})

		t.Run("a batch mixes bodies and records", func(t *testing.T) {
			t.Parallel()
			store := newSpyCache()
			require.NoError(t, store.SetMany(context.Background(), []caching.Item{
				{Key: "pub-1", Value: []byte(`{"id":1}`), TTL: time.Minute},
				{Key: "pub-2", Vary: langVary, TTL: time.Minute},
				{Key: caching.VariantKey("pub-2", caching.VaryDigest([]string{"accept-language"}, http.Header(de))), Value: []byte(`{"id":2,"lang":"de"}`), TTL: time.Minute},
			}))
			hit, res := lookup(t, store, de, "", public, nil)
			require.True(t, hit)
			require.JSONEq(t, `{"data":{"_entities":[{"id":1},{"id":2,"lang":"de"}]}}`, string(res.out))
			require.Equal(t, [][]string{public, {caching.VariantKey("pub-2", caching.VaryDigest([]string{"accept-language"}, http.Header(de)))}}, store.lookups, "only the record needs a second round")

			hit, _ = lookup(t, store, en, "", public, nil)
			require.False(t, hit, "a record without its variant is a miss for the whole fetch")
			require.Len(t, store.lookups, 4)

			require.NoError(t, store.SetMany(context.Background(), []caching.Item{{Key: caching.VariantKey("pub-2", caching.VaryDigest([]string{"accept-language"}, http.Header(de))), Vary: langVary, TTL: time.Minute}}))
			hit, _ = lookup(t, store, de, "", public, nil)
			require.False(t, hit, "a record where a body should be")
		})

		t.Run("a private record wins over a public body", func(t *testing.T) {
			t.Parallel()
			store := newSpyCache()
			require.NoError(t, store.SetMany(context.Background(), []caching.Item{
				{Key: "pub-1", Value: []byte(`{"id":1,"who":"everyone"}`), TTL: time.Minute},
				{Key: "prv-1", Vary: langVary, TTL: time.Minute},
				{Key: caching.VariantKey("prv-1", caching.VaryDigest([]string{"accept-language"}, http.Header(de))), Value: []byte(`{"id":1,"who":"u1","lang":"de"}`), TTL: time.Minute},
			}))

			hit, res := lookup(t, store, de, "u1", public[:1], private[:1])
			require.True(t, hit)
			require.True(t, res.responseCachePrivate)
			require.JSONEq(t, `{"data":{"_entities":[{"id":1,"who":"u1","lang":"de"}]}}`, string(res.out))
			require.Equal(t, [][]string{{"pub-1", "prv-1"}, {caching.VariantKey("prv-1", caching.VaryDigest([]string{"accept-language"}, http.Header(de)))}}, store.lookups)

			hit, _ = lookup(t, store, fr, "u1", public[:1], private[:1])
			require.False(t, hit, "the user's record is followed, not the shared body")

			hit, res = lookup(t, store, fr, "", public[:1], nil)
			require.True(t, hit, "without an id the shared body is what there is")
			require.False(t, res.responseCachePrivate)
		})
	})

	// A merged fetch stores and serves variants like the single fetches it replaces:
	// one Vary for the whole answer, a record per entity, a body per variant.
	t.Run("multi entity", func(t *testing.T) {
		t.Parallel()
		const (
			deResponse = `{"data":{` +
				`"f1":[{"__typename":"Employee","products":["a"]},{"__typename":"Employee","products":["b"]}],` +
				`"f2":[{"__typename":"Employee","notes":"n"}]}}`
			enResponse = `{"data":{` +
				`"f1":[{"__typename":"Employee","products":["x"]},{"__typename":"Employee","products":["y"]}],` +
				`"f2":[{"__typename":"Employee","notes":"m"}]}}`
		)

		varyHeaders := func(vary string) http.Header {
			headers := cacheableHeaders()
			headers.Set("Vary", vary)
			return headers
		}

		run := func(t *testing.T, cache *testCache, multiDS *recordingDataSource, sent sentHeaders) (*Context, string) {
			t.Helper()
			ctx := multiEntityContext(t)
			ctx.SubgraphHeadersBuilder = sent
			ctx.SetResponseCache(ResponseCacheOptions{
				Store:      cache,
				DefaultTTL: 60 * time.Second,
				OnError:    func(err error) { t.Errorf("response cache error: %v", err) },
			})
			multi := twoEntryMultiFetch(multiEntityFirstVar())
			multi.FetchID = 1
			multi.DependsOnFetchIDs = []int{0}
			multi.DataSource = multiDS
			multi.Info.DataSourceName = "products"

			loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
			require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: Sequence(
				Single(multiEntityRootFetch(&recordingDataSource{response: []byte(multiEntityRootResponse)})),
				Single(multi),
			)}))
			assertMergedErrors(t, loader, "")
			return ctx, string(loader.dataBuffer.data.MarshalTo(nil))
		}

		// records and variants split what the cache holds: a record has no body.
		records := func(cache *testCache) (records, variants []string) {
			for key, item := range cache.items {
				if len(item.Vary) > 0 {
					records = append(records, key)
				} else {
					variants = append(variants, key)
				}
			}
			return records, variants
		}

		t.Run("a variant is stored per entity, served whole, and kept beside other variants", func(t *testing.T) {
			t.Parallel()
			cache := newTestCache()
			run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language")}, de)

			bases, variants := records(cache)
			require.Len(t, bases, 3)
			require.Len(t, variants, 3)
			for _, base := range bases {
				require.Equal(t, [][]string{{"accept-language"}}, cache.items[base].Vary)
				variant, ok := cache.items[caching.VariantKey(base, caching.VaryDigest([]string{"accept-language"}, http.Header(de)))]
				require.True(t, ok, "variant of %s", base)
				require.NotEmpty(t, variant.Value)
			}

			warm := &recordingDataSource{err: fmt.Errorf("subgraph must not be called")}
			ctx, out := run(t, cache, warm, de)
			require.Equal(t, 0, warm.calls)
			require.Contains(t, out, `"products":["a"]`)
			require.Contains(t, out, `"notes":"n"`)
			require.ElementsMatch(t, []string{caching.SubgraphSurrogateKey("products"), caching.TypeSurrogateKey("products", "Employee")}, ctx.ResponseCacheSurrogateKeys())

			origin := &recordingDataSource{response: []byte(enResponse), responseHeaders: varyHeaders("Accept-Language")}
			_, out = run(t, cache, origin, en)
			require.Equal(t, 1, origin.calls)
			require.Contains(t, out, `"products":["x"]`)

			bases, variants = records(cache)
			require.Len(t, bases, 3, "one record per entity, rewritten not doubled")
			require.Len(t, variants, 6, "a body per variant per entity")

			_, out = run(t, cache, warm, de)
			require.Contains(t, out, `"products":["a"]`)
			_, out = run(t, cache, warm, en)
			require.Contains(t, out, `"products":["x"]`)
			require.Equal(t, 0, warm.calls)
		})

		t.Run("a record whose variant is gone sends its entry to the origin, the rest stay warm", func(t *testing.T) {
			t.Parallel()
			cache := newTestCache()
			run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language")}, de)

			// Evict f2's body, leaving its record pointing at nothing.
			for key, item := range cache.items {
				if bytes.Contains(item.Value, []byte("notes")) {
					delete(cache.items, key)
				}
			}
			bases, variants := records(cache)
			require.Len(t, bases, 3)
			require.Len(t, variants, 2)

			partial := &recordingDataSource{
				response:        []byte(`{"data":{"f2":[{"__typename":"Employee","notes":"again"}]}}`),
				responseHeaders: varyHeaders("Accept-Language"),
			}
			_, out := run(t, cache, partial, de)
			require.Equal(t, 1, partial.calls)
			require.NotContains(t, string(partial.lastInput), `"id":1`, "f1 was warm and not asked for")
			require.Contains(t, string(partial.lastInput), `"id":9`)
			require.Contains(t, out, `"products":["a"]`)
			require.Contains(t, out, `"notes":"again"`)

			_, variants = records(cache)
			require.Len(t, variants, 3, "the origin's answer is stored back under its variant")
		})

		t.Run("a narrower Vary later keeps the wider set's variants reachable", func(t *testing.T) {
			t.Parallel()
			cache := newTestCache()
			deEU := sentHeaders{"Accept-Language": []string{"de"}, "X-Region": []string{"eu"}}
			run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language, X-Region")}, deEU)

			// A request the wide set has no variant for: a miss, and this time the
			// origin varies on fewer headers.
			narrow := &recordingDataSource{response: []byte(enResponse), responseHeaders: varyHeaders("Accept-Language")}
			run(t, cache, narrow, en)
			require.Equal(t, 1, narrow.calls)

			bases, variants := records(cache)
			require.Len(t, bases, 3)
			require.Len(t, variants, 6)
			for _, base := range bases {
				require.Equal(t, [][]string{{"accept-language"}, {"accept-language", "x-region"}}, cache.items[base].Vary)
			}

			warm := &recordingDataSource{err: fmt.Errorf("subgraph must not be called")}
			_, out := run(t, cache, warm, deEU)
			require.Contains(t, out, `"products":["a"]`, "the wide set's variant, under the narrow set there is none for de")
			_, out = run(t, cache, warm, en)
			require.Contains(t, out, `"products":["x"]`, "the narrow set's variant")
			_, out = run(t, cache, warm, sentHeaders{"Accept-Language": []string{"en"}, "X-Region": []string{"us"}})
			require.Contains(t, out, `"products":["x"]`, "a header outside the narrow set does not change its variant")
			require.Equal(t, 0, warm.calls)
		})
	})

	// A record holds every name set responses at its key have varied on, and a
	// request is served from whichever set's variant is there.
	t.Run("sets", func(t *testing.T) {
		t.Parallel()
		langRegion := []string{"accept-language", "x-region"}
		deEU := sentHeaders{"Accept-Language": []string{"de"}, "X-Region": []string{"eu"}}
		digest := func(names []string, sent sentHeaders) caching.Digest {
			return caching.VaryDigest(names, http.Header(sent))
		}
		variant := func(names []string, sent sentHeaders, body string) caching.Item {
			return caching.Item{Key: caching.VariantKey("pub-1", digest(names, sent)), Value: []byte(body), TTL: time.Minute}
		}
		record := caching.Item{Key: "pub-1", Vary: [][]string{{"accept-language"}, langRegion}, TTL: time.Minute}

		seed := func(t *testing.T, store caching.Cache, items ...caching.Item) {
			t.Helper()
			require.NoError(t, store.SetMany(context.Background(), items))
		}
		lookup := func(t *testing.T, store caching.Cache, sent sentHeaders) (bool, *result) {
			t.Helper()
			loader, res := varyLoader(t, store, "", "", "", "", sent)
			prepared := &preparedFetch{res: res, responseCacheKeys: []string{"pub-1"}}
			return loader.responseCacheLookup(prepared), res
		}

		t.Run("every set is asked for in one round and the newest wins", func(t *testing.T) {
			t.Parallel()
			store := newSpyCache()
			seed(t, store, record, variant([]string{"accept-language"}, deEU, `{"set":"lang"}`), variant(langRegion, deEU, `{"set":"lang-region"}`))

			hit, res := lookup(t, store, deEU)
			require.True(t, hit)
			require.Contains(t, string(res.out), `"set":"lang"}`)
			require.Len(t, store.lookups, 2)
			require.Equal(t, []string{
				caching.VariantKey("pub-1", digest([]string{"accept-language"}, deEU)),
				caching.VariantKey("pub-1", digest(langRegion, deEU)),
			}, store.lookups[1])
		})

		t.Run("a header the request does not send counts as empty", func(t *testing.T) {
			t.Parallel()
			store := newTestCache()
			// Stored by a request missing one of the set's headers.
			seed(t, store, record, variant(langRegion, de, `{"region":"none"}`))

			hit, res := lookup(t, store, de)
			require.True(t, hit, "missing it again reads the same variant")
			require.Contains(t, string(res.out), `"region":"none"`)

			hit, _ = lookup(t, store, deEU)
			require.False(t, hit, "sending it is another variant")
		})

		t.Run("a write after a miss keeps the sets already there", func(t *testing.T) {
			t.Parallel()
			const body = `{"data":{"_entities":[{"__typename":"User","id":1}]}}`
			store := newTestCache()
			seed(t, store, caching.Item{Key: "pub-1", Vary: [][]string{{"accept-language"}}, TTL: time.Minute})

			loader, res := varyLoader(t, store, "max-age=60", "Accept-Language, X-Region", body, "", deEU)
			prepared := &preparedFetch{res: res, responseCacheKeys: []string{"pub-1"}}
			require.False(t, loader.responseCacheLookup(prepared), "a record without its variant")
			require.NoError(t, loader.responseCacheCollect(prepared))

			items := prepared.responseCacheItems
			require.Len(t, items, 2)
			require.Equal(t, "pub-1", items[0].Key)
			require.Equal(t, [][]string{langRegion, {"accept-language"}}, items[0].Vary, "own set first, the seen one kept")
			require.Equal(t, caching.VariantKey("pub-1", digest(langRegion, deEU)), items[1].Key)
		})
	})
}

// sentHeaders forwards the same headers to every subgraph.
type sentHeaders http.Header

func (h sentHeaders) HeadersForSubgraph(string) (http.Header, uint64) {
	return http.Header(h), h.HashAll()
}

func (h sentHeaders) HashAll() uint64 { return xxhash.Sum64String(fmt.Sprint(http.Header(h))) }

// varyLoader is privateLoader with a Vary response header and a request that
// sends Accept-Language.
func varyLoader(t *testing.T, store caching.Cache, cacheControl, vary, body, privateID string, sent sentHeaders) (*Loader, *result) {
	t.Helper()
	loader, res := privateLoader(t, store, cacheControl, body, privateID)
	if vary != "" {
		res.httpResponseContext.Response.Header.Set("Vary", vary)
	}
	loader.ctx.SubgraphHeadersBuilder = sent
	res.sentHeaders = http.Header(sent)
	return loader, res
}
