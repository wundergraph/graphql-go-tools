package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
)

// TestJSONBuilder_MergeWithPath_NullParents tests that null values along the response path
// are skipped when merging resolver results. The context of a resolver call skips null
// parents, so the resolved values only line up with the non-null parents.
func TestJSONBuilder_MergeWithPath_NullParents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		base     string
		resolved string
		path     string
		expected string
	}{
		{
			name:     "Should skip a null list field between non-null ones",
			base:     `{"parents":[{"children":[{"id":"a"},{"id":"b"}]},{"children":null},{"children":[{"id":"c"}]}]}`,
			resolved: `{"result":[{"value":1},{"value":2},{"value":3}]}`,
			path:     "parents.children.value",
			expected: `{"parents":[{"children":[{"id":"a","value":1},{"id":"b","value":2}]},{"children":null},{"children":[{"id":"c","value":3}]}]}`,
		},
		{
			name:     "Should skip a null object field between non-null ones",
			base:     `{"parents":[{"child":null},{"child":{"id":"a"}},{"child":null},{"child":{"id":"b"}}]}`,
			resolved: `{"result":[{"value":1},{"value":2}]}`,
			path:     "parents.child.value",
			expected: `{"parents":[{"child":null},{"child":{"id":"a","value":1}},{"child":null},{"child":{"id":"b","value":2}}]}`,
		},
		{
			name:     "Should skip null list items between non-null ones",
			base:     `{"parents":[{"id":"a"},null,{"id":"b"}]}`,
			resolved: `{"result":[{"value":1},{"value":2}]}`,
			path:     "parents.value",
			expected: `{"parents":[{"id":"a","value":1},null,{"id":"b","value":2}]}`,
		},
		{
			name:     "Should skip null list items below a list field",
			base:     `{"parents":[{"children":[null,{"id":"a"}]},{"children":[{"id":"b"},null]}]}`,
			resolved: `{"result":[{"value":1},{"value":2}]}`,
			path:     "parents.children.value",
			expected: `{"parents":[{"children":[null,{"id":"a","value":1}]},{"children":[{"id":"b","value":2},null]}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base := astjson.MustParse(tt.base)
			resolved := astjson.MustParse(tt.resolved)

			builder := newJSONBuilder(nil, nil)
			require.NoError(t, builder.mergeWithPath(base, resolved, buildPath(tt.path), nil))
			require.JSONEq(t, tt.expected, base.String())
		})
	}
}

// TestJSONBuilder_MergeWithPath_LengthMismatch tests that a resolver result must have
// one value for each parent.
func TestJSONBuilder_MergeWithPath_LengthMismatch(t *testing.T) {
	t.Parallel()

	t.Run("empty resolver result returns an error", func(t *testing.T) {
		t.Parallel()

		base := astjson.MustParse(`{"parents":[{"id":"a"},{"id":"b"}]}`)
		resolved := astjson.MustParse(`{"result":[]}`)

		err := newJSONBuilder(nil, nil).mergeWithPath(base, resolved, buildPath("parents.value"), nil)
		require.EqualError(t, err, "length of values doesn't match the length of the result array, expected 2, got 0")
	})

	t.Run("short resolver result returns an error", func(t *testing.T) {
		t.Parallel()

		base := astjson.MustParse(`{"parents":[{"id":"a"},{"id":"b"}]}`)
		resolved := astjson.MustParse(`{"result":[{"value":1}]}`)

		err := newJSONBuilder(nil, nil).mergeWithPath(base, resolved, buildPath("parents.value"), nil)
		require.EqualError(t, err, "length of values doesn't match the length of the result array, expected 2, got 1")
	})
}

// TestJSONBuilder_MergeWithPath_MissingRootField tests that a missing root field
// returns an error instead of a panic.
func TestJSONBuilder_MergeWithPath_MissingRootField(t *testing.T) {
	t.Parallel()

	base := astjson.MustParse(`{"other":[{"id":"a"}]}`)
	resolved := astjson.MustParse(`{"result":[]}`)

	var err error
	require.NotPanics(t, func() {
		err = newJSONBuilder(nil, nil).mergeWithPath(base, resolved, buildPath("parents.value"), nil)
	})
	require.EqualError(t, err, "field parents not found in object")
}
