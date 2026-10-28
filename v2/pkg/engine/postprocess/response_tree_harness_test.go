package postprocess

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/kylelemons/godebug/diff"
	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// responseTreePrinter prints a response tree in a diffable form.
// Type name lists print sorted, because deduplicateOnTypeNames reads them out of a map.
var responseTreePrinter = &pretty.Config{
	Diffable:          true,
	IncludeUnexported: false,
	Formatter: map[reflect.Type]any{
		reflect.TypeFor[[]byte](): func(b []byte) string { return fmt.Sprintf("%q", b) },
		reflect.TypeFor[[][]byte](): func(b [][]byte) string {
			values := make([]string, 0, len(b))
			for _, v := range b {
				values = append(values, string(v))
			}
			slices.Sort(values)
			printed, _ := json.Marshal(values)
			return string(printed)
		},
		reflect.TypeFor[map[string]struct{}](): func(m map[string]struct{}) string {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			printed, _ := json.Marshal(keys)
			return string(printed)
		},
	},
}

// assertResponseTreeEqual compares two response trees and prints a line diff on a mismatch.
func assertResponseTreeEqual(t *testing.T, expected, actual resolve.Node) {
	t.Helper()
	want := responseTreePrinter.Sprint(expected)
	got := responseTreePrinter.Sprint(actual)
	if want == got {
		return
	}
	t.Errorf("response tree mismatch (-want +got)\n%s", diff.Diff(want, got))
}

// renderResponse resolves the tree against one JSON input and returns the data object as compact JSON.
func renderResponse(t *testing.T, root *resolve.Object, input string) string {
	t.Helper()
	r := resolve.NewResolvable(nil, resolve.ResolvableOptions{})
	require.NoError(t, r.Init(&resolve.Context{}, []byte(input), ast.OperationTypeQuery))
	var out bytes.Buffer
	require.NoError(t, r.Resolve(t.Context(), root, nil, &out))
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &envelope))
	assert.NotContains(t, out.String(), `"errors"`)
	return string(envelope.Data)
}
