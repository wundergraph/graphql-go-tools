package ast

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamedValueSetsAreEqual(t *testing.T) {
	t.Parallel()

	doc := NewDocument()
	field := func(name string, value Value) int {
		return doc.ImportObjectField([]byte(name), value)
	}
	object := func(fields ...int) Value {
		return Value{Kind: ValueKindObject, Ref: doc.ImportObjectValue(fields)}
	}
	list := func(values ...Value) Value {
		refs := make([]int, len(values))
		for i, value := range values {
			refs[i] = doc.AddValue(value)
		}
		return Value{Kind: ValueKindList, Ref: doc.ImportListValue(refs)}
	}
	input := func(value Value) []int {
		return []int{field("input", value)}
	}
	variable := func(name string) Value {
		return Value{Kind: ValueKindVariable, Ref: doc.ImportVariableValue([]byte(name))}
	}
	falseValue, trueValue := Value{Kind: ValueKindBoolean}, Value{Kind: ValueKindBoolean, Ref: 1}
	a, b, c := field("a", falseValue), field("b", trueValue), field("c", variable("id"))

	cases := []struct {
		name        string
		left, right []int
		want        bool
	}{
		{"Empty", nil, nil, true},
		{"MatchingOrder", []int{c, a}, []int{field("c", variable("id")), field("a", falseValue)}, true},
		{"Reordered", []int{a, b, c}, []int{a, c, b}, true},
		{"ConflictingValues", []int{a, b}, []int{b, field("a", trueValue)}, false},
		{"MissingValue", []int{a}, nil, false},
		{"DuplicateNamesMissingValue", []int{a, a}, []int{a, b}, false},
		{"DuplicateNamesConflictingValues", []int{a, a, b}, []int{b, a, field("a", trueValue)}, false},
		{"DuplicateObjectFields", input(object(a, a)), input(object(a, b)), false},
		{"DifferentNames", []int{a}, []int{b}, false},
		{"MatchingObjectOrder", input(object(b, a)), input(object(b, a)), true},
		{"ReorderedNestedObjects", input(list(object(field("nested", object(a, b))), object(a))), input(list(object(field("nested", object(b, a))), object(a))), true},
		{"ConflictingObjectFields", input(object(a, b)), input(object(field("b", falseValue), a)), false},
		{"DifferentObjectFieldNames", input(object(a)), input(object(b)), false},
		{"MissingObjectField", input(object(a)), input(object(a, b)), false},
		{"DifferentVariables", []int{field("id", variable("a"))}, []int{field("id", variable("b"))}, false},
		{"ListOrder", input(list(falseValue, trueValue)), input(list(trueValue, falseValue)), false},
		{"ListLength", input(list(falseValue)), input(list(falseValue, trueValue)), false},
		{"ValueKinds", []int{a}, []int{field("a", Value{Kind: ValueKindNull})}, false},
	}
	for _, field := range doc.ObjectFields {
		doc.AddArgument(Argument{Name: field.Name, Value: field.Value})
	}
	accessors := []struct {
		name    string
		nameAt  func(int) ByteSlice
		valueAt func(int) Value
	}{
		{"Arguments", doc.ArgumentNameBytes, doc.ArgumentValue},
		{"ObjectFields", doc.ObjectFieldNameBytes, doc.ObjectFieldValue},
	}
	for _, accessor := range accessors {
		t.Run(accessor.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					left, right := slices.Clone(tc.left), slices.Clone(tc.right)
					assert.Equal(t, tc.want, namedValueSetsAreEqual(doc, left, right, accessor.nameAt, accessor.valueAt))
					assert.Equal(t, tc.want, namedValueSetsAreEqual(doc, right, left, accessor.nameAt, accessor.valueAt))
					assert.Equal(t, tc.left, left)
					assert.Equal(t, tc.right, right)
				})
			}
		})
	}
}
