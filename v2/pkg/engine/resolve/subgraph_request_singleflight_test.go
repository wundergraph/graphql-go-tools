package resolve

import (
	"bytes"
	"fmt"
	"testing"
)

type stubFetch struct {
	info *FetchInfo
}

func (s *stubFetch) FetchKind() FetchKind {
	return FetchKindSingle
}

func (s *stubFetch) Dependencies() *FetchDependencies {
	return nil
}

func (s *stubFetch) FetchInfo() *FetchInfo {
	return s.info
}

type nilInfoFetch struct{}

func (n *nilInfoFetch) FetchKind() FetchKind {
	return FetchKindSingle
}

func (n *nilInfoFetch) Dependencies() *FetchDependencies {
	return nil
}

func (n *nilInfoFetch) FetchInfo() *FetchInfo {
	return nil
}

func newFetchItem(info *FetchInfo) *FetchItem {
	return &FetchItem{
		Fetch: &stubFetch{
			info: info,
		},
	}
}

func TestSubgraphRequestSingleFlight_LeaderFollowerSizeHint(t *testing.T) {
	flight := NewSingleFlight(2)
	fetchInfo := &FetchInfo{
		DataSourceID: "accounts",
		RootFields: []GraphCoordinate{
			{TypeName: "Query", FieldName: "viewer"},
		},
	}
	fetchItem := newFetchItem(fetchInfo)

	item, shared := flight.GetOrCreateItem(fetchItem, []byte("query { viewer { id } }"), 42)
	if shared {
		t.Fatalf("expected leader to be first caller")
	}
	if item == nil {
		t.Fatalf("expected item, got nil")
	}
	if item.sizeHint != 0 {
		t.Fatalf("expected empty size hint, got %d", item.sizeHint)
	}

	follower, followerShared := flight.GetOrCreateItem(fetchItem, []byte("query { viewer { id } }"), 42)
	if !followerShared {
		t.Fatalf("expected second caller to be follower")
	}
	if follower != item {
		t.Fatalf("expected follower to receive same item instance")
	}

	item.response = []byte("hello")
	flight.Finish(item)

	select {
	case <-item.loaded:
	default:
		t.Fatalf("expected leader to close loaded channel")
	}

	next, nextShared := flight.GetOrCreateItem(fetchItem, []byte("query { viewer { id } }"), 42)
	if nextShared {
		t.Fatalf("expected new leader after finish")
	}
	if next == item {
		t.Fatalf("expected new item after finish")
	}
	if next.sizeHint != len("hello") {
		t.Fatalf("expected size hint %d, got %d", len("hello"), next.sizeHint)
	}
}

func TestSubgraphRequestSingleFlight_SimilarFetchesShareFetchKey(t *testing.T) {
	flight := NewSingleFlight(1)
	fetchInfo := &FetchInfo{
		DataSourceID: "reviews",
		RootFields: []GraphCoordinate{
			{TypeName: "Query", FieldName: "reviews"},
		},
	}
	fetchItem := newFetchItem(fetchInfo)

	item1, shared1 := flight.GetOrCreateItem(fetchItem, []byte("body-1"), 0)
	if shared1 {
		t.Fatalf("expected first call to be leader")
	}
	item1.response = []byte("first response")
	flight.Finish(item1)

	item2, shared2 := flight.GetOrCreateItem(fetchItem, []byte("body-2"), 0)
	if shared2 {
		t.Fatalf("expected leader after finishing previous item")
	}
	if item1.FetchKey != item2.FetchKey {
		t.Fatalf("expected identical fetch keys for similar fetches")
	}
	if item1.SFKey == item2.SFKey {
		t.Fatalf("expected different single-flight keys for different request bodies")
	}
	item2.response = []byte("second response")
	flight.Finish(item2)
}

func TestSubgraphRequestSingleFlight_FetchKeyZeroWithoutFetchInfo(t *testing.T) {
	t.Run("nil fetch item", func(t *testing.T) {
		flight := NewSingleFlight(1)
		item, shared := flight.GetOrCreateItem(nil, []byte("body"), 0)
		if shared {
			t.Fatalf("expected leader for nil fetch item")
		}
		if item.FetchKey != 0 {
			t.Fatalf("expected fetch key 0, got %d", item.FetchKey)
		}
		flight.Finish(item)
	})

	t.Run("nil fetch", func(t *testing.T) {
		flight := NewSingleFlight(1)
		item, shared := flight.GetOrCreateItem(&FetchItem{}, []byte("body"), 0)
		if shared {
			t.Fatalf("expected leader for nil fetch")
		}
		if item.FetchKey != 0 {
			t.Fatalf("expected fetch key 0, got %d", item.FetchKey)
		}
		flight.Finish(item)
	})

	t.Run("missing fetch info", func(t *testing.T) {
		flight := NewSingleFlight(1)
		item, shared := flight.GetOrCreateItem(&FetchItem{Fetch: &nilInfoFetch{}}, []byte("body"), 0)
		if shared {
			t.Fatalf("expected leader for missing fetch info")
		}
		if item.FetchKey != 0 {
			t.Fatalf("expected fetch key 0, got %d", item.FetchKey)
		}
		flight.Finish(item)
	})
}

func TestSubgraphRequestSingleFlight_SizeHintRollingWindow(t *testing.T) {
	flight := NewSingleFlight(1)
	fetchInfo := &FetchInfo{
		DataSourceID: "products",
		RootFields: []GraphCoordinate{
			{TypeName: "Query", FieldName: "products"},
		},
	}
	fetchItem := newFetchItem(fetchInfo)

	var fetchKey uint64
	for i := 0; i < 50; i++ {
		item, shared := flight.GetOrCreateItem(fetchItem, []byte(fmt.Sprintf("body-%d", i)), 0)
		if shared {
			t.Fatalf("expected leader for iteration %d", i)
		}
		if i == 0 {
			fetchKey = item.FetchKey
		} else if item.FetchKey != fetchKey {
			t.Fatalf("expected consistent fetch key across iterations, got %d and %d", fetchKey, item.FetchKey)
		}
		item.response = bytes.Repeat([]byte("a"), 100)
		flight.Finish(item)
	}

	item, shared := flight.GetOrCreateItem(fetchItem, []byte("body-50"), 0)
	if shared {
		t.Fatalf("expected leader for rolling window update")
	}
	if item.FetchKey != fetchKey {
		t.Fatalf("expected same fetch key, got %d and %d", fetchKey, item.FetchKey)
	}
	item.response = bytes.Repeat([]byte("b"), 200)
	flight.Finish(item)

	next, nextShared := flight.GetOrCreateItem(fetchItem, []byte("body-51"), 0)
	if nextShared {
		t.Fatalf("expected leader for new request")
	}
	expected := 150
	if next.sizeHint != expected {
		t.Fatalf("expected rolling average size hint %d, got %d", expected, next.sizeHint)
	}
}
