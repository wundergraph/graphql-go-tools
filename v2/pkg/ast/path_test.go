package ast_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestPath_Overlaps(t *testing.T) {
	tests := []struct {
		name   string
		a, b   ast.Path
		expect bool
	}{
		{
			name:   "both empty",
			a:      ast.Path{},
			b:      ast.Path{},
			expect: true,
		},
		{
			name:   "same single field",
			a:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("foo")}},
			b:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("foo")}},
			expect: true,
		},
		{
			name:   "one empty",
			a:      ast.Path{},
			b:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("foo")}},
			expect: true,
		},
		{
			name:   "different single field",
			a:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("foo")}},
			b:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("bar")}},
			expect: false,
		},
		{
			name: "prefix matches but one is shorter",
			a:    ast.Path{{Kind: ast.FieldName, FieldName: []byte("foo")}},
			b: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("foo")},
				{Kind: ast.FieldName, FieldName: []byte("bar")},
			},
			expect: true,
		},
		{
			name:   "same index array overlap",
			a:      ast.Path{{Kind: ast.ArrayIndex, ArrayIndex: 0}},
			b:      ast.Path{{Kind: ast.ArrayIndex, ArrayIndex: 0}},
			expect: true,
		},
		{
			name:   "different index array overlap",
			a:      ast.Path{{Kind: ast.ArrayIndex, ArrayIndex: 1}},
			b:      ast.Path{{Kind: ast.ArrayIndex, ArrayIndex: 2}},
			expect: false,
		},
		{
			name:   "fragment mismatch",
			a:      ast.Path{{Kind: ast.InlineFragmentName, FragmentRef: 1, FieldName: []byte("FragA")}},
			b:      ast.Path{{Kind: ast.InlineFragmentName, FragmentRef: 2, FieldName: []byte("FragA")}},
			expect: false,
		},
		{
			name:   "fragment match",
			a:      ast.Path{{Kind: ast.InlineFragmentName, FragmentRef: 1, FieldName: []byte("FragA")}},
			b:      ast.Path{{Kind: ast.InlineFragmentName, FragmentRef: 1, FieldName: []byte("FragA")}},
			expect: true,
		},
		{
			name: "mixed path partial overlap",
			a: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("foo")},
				{Kind: ast.ArrayIndex, ArrayIndex: 1},
			},
			b: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("foo")},
				{Kind: ast.ArrayIndex, ArrayIndex: 1},
				{Kind: ast.FieldName, FieldName: []byte("extra")},
			},
			expect: true,
		},
		{
			name: "mixed path no overlap at second item",
			a: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("foo")},
				{Kind: ast.ArrayIndex, ArrayIndex: 2},
			},
			b: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("foo")},
				{Kind: ast.ArrayIndex, ArrayIndex: 3},
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.a.Overlaps(tt.b), tt.expect)
			assert.Equal(t, tt.b.Overlaps(tt.a), tt.expect)
		})
	}
}
