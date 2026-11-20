package resolve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestInterfaceContextGroups(t *testing.T) {
	l := Loader{dataBuffer: &DataBuffer{data: astjson.MustParse(`{"accounts":[{"__typename":"Person","locale":"en","wallet":{"id":"1"}},{"__typename":"Team","defaultLocale":"de","wallet":{"id":"1"}},{"__typename":"Person","locale":"en","wallet":{"id":"1"}},{"__typename":"Unknown","wallet":{"id":"1"}}]}`)}}
	binding := ContextBinding{Variable: "lang", Sources: []ContextSource{{Depth: 1, Selection: []ContextSelection{
		{Types: []string{"Person"}, Children: []ContextSelection{{Field: "locale"}}},
		{Types: []string{"Team"}, Children: []ContextSelection{{Field: "defaultLocale"}}},
	}}}}
	groups, _ := l.contextGroups([]FetchItemPathElement{{Path: []string{"accounts"}}, {Path: []string{"wallet"}}}, []ContextBinding{binding}, nil)
	require.Len(t, groups, 3)
	require.Len(t, groups[0].items, 2)
	require.Same(t, l.dataBuffer.Get().Get("accounts").GetArray()[2].Get("wallet"), groups[0].items[1])
	template := InputTemplate{SetTemplateOutputToNullOnVariableNull: true, Segments: []TemplateSegment{{SegmentType: StaticSegmentType, Data: []byte("[")}, binding.TemplateSegment(), {SegmentType: StaticSegmentType, Data: []byte("]")}}}
	for i, want := range []string{`["en"]`, `["de"]`, `[null]`} {
		var buf bytes.Buffer
		require.NoError(t, template.Render(NewContext(t.Context()), groups[i].variables, &buf))
		require.Equal(t, want, buf.String())
	}
}

func TestContextResponseCacheKeys(t *testing.T) {
	binding := ContextBinding{Variable: "plan"}
	header := InputTemplate{Segments: []TemplateSegment{{SegmentType: StaticSegmentType, Data: []byte(`{"variables":{"plan":`)}, binding.TemplateSegment(), {SegmentType: StaticSegmentType, Data: []byte(`,"representations":[`)}}}
	footer := InputTemplate{Segments: []TemplateSegment{{SegmentType: StaticSegmentType, Data: []byte(`]}}`)}}}
	fetch := BatchEntityFetch{Info: &FetchInfo{DataSourceName: "wallets"}, Input: BatchInput{Header: header, Items: []InputTemplate{plainIDTemplate()}, Footer: footer}}
	keys := func(plan, privateID string) (string, string) {
		l, _ := subgraphLoader(t, newTestCache(), "wallets", "private", "", ResponseCacheOptions{PrivateID: privateID})
		res := &result{}
		prepared := &preparedFetch{res: res, contextVariables: astjson.ObjectValue(nil)}
		prepared.contextVariables.Set(nil, "plan", astjson.StringValue(nil, plan))
		require.NoError(t, l.prepareBatchEntityFetch(&FetchItem{}, &fetch, []*astjson.Value{astjson.MustParse(`{"id":"42"}`)}, res, prepared))
		defer batchEntityToolPool.Put(res.tools)
		return prepared.responseCacheKeys[0], prepared.responseCachePrivateKeys[0]
	}
	first, firstPrivate := keys("FREE", "member1")
	again, againPrivate := keys("FREE", "member1")
	other, otherPrivate := keys("PRO", "member1")
	same, secondPrivate := keys("FREE", "member2")
	require.Equal(t, first, again)
	require.Equal(t, firstPrivate, againPrivate)
	require.NotEqual(t, first, other)
	require.NotEqual(t, firstPrivate, otherPrivate)
	require.Equal(t, first, same)
	require.NotEqual(t, firstPrivate, secondPrivate)
}

