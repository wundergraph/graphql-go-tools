package ast

import "bytes"

func namedValueSetsAreEqual(d *Document, left, right []int, name func(int) ByteSlice, value func(int) Value) bool {
	if len(left) != len(right) {
		return false
	}
	for i, leftRef := range left {
		rightRef := right[i]
		if bytes.Equal(name(leftRef), name(rightRef)) {
			if !d.ArgumentValuesAreEqual(value(leftRef), value(rightRef)) {
				return false
			}
			continue
		}
		// Check both directions so repeated names cannot hide a different
		// argument or input-object field during normalization.
		if !namedValueHasMatch(d, leftRef, right, name, value) || !namedValueHasMatch(d, rightRef, left, name, value) {
			return false
		}
	}
	return true
}

func namedValueHasMatch(d *Document, ref int, refs []int, name func(int) ByteSlice, value func(int) Value) bool {
	refName := name(ref)
	for _, candidate := range refs {
		if bytes.Equal(refName, name(candidate)) {
			return d.ArgumentValuesAreEqual(value(ref), value(candidate))
		}
	}
	return false
}

// ArgumentValuesAreEqual reports whether two argument values are equal, ignoring
// input-object field order. List order and variable names remain significant.
//
// See https://spec.graphql.org/September2025/#sec-Input-Object-Values
func (d *Document) ArgumentValuesAreEqual(left, right Value) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case ValueKindObject:
		return namedValueSetsAreEqual(d, d.ObjectValues[left.Ref].Refs, d.ObjectValues[right.Ref].Refs, d.ObjectFieldNameBytes, d.ObjectFieldValue)
	case ValueKindList:
		leftValues, rightValues := d.ListValues[left.Ref].Refs, d.ListValues[right.Ref].Refs
		if len(leftValues) != len(rightValues) {
			return false
		}
		for i := range leftValues {
			if !d.ArgumentValuesAreEqual(d.Value(leftValues[i]), d.Value(rightValues[i])) {
				return false
			}
		}
		return true
	default:
		return d.ValuesAreEqual(left, right)
	}
}
