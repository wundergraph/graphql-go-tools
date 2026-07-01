package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"
)

func TestDecisionString(t *testing.T) {
	assert.Equal(t, "Fetch", DecisionFetch.String())
	assert.Equal(t, "SkipFullHit", DecisionSkipFullHit.String())
	assert.Equal(t, "FetchPartial", DecisionFetchPartial.String())
	assert.Equal(t, "FetchShadow", DecisionFetchShadow.String())
	assert.Equal(t, "Decision(9)", Decision(9).String())
}

func TestFetchCacheHandleString(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var h *FetchCacheHandle
		assert.Equal(t, "<nil>", h.String())
	})

	t.Run("zero value", func(t *testing.T) {
		assert.Equal(t, "{decision:Fetch items:0 hits:0 writeback:0 shadow:false}", (&FetchCacheHandle{}).String())
	})

	t.Run("populated", func(t *testing.T) {
		cached := astjson.MustParseBytes([]byte(`{"__typename":"Product","upc":"1"}`))
		h := &FetchCacheHandle{
			Decision: DecisionSkipFullHit,
			WasHit:   true,
			Items: []ItemCacheState{
				{FromCache: cached},
				{FromCache: cached, NeedsWriteback: true},
				{},
			},
		}
		assert.Equal(t, "{decision:SkipFullHit items:3 hits:2 writeback:1 shadow:false}", h.String())
	})

	t.Run("shadow", func(t *testing.T) {
		h := &FetchCacheHandle{
			Decision: DecisionFetchShadow,
			Shadow:   true,
			Items:    []ItemCacheState{{}},
		}
		assert.Equal(t, "{decision:FetchShadow items:1 hits:0 writeback:0 shadow:true}", h.String())
	})
}