func TestNestedListsInFetchPath(t *testing.T) {
	doc := astjson.MustParse(`{"matrix":[[],[null,{"__typename":"Member","tier":"FREE","wallet":{"id":1}}],null,[{"__typename":"Team","tier":"PRO","wallet":{"id":2}}]]}`)
	loader := Loader{dataBuffer: &DataBuffer{data: doc}}
	path := []FetchItemPathElement{ArrayPath("matrix"), ObjectPath("wallet")}
	first := doc.Get("matrix", "1", "1", "wallet")
	second := doc.Get("matrix", "3", "0", "wallet")

	t.Run("ordinary entity fetch", func(t *testing.T) {
		require.Equal(t, []*astjson.Value{first, second}, loader.selectItemsForPath(path, nil))
	})
	t.Run("contextual entity fetch", func(t *testing.T) {
		binding := ContextBinding{Variable: "tier", Sources: []ContextSource{{Depth: 1, Selection: []ContextSelection{{Field: "tier"}}}}}
		groups, _ := loader.contextGroups(path, []ContextBinding{binding}, nil)
		require.Len(t, groups, 2)
		require.Equal(t, `{"tier":"FREE"}`, string(groups[0].variables.MarshalTo(nil)))
		require.Equal(t, `{"tier":"PRO"}`, string(groups[1].variables.MarshalTo(nil)))
		require.Same(t, first, groups[0].items[0])
		require.Same(t, second, groups[1].items[0])
	})
}

// The fixture uses real entity cache keys and merge paths. Only the subgraph
// transport is replaced, so call counts also distinguish a hit from a refetch.
func contextTestBatch(source DataSource, name string, id int, dependencies ...int) *BatchEntityFetch {
	return &BatchEntityFetch{
		FetchDependencies: FetchDependencies{FetchID: id, DependsOnFetchIDs: dependencies},
		Info:              &FetchInfo{DataSourceName: name},
		DataSource:        source,
		Input: BatchInput{
			Header:    multiStaticTemplate(`{"ids":[`),
			Items:     []InputTemplate{plainIDTemplate()},
			Separator: multiStaticTemplate(","),
			Footer:    multiStaticTemplate("]}"),
		},
		PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
	}
}

func contextTestQuoteFetch(source DataSource) *BatchEntityFetch {
	fetch := contextTestBatch(source, "quotes", 1)
	binding := ContextBinding{
		Variable: "tier",
		Sources:  []ContextSource{{Depth: 1, Selection: []ContextSelection{{Field: "tier"}}}},
	}
	fetch.Input.Header = multiStaticTemplate(`{"tier":`)
	fetch.Input.Header.Segments = append(fetch.Input.Header.Segments,
		binding.TemplateSegment(), TemplateSegment{SegmentType: StaticSegmentType, Data: []byte(`,"ids":[`)})
	return fetch
}

