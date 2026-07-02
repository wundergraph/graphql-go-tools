package resolve

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func resolveWith(t *testing.T, behavior ErrorBehavior, data string, root *Object) string {
	t.Helper()
	res := NewResolvable(nil, ResolvableOptions{})
	err := res.Init(&Context{ExecutionOptions: ExecutionOptions{ErrorBehavior: behavior}},
		[]byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)
	out := &bytes.Buffer{}
	assert.NoError(t, res.Resolve(context.Background(), root, nil, out))
	return out.String()
}

func TestErrorBehaviorMatrix(t *testing.T) {
	// tree: { hero: { id (nn String), name (nn String) }, time (nullable String) }
	tree := func(heroNullable bool) *Object {
		return &Object{Fields: []*Field{
			{Name: []byte("hero"), Value: &Object{Path: []string{"hero"}, Nullable: heroNullable, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}},
			{Name: []byte("time"), Value: &String{Path: []string{"time"}, Nullable: true}},
		}}
	}
	names := func() *Object {
		return &Object{Fields: []*Field{{Name: []byte("names"), Value: &Array{Path: []string{"names"}, Item: &String{}}}}}
	}

	cases := []struct {
		name     string
		behavior ErrorBehavior
		data     string
		root     *Object
		want     string
	}{
		// ---- non-null leaf null ----
		{"propagate/leaf-null/hero-nonnull", ErrorBehaviorPropagate,
			`{"hero":{"id":"1","name":null},"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":null}`},
		{"propagate/leaf-null/hero-nullable", ErrorBehaviorPropagate,
			`{"hero":{"id":"1","name":null},"time":"now"}`, tree(true),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":{"hero":null,"time":"now"}}`},
		{"null/leaf-null", ErrorBehaviorNull,
			`{"hero":{"id":"1","name":null},"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":{"hero":{"id":"1","name":null},"time":"now"}}`},
		{"halt/leaf-null", ErrorBehaviorHalt,
			`{"hero":{"id":"1","name":null},"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}],"data":null}`},

		// ---- non-null object null ----
		{"propagate/object-null/hero-nonnull", ErrorBehaviorPropagate,
			`{"hero":null,"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero'.","path":["hero"]}],"data":null}`},
		{"null/object-null", ErrorBehaviorNull,
			`{"hero":null,"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero'.","path":["hero"]}],"data":{"hero":null,"time":"now"}}`},
		{"halt/object-null", ErrorBehaviorHalt,
			`{"hero":null,"time":"now"}`, tree(false),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.hero'.","path":["hero"]}],"data":null}`},

		// ---- non-null list item null ([String!]! list) ----
		{"null/list-item-null", ErrorBehaviorNull,
			`{"names":["a",null,"c"]}`, names(),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.names'.","path":["names",1]}],"data":{"names":["a",null,"c"]}}`},
		{"propagate/list-item-null", ErrorBehaviorPropagate,
			`{"names":["a",null,"c"]}`, names(),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.names'.","path":["names",1]}],"data":null}`},
		{"halt/list-item-null", ErrorBehaviorHalt,
			`{"names":["a",null,"c"]}`, names(),
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.names'.","path":["names",1]}],"data":null}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, resolveWith(t, c.behavior, c.data, c.root))
		})
	}
}

// Regression (Codex finding 1): a non-null Float list item with a type mismatch
// under NULL must render null in place, never leak the raw invalid value.
func TestResolvable_NullBehavior_FloatListItemTypeMismatch(t *testing.T) {
	root := &Object{Fields: []*Field{{
		Name:  []byte("values"),
		Value: &Array{Path: []string{"values"}, Item: &Float{}}, // [Float!]!
	}}}
	got := resolveWith(t, ErrorBehaviorNull, `{"values":[1.5,"bad",3.5]}`, root)
	assert.Equal(t,
		`{"errors":[{"message":"Float cannot represent non-float value: \"\"bad\"\"","path":["values",1]}],"data":{"values":[1.5,null,3.5]}}`,
		got)
}

// Regression (Codex finding 3): a non-null object with an invalid abstract
// __typename under NULL must render null in place, not propagate.
func TestResolvable_NullBehavior_InvalidTypename(t *testing.T) {
	root := &Object{Fields: []*Field{
		{Name: []byte("hero"), Value: &Object{
			Path:          []string{"hero"},
			TypeName:      "Character",
			PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
			Fields: []*Field{
				{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: true}},
			},
		}}, // non-null object
		{Name: []byte("time"), Value: &String{Path: []string{"time"}, Nullable: true}},
	}}
	got := resolveWith(t, ErrorBehaviorNull, `{"hero":{"__typename":"Alien","name":"x"},"time":"now"}`, root)
	assert.Equal(t,
		`{"errors":[{"message":"Subgraph '' returned invalid value 'Alien' for __typename field.","path":["hero"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"hero":null,"time":"now"}}`,
		got)
}

func TestHalt_TrimsToSingleError(t *testing.T) {
	// two sibling non-null leaves both null -> PROPAGATE would still bubble to
	// data:null; HALT must additionally guarantee exactly ONE error entry.
	root := &Object{Fields: []*Field{
		{Name: []byte("a"), Value: &String{Path: []string{"a"}}},
		{Name: []byte("b"), Value: &String{Path: []string{"b"}}},
	}}
	got := resolveWith(t, ErrorBehaviorHalt, `{"a":null,"b":null}`, root)
	assert.Equal(t,
		`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.a'.","path":["a"]}],"data":null}`,
		got)
}

// With ErrorBehavior unset, output must match the explicit PROPAGATE output.
func TestErrorBehavior_UnsetEqualsPropagate(t *testing.T) {
	data := `{"hero":{"id":"1","name":null},"time":"now"}`
	root := func() *Object {
		return &Object{Fields: []*Field{
			{Name: []byte("hero"), Value: &Object{Path: []string{"hero"}, Nullable: true, Fields: []*Field{
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
			}}},
			{Name: []byte("time"), Value: &String{Path: []string{"time"}, Nullable: true}},
		}}
	}
	unset := resolveWith(t, "", data, root())
	propagate := resolveWith(t, ErrorBehaviorPropagate, data, root())
	assert.Equal(t, propagate, unset)
}
