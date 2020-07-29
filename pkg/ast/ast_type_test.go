package ast

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDocument_TypeValueNeedsQuotes(t *testing.T) {
	doc := Document{}

	stringRef := doc.Input.AppendInputString("String")
	doc.Types = append(doc.Types, Type{
		TypeKind: TypeKindNamed,
		Name:     stringRef,
	})
	stringNeedsQuotes := doc.TypeValueNeedsQuotes(0)
	assert.Equal(t, true, stringNeedsQuotes)

	idRef := doc.Input.AppendInputString("ID")
	doc.Types = append(doc.Types, Type{
		TypeKind: TypeKindNamed,
		Name:     idRef,
	})
	idNeedsQuotes := doc.TypeValueNeedsQuotes(1)
	assert.Equal(t, true, idNeedsQuotes)

	intRef := doc.Input.AppendInputString("Int")
	doc.Types = append(doc.Types, Type{
		TypeKind: TypeKindNamed,
		Name:     intRef,
	})
	intNeedsQuotes := doc.TypeValueNeedsQuotes(2)
	assert.Equal(t, false, intNeedsQuotes)

	floatRef := doc.Input.AppendInputString("Float")
	doc.Types = append(doc.Types, Type{
		TypeKind: TypeKindNamed,
		Name:     floatRef,
	})
	floatNeedsQuotes := doc.TypeValueNeedsQuotes(3)
	assert.Equal(t, false, floatNeedsQuotes)

	booleanRef := doc.Input.AppendInputString("Boolean")
	doc.Types = append(doc.Types, Type{
		TypeKind: TypeKindNamed,
		Name:     booleanRef,
	})
	booleanNeedsQuotes := doc.TypeValueNeedsQuotes(4)
	assert.Equal(t, false, booleanNeedsQuotes)

	jsonRef := doc.Input.AppendInputString("JSON")
	doc.Types = append(doc.Types, Type{
		TypeKind: TypeKindNamed,
		Name:     jsonRef,
	})
	jsonNeedsQuotes := doc.TypeValueNeedsQuotes(5)
	assert.Equal(t, false, jsonNeedsQuotes)

	dateRef := doc.Input.AppendInputString("Date")
	doc.Types = append(doc.Types, Type{
		TypeKind: TypeKindNamed,
		Name:     dateRef,
	})
	dateNeedsQuotes := doc.TypeValueNeedsQuotes(6)
	assert.Equal(t, true, dateNeedsQuotes)

	customRef := doc.Input.AppendInputString("CreateUserInput")
	doc.Types = append(doc.Types, Type{
		TypeKind: TypeKindNamed,
		Name:     customRef,
	})
	customNeedsQuotes := doc.TypeValueNeedsQuotes(7)
	assert.Equal(t, false, customNeedsQuotes)
}