func TestDistinctContextGroupsLoadConcurrently(t *testing.T) {
	source := NewMockDataSource(gomock.NewController(t))
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	for _, tier := range []string{"FREE", "PRO"} {
		source.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"`+tier+`","ids":[1]}`)).
			DoAndReturn(func(context.Context, http.Header, []byte) ([]byte, error) {
				started <- struct{}{}
				<-release
				return []byte(`{"data":{"_entities":[{"quote":"` + tier + `"}]}}`), nil
			})
	}
	// One entity appears under three ancestors. Equal bindings share a request.
	doc := astjson.MustParse(`{"members":[{"id":1,"tier":"FREE"},{"id":1,"tier":"PRO"},{"id":1,"tier":"FREE"}]}`)
	loader := Loader{dataBuffer: &DataBuffer{data: doc}}
	ctx := NewContext(t.Context())
	plan := Single(contextTestQuoteFetch(source), ArrayPath("members"))
	done := make(chan error, 1)
	go func() { done <- loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: plan}) }()
	defer func() {
		close(release)
		require.NoError(t, <-done)
		for i, tier := range []string{"FREE", "PRO", "FREE"} {
			require.Equal(t, tier, string(doc.Get("members").GetArray()[i].GetStringBytes("quote")))
		}
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("a distinct context group waited for another group's response before starting its request")
		}
	}
}

func TestContextGroupFailureIsIsolated(t *testing.T) {
	groupSource := NewMockDataSource(gomock.NewController(t))
	groupSource.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"FREE","ids":[1],"__trace__":true}`)).
		Return(nil, errors.New("FREE failed"))
	groupSource.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"PRO","ids":[2],"__trace__":true}`)).
		Return([]byte(`{"data":{"_entities":[{"quote":{"id":7}}]}}`), nil)

	details := &recordingDataSource{response: []byte(`{"data":{"_entities":[{"detail":"ok"}]}}`)}
	indirect := &recordingDataSource{response: []byte(`{"data":{"_entities":[{"extra":true}]}}`)}
	independent := &recordingDataSource{response: []byte(`{"data":{"_entities":[{"other":true},{"other":true}]}}`)}
	memberPath, quotePath := ArrayPath("members"), ObjectPath("quote")
	plan := Sequence(
		Single(contextTestQuoteFetch(groupSource), memberPath),
		Single(contextTestBatch(details, "details", 2, 1), memberPath, quotePath),
		Single(contextTestBatch(indirect, "extra", 3, 2), memberPath, quotePath),
		Single(contextTestBatch(independent, "other", 4), memberPath, quotePath),
	)

	// Another field already populated the failed member's quote. Dependency
	// filtering must still exclude it from both direct and transitive dependents.
	doc := astjson.MustParse(`{"members":[{"id":1,"tier":"FREE","quote":{"id":8}},{"id":2,"tier":"PRO"}]}`)
	loader := Loader{dataBuffer: &DataBuffer{data: doc}}
	ctx := NewContext(t.Context())
	ctx.TracingOptions = TraceOptions{Enable: true, EnablePredictableDebugTimings: true}
	require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery}, Fetches: plan}))
	require.JSONEq(t, `{"ids":[7],"__trace__":true}`, string(details.lastInput))
	require.JSONEq(t, `{"ids":[7],"__trace__":true}`, string(indirect.lastInput))
	require.JSONEq(t, `{"ids":[8,7],"__trace__":true}`, string(independent.lastInput))
	require.Nil(t, doc.Get("members", "0", "quote", "detail"))
	require.Equal(t, "ok", string(doc.GetStringBytes("members", "1", "quote", "detail")))
	traces := plan.Trace().Children[0].Fetch.Traces
	require.Len(t, traces, 2)
	require.Contains(t, traces[0].LoadError, "FREE failed")
	require.Empty(t, traces[1].LoadError)
}

func TestContextGroupOnlyIfCached(t *testing.T) {
	headers := http.Header{"Cache-Control": []string{"public, max-age=60"}}
	quotes := &recordingDataSource{responseHeaders: headers, response: []byte(`{"data":{"_entities":[{"quote":{"id":7}}]}}`)}
	details := &recordingDataSource{responseHeaders: headers, response: []byte(`{"data":{"_entities":[{"detail":"ok"}]}}`)}
	memberPath := ArrayPath("members")
	plan := Sequence(
		Single(contextTestQuoteFetch(quotes), memberPath),
		Single(contextTestBatch(details, "details", 2, 1), memberPath, ObjectPath("quote")),
	)
	store := newTestCache()
	run := func(input string, onlyIfCached bool) *astjson.Value {
		t.Helper()
		ctx := NewContext(t.Context())
		ctx.SetResponseCache(ResponseCacheOptions{Store: store, DefaultTTL: time.Minute, OnlyIfCached: onlyIfCached})
		doc := astjson.MustParse(input)
		loader := Loader{dataBuffer: &DataBuffer{data: doc}}
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery}, Fetches: plan}))
		require.Equal(t, onlyIfCached, ctx.ResponseCacheUnsatisfied())
		return doc
	}
	run(`{"members":[{"id":2,"tier":"PRO"}]}`, false)
	doc := run(`{"members":[{"id":2,"tier":"FREE","quote":{"id":7}},{"id":2,"tier":"PRO"}]}`, true)
	require.Equal(t, 1, quotes.calls)
	require.Equal(t, 1, details.calls)
	require.Nil(t, doc.Get("members", "0", "quote", "detail"))
	require.Equal(t, "ok", string(doc.GetStringBytes("members", "1", "quote", "detail")))
}

func TestContextGroupFailureAcrossDeferredPatches(t *testing.T) {
	for _, tc := range []struct {
		name           string
		freeResponse   string
		freeError      error
		detailIDs      string
		detailResponse string
	}{
		{
			name:           "transport error",
			freeError:      errors.New("FREE failed"),
			detailIDs:      `{"ids":[7]}`,
			detailResponse: `{"data":{"_entities":[{"detail":"a"}]}}`,
		},
		{
			name:           "null data",
			freeResponse:   `{"data":null,"errors":[{"message":"FREE failed"}]}`,
			detailIDs:      `{"ids":[7]}`,
			detailResponse: `{"data":{"_entities":[{"detail":"a"}]}}`,
		},
		{
			name:           "null entity in partial batch",
			freeResponse:   `{"data":{"_entities":[null,{"quote":{"id":9}}]},"errors":[{"message":"failed quote","path":["_entities",0,"quote"]}]}`,
			detailIDs:      `{"ids":[9,7]}`,
			detailResponse: `{"data":{"_entities":[{"detail":"b"},{"detail":"a"}]}}`,
		},
		{
			name:           "null required field in partial batch",
			freeResponse:   `{"data":{"_entities":[{"__typename":"Member","required":null},{"quote":{"id":9}}]},"errors":[{"message":"failed requirement","path":["_entities",0,"required"]}]}`,
			detailIDs:      `{"ids":[9,7]}`,
			detailResponse: `{"data":{"_entities":[{"detail":"b"},{"detail":"a"}]}}`,
		},
		{
			name:           "error on unrelated field",
			freeResponse:   `{"data":{"_entities":[{"quote":{"id":8},"color":null},{"quote":{"id":9}}]},"errors":[{"message":"failed color","path":["_entities",0,"color"]}]}`,
			detailIDs:      `{"ids":[8,9,7]}`,
			detailResponse: `{"data":{"_entities":[{"detail":"c"},{"detail":"b"},{"detail":"a"}]}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			member, quote := ArrayPath("members"), ObjectPath("quote")
			quotes := NewMockDataSource(gomock.NewController(t))
			quotes.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"FREE","ids":[1,3]}`)).Return([]byte(tc.freeResponse), tc.freeError)
			quotes.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"PRO","ids":[2]}`)).Return([]byte(`{"data":{"_entities":[{"quote":{"id":7}}]}}`), nil)
			fetch := contextTestQuoteFetch(quotes)
			fetch.PostProcessing.SelectResponseErrorsPath = []string{"errors"}
			fetch.Info.FetchReasons = []FetchReason{{TypeName: "Member", FieldName: "required", IsRequires: true, Nullable: true}}
			sources := []*recordingDataSource{{response: []byte(tc.detailResponse)}, {response: []byte(tc.detailResponse)}}
			var groups []*DeferFetchGroup
			for i, subgraph := range sources {
				groups = append(groups, &DeferFetchGroup{DeferID: i + 1, Fetches: Single(contextTestBatch(subgraph, "details", 2+i, 1), member, quote)})
			}
			response := minimalDeferResponse(groups, map[int]DeferDescriptor{1: {ID: 1}, 2: {ID: 2}})
			// FREE members already have quotes from a previous fetch. The repeated
			// representation is sent once but has two distinct merge targets.
			response.Response.Fetches = Sequence(
				Single(&SingleFetch{FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(`{"members":[{"id":1,"tier":"FREE","quote":{"id":8}},{"id":1,"tier":"FREE","quote":{"id":10}},{"id":3,"tier":"FREE","quote":{"id":9}},{"id":2,"tier":"PRO"}]}`)}}),
				Single(fetch, member),
			)
			response.DeferTree = DeferParallel(DeferSingle(groups[0]), DeferSingle(groups[1]))
			writer := &testDeferWriter{}
			resolver := newResolver(t.Context())
			resolver.options.ValidateRequiredExternalFields = true
			_, err := resolver.ResolveGraphQLDeferResponse(NewContext(t.Context()), response, writer)
			require.NoError(t, err)
			require.Len(t, writer.payloads, 3)
			for _, source := range sources {
				require.JSONEq(t, tc.detailIDs, string(source.lastInput))
			}
		})
	}
}

