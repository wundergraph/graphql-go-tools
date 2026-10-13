package ast

import "bytes"

func namedValueSetsAreEqual(d *Document, left, right []int, name func(int) ByteSlice, value func(int) Value) bool {
	if len(left) != len(right) {
		return false
	}
	for i, leftRef := range left {
		rightRef := right[i]
		if !bytes.Equal(name(leftRef), name(rightRef)) {
			rightRef = InvalidRef
			for _, candidate := range right {
				if bytes.Equal(name(leftRef), name(candidate)) {
					rightRef = candidate
					break
				}
			}
			if rightRef == InvalidRef {
				return false
			}
		}
		if !d.argumentValuesAreEqual(value(leftRef), value(rightRef)) {
			return false
		}
	}
	return true
}

func (d *Document) argumentValuesAreEqual(left, right Value) bool {
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
			if !d.argumentValuesAreEqual(d.Value(leftValues[i]), d.Value(rightValues[i])) {
				return false
			}
		}
		return true
	default:
		return d.ValuesAreEqual(left, right)
	}
}
