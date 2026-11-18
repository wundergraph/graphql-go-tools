package resolve

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

type variableTestArena struct {
	arena.Arena
	allocated map[unsafe.Pointer]bool
}

func (a *variableTestArena) Alloc(size, alignment uintptr) unsafe.Pointer {
	p := a.Arena.Alloc(size, alignment)
	a.allocated[p] = true
	return p
}

func TestFetchRepresentationUsesInputArena(t *testing.T) {
	representation := InputTemplate{Segments: []TemplateSegment{{
		SegmentType: VariableSegmentType, VariableKind: ResolvableObjectVariableKind,
		Renderer: NewGraphQLVariableResolveRenderer(&Object{Fields: []*Field{{
			Name: []byte("seller"), Value: &Object{
				Path: []string{"seller"}, Nullable: true, Fields: []*Field{{
					Name: []byte("id"), Value: &String{Path: []string{"id"}},
				}},
			},
		}}}),
	}}}
	multi := twoEntryMultiFetch(nil)
	multi.Input.Entries[1].Representations = representation
	path := ObjectPath("employee")

	for _, tc := range []struct {
		name string
		node *FetchTreeNode
	}{
		{"single", Single(&SingleFetch{InputTemplate: representation}, path)},
		{"entity", Single(&EntityFetch{Input: EntityInput{Item: representation}}, path)},
		{"batch entity", Single(&BatchEntityFetch{Input: BatchInput{Items: []InputTemplate{representation}}}, path)},
		{"multi entity", Single(multi)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &variableTestArena{Arena: arena.NewMonotonicArena(), allocated: make(map[unsafe.Pointer]bool)}
			data, err := astjson.ParseWithArena(a, `{"employee":{"seller":{"name":"Ada"}}}`)
			require.NoError(t, err)
			loader := &Loader{ctx: NewContext(t.Context()), jsonArena: a, dataBuffer: &DataBuffer{data: data}}
			prepared, err := loader.preparePhase(tc.node.Item)
			require.NoError(t, err)
			defer batchEntityToolPool.Put(prepared.res.tools)
			require.Contains(t, string(prepared.input), `{"seller":null}`)
			seller := data.Get("employee", "seller")
			require.True(t, seller == astjson.NullValue || a.allocated[unsafe.Pointer(seller)], "replacement must belong to the input arena or be a shared null")
			require.Equal(t, astjson.TypeNull, seller.Type())
		})
	}
}