func TestContextSingleEntityRequiredFieldError(t *testing.T) {
	member := ObjectPath("member")
	quotes := &recordingDataSource{response: []byte(`{"data":{"_entities":[{"__typename":"Member","required":null}]},"errors":[{"message":"failed requirement","path":["_entities",0,"required"]}]}`)}
	batch := contextTestQuoteFetch(quotes)
	fetch := &EntityFetch{
		FetchDependencies: batch.FetchDependencies,
		Info:              batch.Info,
		DataSource:        quotes,
		Input:             EntityInput{Header: batch.Input.Header, Item: batch.Input.Items[0], Footer: batch.Input.Footer},
		PostProcessing:    PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}, SelectResponseErrorsPath: []string{"errors"}},
	}
	fetch.Info.FetchReasons = []FetchReason{{TypeName: "Member", FieldName: "required", IsRequires: true, Nullable: true}}
	details := &recordingDataSource{response: []byte(`{"data":{"_entities":[{"detail":"stale"}]}}`)}
	independent := &recordingDataSource{response: []byte(`{"data":{"_entities":[{"color":"blue"}]}}`)}
	plan := Sequence(
		Single(fetch, member),
		Single(contextTestBatch(details, "details", 2, 1), member, ObjectPath("quote")),
		Single(contextTestBatch(independent, "colors", 3), member),
	)
	doc := astjson.MustParse(`{"member":{"id":1,"tier":"FREE","quote":{"id":8}}}`)
	loader := Loader{dataBuffer: &DataBuffer{data: doc}, validateRequiredExternalFields: true}
	require.NoError(t, loader.LoadGraphQLResponseData(NewContext(t.Context()), &GraphQLResponse{Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery}, Fetches: plan}))
	require.Equal(t, 1, quotes.calls)
	require.Zero(t, details.calls)
	require.Equal(t, 1, independent.calls)
}

