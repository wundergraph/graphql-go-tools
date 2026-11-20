package resolve

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
)

func TestContextFetchAfterDeduplicatedEntity(t *testing.T) {
	for _, kind := range []string{"batch", "multi", "deferred"} {
		t.Run(kind, func(t *testing.T) {
			// Both members point to one wallet. The first fetch is deduplicated
			// and returns its offers once. The quote still depends on the member.
			seed := `{"employees":[{"tier":"FREE","wallet":{"__typename":"Wallet","id":1}},{"tier":"PRO","wallet":{"__typename":"Wallet","id":1}}]}`
			member, wallet := ArrayPath("employees"), ObjectPath("wallet")
			const entity = `{"offers":[{"id":7}]}`
			var upstream *FetchTreeNode
			switch kind {
			case "batch", "deferred":
				source := &recordingDataSource{response: []byte(`{"data":{"_entities":[` + entity + `]}}`)}
				upstream = Single(contextTestBatch(source, "wallets", 0), member, wallet)
			case "multi":
				fetch := twoEntryMultiFetch(nil)
				fetch.Input.Entries[0].Item.FetchPath = []FetchItemPathElement{member, wallet}
				fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[` + entity + `]}}`)}
				upstream = Single(fetch)
			}

			source := NewMockDataSource(gomock.NewController(t))
			for _, tier := range []string{"FREE", "PRO"} {
				source.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"`+tier+`","ids":[7]}`)).
					Return([]byte(`{"data":{"_entities":[{"quote":"`+tier+`"}]}}`), nil)
			}
			quote := Single(contextTestQuoteFetch(source), member, wallet, ArrayPath("offers"))
			ctx := NewContext(t.Context())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			if kind == "deferred" {
				group := &DeferFetchGroup{DeferID: 1, Fetches: quote}
				response := minimalDeferResponse([]*DeferFetchGroup{group}, map[int]DeferDescriptor{1: {ID: 1}})
				response.Response.Fetches = Sequence(Single(&SingleFetch{FetchConfiguration: FetchConfiguration{DataSource: FakeDataSource(seed)}}), upstream)
				response.DeferTree = DeferSingle(group)
				response.Response.Data.Fields[0] = &Field{
					Name: []byte("employees"), Defer: &DeferField{DeferID: 1},
					Value: &Array{Path: []string{"employees"}, Item: &Object{Fields: []*Field{{
						Name: []byte("quote"), Defer: &DeferField{DeferID: 1}, Value: &String{Path: []string{"wallet", "offers", "0", "quote"}},
					}}}},
				}
				writer := &testDeferWriter{}
				_, err := newResolver(t.Context()).ResolveGraphQLDeferResponse(ctx, response, writer)
				require.NoError(t, err)
				require.Len(t, writer.payloads, 2)
				patch := astjson.MustParse(writer.payloads[1])
				require.JSONEq(t, `[{"quote":"FREE"},{"quote":"PRO"}]`, string(patch.Get("incremental", "0", "data", "employees").MarshalTo(nil)))
				return
			}
			doc := astjson.MustParse(seed)
			loader := Loader{dataBuffer: &DataBuffer{data: doc}}
			require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: Sequence(upstream, quote)}))
			for _, member := range doc.GetArray("employees") {
				require.Equal(t, string(member.GetStringBytes("tier")), string(member.GetStringBytes("wallet", "offers", "0", "quote")))
			}
		})
	}
}

func TestDeduplicatedEntityWithUnrelatedContextKeepsSharedContainers(t *testing.T) {
	for _, tt := range []struct {
		name      string
		mergePath []string
	}{
		{name: "sibling fetch path"},
		{name: "sibling merge path", mergePath: []string{"wallet"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc := astjson.MustParse(`{"employees":[{"id":1,"tier":"FREE","wallet":{"id":1},"notice":{"id":7}},{"id":1,"tier":"PRO","wallet":{"id":1},"notice":{"id":7}}]}`)
			wallet := contextTestBatch(FakeDataSource(`{"data":{"_entities":[{"offers":[{"id":7}]}]}}`), "wallets", 0)
			wallet.PostProcessing.MergePath = tt.mergePath
			fetchPath := []FetchItemPathElement{ArrayPath("employees")}
			if len(tt.mergePath) == 0 {
				fetchPath = append(fetchPath, ObjectPath("wallet"))
			}
			quote := contextTestQuoteFetch(FakeDataSource(`{"data":{"_entities":[{"quote":"ok"}]}}`))
			plan := Sequence(Single(wallet, fetchPath...), Single(quote, ArrayPath("employees"), ObjectPath("notice")))
			loader := Loader{dataBuffer: &DataBuffer{data: doc}}
			ctx := NewContext(t.Context())
			ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
			require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: plan}))
			members := doc.GetArray("employees")
			require.Equal(t, "ok", string(members[1].GetStringBytes("notice", "quote")))
			firstOffers := members[0].Get("wallet", "offers")
			require.Equal(t, `[{"id":7}]`, string(firstOffers.MarshalTo(nil)))
			require.Same(t, firstOffers, members[1].Get("wallet", "offers"))
		})
	}
}

func TestDeduplicatedEntityIsolatesDependentContextWrites(t *testing.T) {
	doc := astjson.MustParse(`{"employees":[{"tier":"FREE","wallet":{"id":1},"notice":{"id":7}},{"tier":"PRO","wallet":{"id":1},"notice":{"id":7}}]}`)
	wallet := contextTestBatch(FakeDataSource(`{"data":{"_entities":[{"offer":{"id":7}}]}}`), "wallets", 0)
	source := NewMockDataSource(gomock.NewController(t))
	source.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"FREE","ids":[7]}`)).
		Return([]byte(`{"data":{"_entities":[{"quote":1}]}}`), nil)
	source.EXPECT().Load(gomock.Any(), gomock.Any(), []byte(`{"tier":"PRO","ids":[7]}`)).
		Return([]byte(`{"data":{"_entities":[{"quote":2}]}}`), nil)
	quote := contextTestQuoteFetch(source)
	// Two ordinary fetches propagate the contextual quote through @requires.
	// The last fetch writes a nested wallet field even though wallet IDs match.
	code := contextTestBatch(FakeDataSource(`{"data":{"_entities":[{"code":20},{"code":40}]}}`), "codes", 2, 1)
	code.Input.Items[0].Segments[0].VariableSourcePath = []string{"quote"}
	price := contextTestBatch(FakeDataSource(`{"data":{"_entities":[{"wallet":{"offer":{"price":5}}},{"wallet":{"offer":{"price":10}}}]}}`), "prices", 3, 2)
	price.Input.Items[0].Segments[0].VariableSourcePath = []string{"notice", "code"}
	member, notice := ArrayPath("employees"), ObjectPath("notice")
	plan := Sequence(Single(wallet, member, ObjectPath("wallet")), Single(quote, member, notice), Single(code, member, notice), Single(price, member))
	loader := Loader{dataBuffer: &DataBuffer{data: doc}}
	ctx := NewContext(t.Context())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: plan}))
	employees := doc.GetArray("employees")
	require.Equal(t, 5, employees[0].GetInt("wallet", "offer", "price"))
	require.Equal(t, 10, employees[1].GetInt("wallet", "offer", "price"))
}
