package resolve

import (
	"bytes"
	"context"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// The known xxhash collision pair, see caching.TestKeyCollisionResistance.
const (
	collidingRepA = "user-ba3756407998bfc3"
	collidingRepB = "user-70ca41f165edbbb0"
)

// plainIDTemplate renders an item's id unquoted, so the rendered bytes are
// exactly the id and a colliding pair can be fed through as items.
func plainIDTemplate() InputTemplate {
	return InputTemplate{Segments: []TemplateSegment{{
		SegmentType:        VariableSegmentType,
		VariableKind:       ObjectVariableKind,
		VariableSourcePath: []string{"id"},
		Renderer:           NewPlainVariableRenderer(),
	}}}
}

func TestPrepareBatchEntityFetchDedup(t *testing.T) {
	require.Equal(t, xxhash.Sum64String(collidingRepA), xxhash.Sum64String(collidingRepB), "the pair must collide under xxhash")

	static := func(s string) InputTemplate {
		return InputTemplate{Segments: []TemplateSegment{{SegmentType: StaticSegmentType, Data: []byte(s)}}}
	}
	fetch := &BatchEntityFetch{
		Input: BatchInput{
			Header:    static("["),
			Items:     []InputTemplate{plainIDTemplate()},
			Separator: static(","),
			Footer:    static("]"),
		},
	}
	prepare := func(t *testing.T, doc string) (*result, *preparedFetch, []*astjson.Value) {
		t.Helper()
		items := astjson.MustParse(doc).GetArray()
		loader := &Loader{ctx: NewContext(context.Background())}
		res, prepared := &result{}, &preparedFetch{}
		require.NoError(t, loader.prepareBatchEntityFetch(&FetchItem{}, fetch, items, res, prepared))
		return res, prepared, items
	}

	t.Run("equal representations share one slot", func(t *testing.T) {
		res, prepared, items := prepare(t, `[{"id":"a"},{"id":"b"},{"id":"a"}]`)
		require.Equal(t, "[a,b]", string(prepared.input))
		require.Equal(t, [][]*astjson.Value{{items[0], items[2]}, {items[1]}}, res.batchStats)
	})

	t.Run("representations colliding under the 64-bit hash keep their own slots", func(t *testing.T) {
		res, prepared, items := prepare(t, `[{"id":"`+collidingRepA+`"},{"id":"`+collidingRepB+`"},{"id":"`+collidingRepB+`"}]`)
		require.Equal(t, "["+collidingRepA+","+collidingRepB+"]", string(prepared.input))
		require.Equal(t, [][]*astjson.Value{{items[0]}, {items[1], items[2]}}, res.batchStats)
	})
}

func TestRenderEntryRepresentationsDedup(t *testing.T) {
	render := func(t *testing.T, doc string) (*result, []byte, []*astjson.Value) {
		t.Helper()
		items := astjson.MustParse(doc).GetArray()
		loader := &Loader{ctx: NewContext(context.Background())}
		tools := batchEntityToolPool.Get(len(items))
		defer batchEntityToolPool.Put(tools)
		entry := &MultiEntityFetchEntry{Representations: plainIDTemplate()}
		res := &result{}
		rendered, err := loader.renderEntryRepresentations(entry, res, items, arena.NewArenaBuffer(tools.a), tools)
		require.NoError(t, err)
		require.True(t, rendered.entryIncluded)
		// The buffer is arena-backed and the deferred Put resets the arena.
		return res, bytes.Clone(rendered.representationBuffer), items
	}

	t.Run("equal representations share one slot", func(t *testing.T) {
		res, reps, items := render(t, `[{"id":"a"},{"id":"b"},{"id":"a"}]`)
		require.Equal(t, "a,b", string(reps))
		require.Equal(t, [][]*astjson.Value{{items[0], items[2]}, {items[1]}}, res.batchStats)
	})

	t.Run("representations colliding under the 64-bit hash keep their own slots", func(t *testing.T) {
		res, reps, items := render(t, `[{"id":"`+collidingRepA+`"},{"id":"`+collidingRepB+`"},{"id":"`+collidingRepB+`"}]`)
		require.Equal(t, collidingRepA+","+collidingRepB, string(reps))
		require.Equal(t, [][]*astjson.Value{{items[0]}, {items[1], items[2]}}, res.batchStats)
	})
}