func TestFailedContextProjectionOnSibling(t *testing.T) {
	for _, validate := range []bool{false, true} {
		for _, sourceKind := range []string{"context group", "root prerequisite", "ordinary prerequisite", "merged prerequisite"} {
			t.Run(sourceKind+"/validate="+strconv.FormatBool(validate), func(t *testing.T) {
				member, profiles, wallets := ArrayPath("members"), ArrayPath("profiles"), ArrayPath("wallets")
				// The first profile fails while the second legitimately supplies null. Both wallets
				// on the first member must be skipped, including in the dependent deferred fetch.
				seed := `{"members":[{"tier":"A","profiles":[[{"id":11}]],"wallets":[{"id":101},{"id":102}]},{"tier":"B","profiles":[[{"id":22}]],"wallets":[{"id":201}]}]}`
				source := NewMockDataSource(gomock.NewController(t))
				const failure = `{"data":{"_entities":[{"__typename":"Profile","locale":null},{"locale":null}]},"errors":[{"message":"profile failed","path":["_entities",0,"locale"]}]}`
				required := []FetchReason{{TypeName: "Profile", FieldName: "locale", IsRequires: true, Nullable: true}}
				seedFetch := &SingleFetch{FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(seed)}}
				var profileNode *FetchTreeNode
				switch sourceKind {
				case "context group":
					seed = strings.Replace(seed, `"id":11`, `"id":11,"locale":"stale"`, 1)
					seedFetch.DataSource = FakeDataSource(seed)
					profileNode = Single(contextTestQuoteFetch(source), member, profiles)
					source.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"A","ids":[11]}`)).Return(nil, errors.New("profile unavailable"))
					source.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"B","ids":[22]}`)).
						Return([]byte(`{"data":{"_entities":[{"locale":null}]}}`), nil)

				case "root prerequisite":
					seed = strings.ReplaceAll(strings.ReplaceAll(seed, `"id":11`, `"id":11,"locale":null`), `"id":22`, `"id":22,"locale":null`)
					seedFetch.FetchID = 1
					seedFetch.PostProcessing = PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}, SelectResponseErrorsPath: []string{"errors"}}
					seedFetch.DataSource = FakeDataSource(`{"data":` + seed + `,"errors":[{"message":"failed locale","path":["members",0,"profiles",0,0,"locale"]}]}`)
					profileNode = Sequence()
				case "ordinary prerequisite":
					profile := contextTestBatch(source, "profiles", 1)
					profileNode = Single(profile, member, profiles)
					profile.Info.FetchReasons = required
					profile.PostProcessing.SelectResponseErrorsPath = []string{"errors"}
					source.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"ids":[11,22]}`)).
						Return([]byte(failure), nil)
				case "merged prerequisite":
					multi := twoEntryMultiFetch(nil)
					multi.FetchID = 1
					multi.Input.Entries[0].Item.FetchPath = []FetchItemPathElement{member, profiles}
					multi.Input.Entries[0].Info.FetchReasons = required
					multi.Input.Entries[0].Representations = plainIDTemplate()
					multi.DataSource = FakeDataSource(strings.ReplaceAll(failure, "_entities", "f1"))
					profileNode = Single(multi)
				}
				quoteSource := NewMockDataSource(gomock.NewController(t))
				quoteSource.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":[[null]],"ids":[201]}`)).
					Return([]byte(`{"data":{"_entities":[{"quote":"default"}]}}`), nil)
				quote := contextTestQuoteFetch(quoteSource)
				quote.FetchDependencies = FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}
				binding := quote.Input.Header.Segments[1].Renderer.(*ContextBinding)
				binding.Sources[0].Selection = []ContextSelection{{Field: "profiles", Children: []ContextSelection{{Field: "locale"}}}}

				// An independent projection on the same profile object reads its id successfully.
				ids := NewMockDataSource(gomock.NewController(t))
				ids.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":[[11]],"ids":[101,102]}`)).
					Return([]byte(`{"data":{"_entities":[{},{}]}}`), nil)
				ids.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":[[22]],"ids":[201]}`)).
					Return([]byte(`{"data":{"_entities":[{}]}}`), nil)
				byID := contextTestQuoteFetch(ids)
				byID.FetchID = 5
				if sourceKind != "context group" {
					byID.DependsOnFetchIDs = []int{1}
				}
				byID.Input.Header.Segments[1].Renderer.(*ContextBinding).Sources[0].Selection =
					[]ContextSelection{{Field: "profiles", Children: []ContextSelection{{Field: "id"}}}}

				dependent := &recordingDataSource{response: []byte(`{"data":{"_entities":[{"details":"ok"}]}}`)}
				independent := &recordingDataSource{response: []byte(`{"data":{"_entities":[{},{},{}]}}`)}
				group := &DeferFetchGroup{DeferID: 1, Fetches: Sequence(
					Single(contextTestBatch(dependent, "details", 3, 2), member, wallets),
					Single(contextTestBatch(independent, "other", 4), member, wallets),
				)}
				response := minimalDeferResponse([]*DeferFetchGroup{group}, map[int]DeferDescriptor{1: {ID: 1}})
				response.Response.Fetches = Sequence(
					Single(seedFetch),
					profileNode,
					Single(quote, member, wallets),
					Single(byID, member, wallets),
				)
				response.DeferTree = DeferSingle(group)
				resolver := newResolver(t.Context())
				resolver.options.ValidateRequiredExternalFields = validate
				_, err := resolver.ResolveGraphQLDeferResponse(NewContext(t.Context()), response, &testDeferWriter{})
				require.NoError(t, err)
				require.JSONEq(t, `{"ids":[201]}`, string(dependent.lastInput))
				require.JSONEq(t, `{"ids":[101,102,201]}`, string(independent.lastInput))
			})
		}
	}
}

