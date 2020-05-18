package ast

import "github.com/jensneuse/graphql-go-tools/pkg/lexer/position"

// ObjectValue
// example:
// { lon: 12.43, lat: -53.211 }
type ObjectValue struct {
	LBRACE position.Position
	Refs   []int // ObjectField
	RBRACE position.Position
}

func (d *Document) AddObjectValue(fieldRefs []int) (ref int) {
	d.ObjectValues = append(d.ObjectValues, ObjectValue{
		Refs: fieldRefs,
	})
	return len(d.ObjectValues) - 1
}
