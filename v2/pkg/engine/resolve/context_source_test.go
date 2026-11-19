package resolve

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
)

func TestInterfaceContextGroups(t *testing.T) {
	l := Loader{dataBuffer: &DataBuffer{data: astjson.MustParse(`{"accounts":[{"__typename":"Person","locale":"en","wallet":{"id":"1"}},{"__typename":"Team","defaultLocale":"de","wallet":{"id":"1"}},{"__typename":"Person","locale":"en","wallet":{"id":"1"}},{"__typename":"Unknown","wallet":{"id":"1"}}]}`)}}
	binding := ContextBinding{Variable: "lang", Sources: []ContextSource{{Depth: 1, Selection: []ContextSelection{
		{Types: []string{"Person"}, Children: []ContextSelection{{Field: "locale"}}},
		{Types: []string{"Team"}, Children: []ContextSelection{{Field: "defaultLocale"}}},
	}}}}
	groups := l.contextGroups([]FetchItemPathElement{{Path: []string{"accounts"}}, {Path: []string{"wallet"}}}, []ContextBinding{binding})
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