func TestSubscriptionEventContextErrorInNestedList(t *testing.T) {
	source := &recordingDataSource{response: []byte(`{"data":{"_entities":[{}]}}`)}
	fetch := contextTestQuoteFetch(source)
	sub := &subscriptionState{
		writer: &FakeSubscriptionWriter{},
		resolve: &GraphQLSubscription{
			Trigger: GraphQLSubscriptionTrigger{PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}, SelectResponseErrorsPath: []string{"errors"}}},
			Response: &GraphQLResponse{
				Info:    &GraphQLResponseInfo{OperationType: ast.OperationTypeSubscription},
				Data:    &Object{},
				Fetches: Single(fetch, ArrayPath("updates"), ObjectPath("wallet")),
			},
		},
	}
	// Both list-valued context arguments contain a null element, but only
	// the first one came from a field error in the subscription event.
	event := []byte(`{"data":{"updates":[{"tier":[[null]],"wallet":{"id":1}},{"tier":[[null]],"wallet":{"id":2}}]},"errors":[{"message":"failed element","path":["updates",0,"tier",0,0]}]}`)
	r := newResolver(t.Context())
	r.executeSubscriptionUpdate(NewContext(t.Context()), sub, event, nil)
	require.Equal(t, 1, source.calls)
	require.JSONEq(t, `{"tier":[[null]],"ids":[2]}`, string(source.lastInput))
}

