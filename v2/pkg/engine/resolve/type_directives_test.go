package resolve

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type testObjectDirectiveHandler struct {
	onObject func(ctx *Context, directive *TypeDirective, path []string, object *astjson.Value) (ObjectDirectiveResult, error)
}

func (h *testObjectDirectiveHandler) OnObject(ctx *Context, directive *TypeDirective, path []string, object *astjson.Value) (ObjectDirectiveResult, error) {
	return h.onObject(ctx, directive, path, object)
}

// ownerHandler keeps objects whose hidden "_owner" key is "u1" and nulls the others.
func ownerHandler(calls *[]string) *testObjectDirectiveHandler {
	return &testObjectDirectiveHandler{
		onObject: func(_ *Context, directive *TypeDirective, path []string, object *astjson.Value) (ObjectDirectiveResult, error) {
			*calls = append(*calls, directive.TypeName+"@"+joinPath(path))
			if string(object.GetStringBytes("_owner")) == "u1" {
				return ObjectDirectiveResult{}, nil
			}
			return ObjectDirectiveResult{Null: true, Error: &ObjectDirectiveError{Message: "Forbidden", Code: "FORBIDDEN"}}, nil
		},
	}
}

func joinPath(path []string) string {
	var b bytes.Buffer
	for i, p := range path {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(p)
	}
	return b.String()
}

func documentObject(path []string, nullable bool, directives map[string][]*TypeDirective) *Object {
	return &Object{
		Path:           path,
		Nullable:       nullable,
		TypeName:       "Document",
		PossibleTypes:  map[string]struct{}{"Document": {}},
		TypeDirectives: directives,
		Fields: []*Field{
			{Name: []byte("title"), Value: &String{Path: []string{"title"}}},
		},
	}
}

func resolveWithObjectDirectives(t *testing.T, input string, root *Object, handler ObjectDirectiveHandler) string {
	t.Helper()
	res := NewResolvable(nil, ResolvableOptions{})
	ctx := &Context{}
	ctx.SetObjectDirectiveHandler(handler)
	require.NoError(t, res.Init(ctx, []byte(input), ast.OperationTypeQuery))
	out := &bytes.Buffer{}
	require.NoError(t, res.Resolve(context.Background(), root, nil, out))
	return out.String()
}

