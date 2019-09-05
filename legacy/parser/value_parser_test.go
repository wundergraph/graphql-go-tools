package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseValue(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		run("1337", mustParseValue(
			document.ValueTypeInt,
			expectIntegerValue(1337),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   4},
			),
		))
	})
	t.Run("string", func(t *testing.T) {
		run(`"foo"`, mustParseValue(
			document.ValueTypeString,
			expectByteSliceValue("foo"),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("list", func(t *testing.T) {
		run("[1,3,3,7]", mustParseValue(
			document.ValueTypeList,
			expectListValue(
				expectIntegerValue(1),
				expectIntegerValue(3),
				expectIntegerValue(3),
				expectIntegerValue(7),
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   10,
			}),
		))
	})
	t.Run("mixed list", func(t *testing.T) {
		run(`[ 1	,"2" 3,,[	1	], { foo: 1337 } ]`,
			mustParseValue(
				document.ValueTypeList,
				expectListValue(
					expectIntegerValue(1),
					expectByteSliceValue("2"),
					expectIntegerValue(3),
					expectListValue(
						expectIntegerValue(1),
					),
					expectObjectValue(
						node(
							hasName("foo"),
							expectIntegerValue(1337),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					LineEnd:   1,
					CharStart: 1,
					CharEnd:   35,
				}),
			))
	})
	t.Run("object", func(t *testing.T) {
		run(`{foo: "bar"}`,
			mustParseValue(document.ValueTypeObject,
				expectObjectValue(
					node(
						hasName("foo"),
						expectByteSliceValue("bar"),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					LineEnd:   1,
					CharStart: 1,
					CharEnd:   13,
				}),
			))
	})
	t.Run("invalid object", func(t *testing.T) {
		run(`{foo. "bar"}`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid object 2", func(t *testing.T) {
		run(`{foo: [String!}`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid object 3", func(t *testing.T) {
		run(`{foo: "bar" )`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("nested object", func(t *testing.T) {
		run(`{foo: {bar: "baz"}, someEnum: SOME_ENUM }`, mustParseValue(document.ValueTypeObject,
			expectObjectValue(
				node(
					hasName("foo"),
					expectObjectValue(
						node(
							hasName("bar"),
							expectByteSliceValue("baz"),
						),
					),
				),
				node(
					hasName("someEnum"),
					expectByteSliceValue("SOME_ENUM"),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   42,
			}),
		))
	})
	t.Run("variable", func(t *testing.T) {
		run("$1337", mustParseValue(
			document.ValueTypeVariable,
			expectByteSliceValue("1337"),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("variable 2", func(t *testing.T) {
		run("$foo", mustParseValue(document.ValueTypeVariable,
			expectByteSliceValue("foo"),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 1,
				End:   4},
			),
		))
	})
	t.Run("variable 3", func(t *testing.T) {
		run("$_foo", mustParseValue(document.ValueTypeVariable, expectByteSliceValue("_foo")))
	})
	t.Run("invalid variable", func(t *testing.T) {
		run("$ foo", mustPanic(mustParseValue(document.ValueTypeVariable, expectByteSliceValue(" foo"))))
	})
	t.Run("float", func(t *testing.T) {
		run("13.37", mustParseValue(
			document.ValueTypeFloat,
			expectFloatValue(13.37),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   5},
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("invalid float", func(t *testing.T) {
		run("1.3.3.7", mustPanic(mustParseValue(document.ValueTypeFloat, expectFloatValue(13.37))))
	})
	t.Run("boolean", func(t *testing.T) {
		run("true", mustParseValue(
			document.ValueTypeBoolean,
			expectBooleanValue(true),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   4},
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
		))
	})
	t.Run("boolean 2", func(t *testing.T) {
		run("false", mustParseValue(document.ValueTypeBoolean,
			expectBooleanValue(false),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   5},
			),
		))
	})
	t.Run("string", func(t *testing.T) {
		run(`"foo"`,
			mustParseValue(document.ValueTypeString,
				expectByteSliceValue("foo"),
				expectByteSliceRef(document.ByteSliceReference{
					Start: 1,
					End:   4},
				),
			))
	})
	t.Run("string 2", func(t *testing.T) {
		run(`"""foo"""`, mustParseValue(document.ValueTypeString, expectByteSliceValue("foo")))
	})
	t.Run("null", func(t *testing.T) {
		run("null", mustParseValue(
			document.ValueTypeNull,
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   0},
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
		))
	})
	t.Run("valid float", func(t *testing.T) {
		run("", mustParseFloatValue(t, "13.37", 13.37))
	})
	t.Run("invalid float", func(t *testing.T) {
		run("1.3.3.7", mustPanic(mustParseFloatValue(t, "1.3.3.7", 13.37)))
	})
}