func TestContextPrerequisiteResponseAvailability(t *testing.T) {
	for _, fetchKind := range []string{"batch", "single", "merged single"} {
		for _, suppress := range []bool{false, true} {
			for _, tc := range []struct {
				name, response, wantInput string
			}{
				{"successful null", `{"data":{"_entities":[{"locale":null}]}}`, `{"tier":null,"ids":[2]}`},
				{"missing data", `{"data":null}`, ""},
				{"pathless GraphQL error", `{"data":null,"errors":[{"message":"service unavailable"}]}`, ""},
				{"invalid JSON", "Upstream unavailable", ""},
				{"empty response", "", ""},
				{"invalid response shape", `{}`, ""},
				{"absent entity", `{"data":{"_entities":[null]}}`, `{"tier":null,"ids":[2]}`},
				{"missing representation", `{"data":{"_entities":[]}}`, ""},
			} {
				t.Run(fmt.Sprintf("%s/%s/suppress=%t", fetchKind, tc.name, suppress), func(t *testing.T) {
					member, profile, wallet := ObjectPath("member"), ObjectPath("profile"), ObjectPath("wallet")
					prerequisite := contextTestBatch(FakeDataSource(tc.response), "profiles", 1)
					prerequisite.PostProcessing.SelectResponseErrorsPath = []string{"errors"}
					first := Single(prerequisite, member, profile)
					switch fetchKind {
					case "single":
						single := &EntityFetch{
							FetchDependencies: prerequisite.FetchDependencies, Info: prerequisite.Info, DataSource: prerequisite.DataSource,
							Input:          EntityInput{Header: prerequisite.Input.Header, Item: prerequisite.Input.Items[0], Footer: prerequisite.Input.Footer},
							PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}, SelectResponseErrorsPath: []string{"errors"}},
						}
						first = Single(single, member, profile)
					case "merged single":
						multi := twoEntryMultiFetch(nil)
						multi.FetchID = 1
						multi.Input.Entries[1].Item.FetchPath = []FetchItemPathElement{member, profile}
						multi.Input.Entries[1].Representations = plainIDTemplate()
						multi.DataSource = FakeDataSource(strings.ReplaceAll(tc.response, "_entities", "f2"))
						first = Single(multi)
					}
					consumer := &recordingDataSource{response: []byte(`{"data":{"_entities":[{}]}}`)}
					quote := contextTestQuoteFetch(consumer)
					quote.FetchDependencies = FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}
					quote.Input.Header.Segments[1].Renderer.(*ContextBinding).Sources[0].Selection =
						[]ContextSelection{{Field: "profile", Children: []ContextSelection{{Field: "locale"}}}}
					other := &recordingDataSource{response: []byte(`{"data":{"_entities":[{}]}}`)}
					plan := Sequence(
						first,
						Single(quote, member, wallet),
						Single(contextTestBatch(other, "other", 3), member, profile),
					)
					doc := astjson.MustParse(`{"member":{"profile":{"id":1},"wallet":{"id":2}}}`)
					loader := Loader{apolloCompatibilitySuppressFetchErrors: suppress, dataBuffer: &DataBuffer{data: doc}}
					ctx := NewContext(t.Context())
					ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
					require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery}, Fetches: plan}))
					if tc.wantInput != "" {
						require.Equal(t, 1, consumer.calls)
						require.JSONEq(t, tc.wantInput, string(consumer.lastInput))
					} else {
						require.Zero(t, consumer.calls)
					}
					require.JSONEq(t, `{"ids":[1]}`, string(other.lastInput))
				})
			}
		}
	}
}

