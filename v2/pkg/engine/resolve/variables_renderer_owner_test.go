package resolve

import (
	"bytes"
	"context"
	"io"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
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
	t.Parallel()

	seller := &Object{
		Path:     []string{"seller"},
		Nullable: true,
		Fields: []*Field{
			{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
		},
	}
	representation := InputTemplate{
		Segments: []TemplateSegment{
			{
				SegmentType:  VariableSegmentType,
				VariableKind: ResolvableObjectVariableKind,
				Renderer: NewGraphQLVariableResolveRenderer(&Object{
					Fields: []*Field{
						{Name: []byte("seller"), Value: seller},
					},
				}),
			},
		},
	}
	multi := twoEntryMultiFetch(nil)
	multi.Input.Entries[1].Representations = representation
	path := ObjectPath("employee")

	cases := []struct {
		name string
		node *FetchTreeNode
	}{
		{"single", Single(&SingleFetch{InputTemplate: representation}, path)},
		{"entity", Single(&EntityFetch{Input: EntityInput{Item: representation}}, path)},
		{"batch entity", Single(&BatchEntityFetch{Input: BatchInput{Items: []InputTemplate{representation}}}, path)},
		{"multi entity", Single(multi)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := &variableTestArena{
				Arena:     arena.NewMonotonicArena(),
				allocated: make(map[unsafe.Pointer]bool),
			}
			data, err := astjson.ParseWithArena(a, `{"employee":{"seller":{"name":"Ada"}}}`)
			require.NoError(t, err)

			loader := &Loader{
				ctx:        NewContext(t.Context()),
				jsonArena:  a,
				dataBuffer: &DataBuffer{data: data},
			}
			prepared, err := loader.preparePhase(tc.node.Item)
			require.NoError(t, err)
			defer batchEntityToolPool.Put(prepared.res.tools)

			assert.Contains(t, string(prepared.input), `{"seller":null}`)
			seller := data.Get("employee", "seller")
			require.NotNil(t, seller)
			assert.True(t, seller == astjson.NullValue || a.allocated[unsafe.Pointer(seller)], "replacement must belong to the input arena or be a shared null")
			assert.Equal(t, astjson.TypeNull, seller.Type())
		})
	}
}

type customArenaRenderer struct {
	JSONVariableRenderer
	received arena.Arena
}

func (r *customArenaRenderer) RenderVariableWithArena(ctx context.Context, a arena.Arena, data *astjson.Value, out io.Writer) error {
	r.received = a
	return r.RenderVariable(ctx, data, out)
}

func TestInputTemplateCustomArenaRenderer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kind VariableKind
	}{
		{"object", ObjectVariableKind},
		{"resolvable object", ResolvableObjectVariableKind},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := arena.NewMonotonicArena()
			data, err := astjson.ParseWithArena(a, `{"id":"1"}`)
			require.NoError(t, err)

			renderer := &customArenaRenderer{}
			template := InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType:  VariableSegmentType,
						VariableKind: tc.kind,
						Renderer:     renderer,
					},
				},
			}

			var out bytes.Buffer
			err = template.RenderWithArena(NewContext(t.Context()), a, data, &out)
			assert.NoError(t, err)
			assert.Same(t, a, renderer.received)
			assert.JSONEq(t, `{"id":"1"}`, out.String())
		})
	}
}
