package ast

import "github.com/wundergraph/graphql-go-tools/pkg/lexer/position"

// ObjectValue
// example:
// { lon: 12.43, lat: -53.211 }
type ObjectValue struct {
	LBRACE position.Position
	Refs   []int // ObjectField
	RBRACE position.Position
}

func (d *Document) CopyObjectValue(ref int) int {
	refs := d.NewEmptyRefs()
	for _, r := range d.ObjectValues[ref].Refs {
		refs = append(refs, d.CopyObjectField(r))
	}
	return d.AddObjectValue(ObjectValue{
		Refs: refs,
	})
}

func (d *Document) AddObjectValue(value ObjectValue) (ref int) {
	d.ObjectValues = append(d.ObjectValues, value)
	return len(d.ObjectValues) - 1
}

func (d *Document) ImportObjectValue(fieldRefs []int) (ref int) {
	return d.AddObjectValue(ObjectValue{
		Refs: fieldRefs,
	})
}
