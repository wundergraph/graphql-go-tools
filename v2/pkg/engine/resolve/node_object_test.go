package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

var (
	pathItem1 = ast.PathItem{Kind: ast.FieldName, FieldName: []byte("query")}
	pathItem2 = ast.PathItem{Kind: ast.FieldName, FieldName: []byte("object1")}
	pathItem3 = ast.PathItem{Kind: ast.ArrayIndex, ArrayIndex: 3, FieldName: []byte("field1")}

	fragmentItem = ast.PathItem{Kind: ast.InlineFragmentName, FieldName: []byte("frag2"), FragmentRef: 2}
	arrayItem    = ast.PathItem{Kind: ast.ArrayIndex, ArrayIndex: 3, FieldName: []byte("arrayField3")}

	emptyPath  = ast.Path{}
	shortPath1 = ast.Path{pathItem1, pathItem2}
	shortPath2 = ast.Path{pathItem2, pathItem3}
	longPath   = ast.Path{pathItem1, pathItem2, pathItem3} // shortPath1 + pathItem3
)

func TestDeferInfo_Equals(t *testing.T) {
	tests := []struct {
		name          string
		first, second *DeferInfo
		expected      bool
	}{
		{
			name:     "equal",
			first:    &DeferInfo{Path: longPath},
			second:   &DeferInfo{Path: longPath},
			expected: true,
		},
		{
			name:     "zero-valued equal",
			first:    &DeferInfo{},
			second:   &DeferInfo{},
			expected: true,
		},
		{
			name:     "empty paths equal",
			first:    &DeferInfo{Path: emptyPath},
			second:   &DeferInfo{Path: emptyPath},
			expected: true,
		},
		{
			name:     "both nil equal",
			first:    nil,
			second:   nil,
			expected: true,
		},
		{
			name:     "not equal",
			first:    &DeferInfo{Path: shortPath1},
			second:   &DeferInfo{Path: shortPath2},
			expected: false,
		},
		{
			name:     "one nil not equal empty",
			first:    nil,
			second:   &DeferInfo{},
			expected: false,
		},
		{
			name:     "not equal - one empty path",
			first:    &DeferInfo{Path: emptyPath},
			second:   &DeferInfo{Path: shortPath1},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.first.Equals(tt.second))
			assert.Equal(t, tt.expected, tt.second.Equals(tt.first))
		})
	}
}

func TestDeferInfo_Overlaps(t *testing.T) {
	tests := []struct {
		name      string
		input     *DeferInfo
		otherPath ast.Path
		expected  bool
	}{
		{
			name:      "overlaps - equal paths",
			input:     &DeferInfo{Path: shortPath1},
			otherPath: shortPath1,
			expected:  true,
		},
		{
			name:      "overlaps - shorter path",
			input:     &DeferInfo{Path: longPath},
			otherPath: shortPath1,
			expected:  true,
		},
		{
			name:      "overlaps - longer path",
			input:     &DeferInfo{Path: shortPath1},
			otherPath: longPath,
			expected:  true,
		},
		{
			name:      "overlaps - shorter path, mismatched",
			input:     &DeferInfo{Path: shortPath2},
			otherPath: longPath,
			expected:  false,
		},
		{
			name:      "overlaps - longer path, mismatched",
			input:     &DeferInfo{Path: longPath},
			otherPath: shortPath2,
			expected:  false,
		},
		{
			name:      "non-overlapping paths",
			input:     &DeferInfo{Path: shortPath1},
			otherPath: shortPath2,
			expected:  false,
		},
		{
			name:      "empty paths equal",
			input:     &DeferInfo{Path: emptyPath},
			otherPath: emptyPath,
			expected:  true,
		},
		{
			name:      "empty defer path",
			input:     &DeferInfo{Path: emptyPath},
			otherPath: shortPath1,
			expected:  true,
		},
		{
			name:      "nil DeferInfo - empty path",
			input:     nil,
			otherPath: emptyPath,
			expected:  false,
		},
		{
			name:      "nil DeferInfo - non-empty path",
			input:     nil,
			otherPath: shortPath1,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.input.Overlaps(tt.otherPath))
			// TODO: reflexive property test.
		})
	}
}
func TestDeferInfo_HasPrefix(t *testing.T) {
	tests := []struct {
		name      string
		deferInfo *DeferInfo
		prefix    []string
		expected  bool
	}{
		{
			name:      "empty prefix always returns true",
			deferInfo: &DeferInfo{Path: shortPath1},
			prefix:    []string{},
			expected:  true,
		},
		{
			name:      "non-empty prefix with empty DeferInfo path returns false",
			deferInfo: &DeferInfo{Path: emptyPath},
			prefix:    []string{"query"},
			expected:  false,
		},
		{
			name:      "exact match short path",
			deferInfo: &DeferInfo{Path: shortPath1},
			prefix:    []string{"query", "object1"},
			expected:  true,
		},
		{
			name:      "long path with prefix match",
			deferInfo: &DeferInfo{Path: longPath},
			prefix:    []string{"query", "object1"},
			expected:  true,
		},
		{
			name:      "mismatch prefix",
			deferInfo: &DeferInfo{Path: shortPath1},
			prefix:    []string{"x", "y"},
			expected:  false,
		},
		{
			name:      "prefix has no operation type, but matches",
			deferInfo: &DeferInfo{Path: longPath},
			prefix:    []string{"object1", "field1"},
			expected:  true,
		},
		{
			name:      "prefix has no operation type, and mis-matches",
			deferInfo: &DeferInfo{Path: shortPath1},
			prefix:    []string{"x"},
			expected:  false,
		},
		{
			name:      "prefix longer than path",
			deferInfo: &DeferInfo{Path: shortPath1},
			prefix:    []string{"query", "object1", "field1"},
			expected:  false,
		},
		{
			name:      "ignore inline fragment",
			deferInfo: &DeferInfo{Path: ast.Path{pathItem1, pathItem2, fragmentItem, pathItem3}},
			prefix:    []string{"query", "object1", "field1"},
			expected:  true,
		},
		{
			name:      "ignore terminal inline fragment",
			deferInfo: &DeferInfo{Path: ast.Path{pathItem1, pathItem2, fragmentItem}},
			prefix:    []string{"query", "object1"},
			expected:  true,
		},
		{
			name:      "ignore terminal inline fragment, but mis-match",
			deferInfo: &DeferInfo{Path: ast.Path{pathItem1, pathItem2, fragmentItem}},
			prefix:    []string{"query", "x"},
			expected:  false,
		},
		{
			name:      "nil DeferInfo, non-empty prefix",
			deferInfo: nil,
			prefix:    []string{"query", "object1"},
			expected:  false,
		},
		{
			name:      "nil DeferInfo, empty prefix",
			deferInfo: nil,
			prefix:    []string{},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.deferInfo.HasPrefix(tt.prefix))
		})
	}
}
