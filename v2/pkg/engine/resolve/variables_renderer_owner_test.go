package resolve

import (
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

	a := &variableTestArena{Arena: arena.NewMonotonicArena(), allocated: make(map[unsafe.Pointer]bool)}
	data, err := astjson.ParseWithArena(a, `{"seller":{"name":"Ada"}}`)
	require.NoError(t, err)

	seller := &Object{
		Path:     []string{"seller"},
		Nullable: true,
		Fields:   []*Field{{Name: []byte("id"), Value: &String{Path: []string{"id"}}}},
	}
	variable := NewResolvableObjectVariable(&Object{Fields: []*Field{{Name: []byte("seller"), Value: seller}}})
	item := InputTemplate{Segments: []TemplateSegment{variable.TemplateSegment()}}
	fetch := Single(&EntityFetch{Input: EntityInput{Item: item}})
	loader := Loader{ctx: NewContext(t.Context()), jsonArena: a, dataBuffer: &DataBuffer{data: data}}

	prepared, err := loader.preparePhase(fetch.Item)
	require.NoError(t, err)
	assert.Equal(t, `{"seller":null}`, string(prepared.input))
	replacement := data.Get("seller")
	assert.True(t, replacement == astjson.NullValue || a.allocated[unsafe.Pointer(replacement)], "replacement must belong to the input arena or be a shared null")
}