func TestResolvable_TypeDirectives(t *testing.T) {
	t.Parallel()

	directives := map[string][]*TypeDirective{
		"Document": {{Name: "ownedBy", TypeName: "Document"}},
	}

	t.Run("nullable object field is nulled with an error", func(t *testing.T) {
		t.Parallel()
		var calls []string
		root := &Object{Fields: []*Field{
			{Name: []byte("mine"), Value: documentObject([]string{"mine"}, true, directives)},
			{Name: []byte("other"), Value: documentObject([]string{"other"}, true, directives)},
		}}
		out := resolveWithObjectDirectives(t,
			`{"mine":{"title":"a","_owner":"u1"},"other":{"title":"b","_owner":"u2"}}`,
			root, ownerHandler(&calls))
		assert.Equal(t, `{"errors":[{"message":"Forbidden","path":["other"],"extensions":{"code":"FORBIDDEN"}}],"data":{"mine":{"title":"a"},"other":null}}`, out)
		assert.Equal(t, []string{"Document@mine", "Document@other"}, calls)
	})

	t.Run("nullable list items are nulled individually", func(t *testing.T) {
		t.Parallel()
		var calls []string
		root := &Object{Fields: []*Field{
			{Name: []byte("documents"), Value: &Array{
				Path: []string{"documents"},
				Item: documentObject(nil, true, directives),
			}},
		}}
		out := resolveWithObjectDirectives(t,
			`{"documents":[{"title":"a","_owner":"u1"},{"title":"b","_owner":"u2"}]}`,
			root, ownerHandler(&calls))
		assert.Equal(t, `{"errors":[{"message":"Forbidden","path":["documents",1],"extensions":{"code":"FORBIDDEN"}}],"data":{"documents":[{"title":"a"},null]}}`, out)
		assert.Equal(t, []string{"Document@documents.0", "Document@documents.1"}, calls)
	})

	t.Run("union items are matched by __typename", func(t *testing.T) {
		t.Parallel()
		var calls []string
		item := &Object{
			Nullable:       true,
			TypeName:       "SearchResult",
			PossibleTypes:  map[string]struct{}{"Document": {}, "Folder": {}},
			TypeDirectives: directives,
			Fields: []*Field{
				{Name: []byte("title"), Value: &String{Path: []string{"title"}}, OnTypeNames: [][]byte{[]byte("Document")}},
				{Name: []byte("name"), Value: &String{Path: []string{"name"}}, OnTypeNames: [][]byte{[]byte("Folder")}},
			},
		}
		root := &Object{Fields: []*Field{
			{Name: []byte("search"), Value: &Array{Path: []string{"search"}, Item: item}},
		}}
		out := resolveWithObjectDirectives(t,
			`{"search":[{"__typename":"Folder","name":"f"},{"__typename":"Document","title":"b","_owner":"u2"}]}`,
			root, ownerHandler(&calls))
		assert.Equal(t, `{"errors":[{"message":"Forbidden","path":["search",1],"extensions":{"code":"FORBIDDEN"}}],"data":{"search":[{"name":"f"},null]}}`, out)
		assert.Equal(t, []string{"Document@search.1"}, calls)
	})

	t.Run("non-null object bubbles to the nearest nullable parent", func(t *testing.T) {
		t.Parallel()
		var calls []string
		comment := &Object{
			Path:     []string{"comment"},
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("text"), Value: &String{Path: []string{"text"}}},
				{Name: []byte("document"), Value: documentObject([]string{"document"}, false, directives)},
			},
		}
		root := &Object{Fields: []*Field{{Name: []byte("comment"), Value: comment}}}
		out := resolveWithObjectDirectives(t,
			`{"comment":{"text":"hi","document":{"title":"b","_owner":"u2"}}}`,
			root, ownerHandler(&calls))
		assert.Equal(t, `{"errors":[{"message":"Forbidden","path":["comment","document"],"extensions":{"code":"FORBIDDEN"}}],"data":{"comment":null}}`, out)
	})

	t.Run("handler error fails closed", func(t *testing.T) {
		t.Parallel()
		handler := &testObjectDirectiveHandler{
			onObject: func(*Context, *TypeDirective, []string, *astjson.Value) (ObjectDirectiveResult, error) {
				return ObjectDirectiveResult{}, errors.New("boom")
			},
		}
		root := &Object{Fields: []*Field{
			{Name: []byte("doc"), Value: documentObject([]string{"doc"}, true, directives)},
		}}
		out := resolveWithObjectDirectives(t, `{"doc":{"title":"a","_owner":"u1"}}`, root, handler)
		assert.Equal(t, `{"errors":[{"message":"Failed to evaluate directive @ownedBy.","path":["doc"],"extensions":{"code":"INTERNAL_SERVER_ERROR"}}],"data":{"doc":null}}`, out)
	})

	t.Run("objects without directives for their type are not passed to the handler", func(t *testing.T) {
		t.Parallel()
		var calls []string
		root := &Object{Fields: []*Field{
			{Name: []byte("doc"), Value: documentObject([]string{"doc"}, true, map[string][]*TypeDirective{
				"Folder": {{Name: "ownedBy", TypeName: "Folder"}},
			})},
		}}
		out := resolveWithObjectDirectives(t, `{"doc":{"title":"a","_owner":"u2"}}`, root, ownerHandler(&calls))
		assert.Equal(t, `{"data":{"doc":{"title":"a"}}}`, out)
		assert.Empty(t, calls)
	})
}