func TestContextSourceErrorBubblingToEntity(t *testing.T) {
	for _, producer := range []string{"ordinary", "merged", "contextual"} {
		t.Run(producer, func(t *testing.T) {
			members, profile, wallet := ArrayPath("members"), ObjectPath("profile"), ObjectPath("wallet")
			// The first locale error bubbled to its representation. The second
			// representation is legitimately absent, and the third has a region.
			const body = `{"data":{"_entities":[null,null,{"region":"EU"}]},"errors":[{"message":"failed locale","path":["_entities",0,"locale"]}]}`
			var first *FetchTreeNode
			switch producer {
			case "ordinary":
				fetch := contextTestBatch(FakeDataSource(body), "profiles", 1)
				fetch.PostProcessing.SelectResponseErrorsPath = []string{"errors"}
				first = Single(fetch, members, profile)
			case "merged":
				fetch := twoEntryMultiFetch(nil)
				fetch.FetchID = 1
				fetch.Input.Entries[0].Item.FetchPath = []FetchItemPathElement{members, profile}
				fetch.Input.Entries[0].Representations = plainIDTemplate()
				fetch.DataSource = FakeDataSource(strings.ReplaceAll(body, "_entities", "f1"))
				first = Single(fetch)
			case "contextual":
				fetch := contextTestQuoteFetch(FakeDataSource(body))
				fetch.PostProcessing.SelectResponseErrorsPath = []string{"errors"}
				first = Single(fetch, members, profile)
			}
			source := NewMockDataSource(gomock.NewController(t))
			for _, input := range []string{`{"tier":null,"ids":[202]}`, `{"tier":"EU","ids":[303]}`} {
				source.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(input)).Return([]byte(`{"data":{"_entities":[{}]}}`), nil)
			}
			consumer := contextTestQuoteFetch(source)
			consumer.FetchDependencies = FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{1}}
			consumer.Input.Header.Segments[1].Renderer.(*ContextBinding).Sources[0].Selection =
				[]ContextSelection{{Field: "profile", Children: []ContextSelection{{Field: "region"}}}}
			doc := astjson.MustParse(`{"members":[{"profile":{"id":11},"wallet":{"id":101}},{"profile":{"id":22},"wallet":{"id":202}},{"profile":{"id":33},"wallet":{"id":303}}]}`)
			loader := Loader{dataBuffer: &DataBuffer{data: doc}}
			ctx := NewContext(t.Context())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{
				Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery}, Fetches: Sequence(first, Single(consumer, members, wallet)),
			}))
		})
	}
}
