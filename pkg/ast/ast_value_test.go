package ast

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDocument_ValueToJSON(t *testing.T) {
	run := func(prepareDoc func(doc *Document) Value, expectedOutput string) func(t *testing.T) {
		operation := NewDocument()
		return func(t *testing.T) {
			out, err := operation.ValueToJSON(prepareDoc(operation))
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, string(out))
		}
	}

	t.Run("ValueKindNull", run(func(doc *Document) Value {
		return Value{
			Kind: ValueKindNull,
			Ref:  0,
		}
	}, `null`))
	t.Run("ValueKindEnum", run(func(doc *Document) Value {
		doc.EnumValues = append(doc.EnumValues, EnumValue{
			Name: doc.Input.AppendInputString("FOO"),
		})
		return Value{
			Kind: ValueKindEnum,
			Ref:  0,
		}
	}, `"FOO"`))
	t.Run("ValueKindInteger - positive", run(func(doc *Document) Value {
		doc.IntValues = append(doc.IntValues, IntValue{
			Raw: doc.Input.AppendInputString("123"),
		})
		return Value{
			Kind: ValueKindInteger,
			Ref:  0,
		}
	}, `123`))
	t.Run("ValueKindInteger - negative", run(func(doc *Document) Value {
		doc.IntValues = append(doc.IntValues, IntValue{
			Raw:      doc.Input.AppendInputString("123"),
			Negative: true,
		})
		return Value{
			Kind: ValueKindInteger,
			Ref:  0,
		}
	}, `-123`))
	t.Run("ValueKindFloat - positive", run(func(doc *Document) Value {
		doc.FloatValues = append(doc.FloatValues, FloatValue{
			Raw: doc.Input.AppendInputString("12.34"),
		})
		return Value{
			Kind: ValueKindFloat,
			Ref:  0,
		}
	}, `12.34`))
	t.Run("ValueKindFloat - negative", run(func(doc *Document) Value {
		doc.FloatValues = append(doc.FloatValues, FloatValue{
			Raw:      doc.Input.AppendInputString("12.34"),
			Negative: true,
		})
		return Value{
			Kind: ValueKindFloat,
			Ref:  0,
		}
	}, `-12.34`))
	t.Run("ValueKindBoolean - false", run(func(doc *Document) Value {
		return Value{
			Kind: ValueKindBoolean,
			Ref:  0,
		}
	}, `false`))
	t.Run("ValueKindBoolean - true", run(func(doc *Document) Value {
		return Value{
			Kind: ValueKindBoolean,
			Ref:  1,
		}
	}, `true`))
	t.Run("ValueKindString", run(func(doc *Document) Value {
		doc.StringValues = append(doc.StringValues, StringValue{
			Content: doc.Input.AppendInputString("foo"),
		})
		return Value{
			Kind: ValueKindString,
			Ref:  0,
		}
	}, `"foo"`))
	t.Run("ValueKindList", run(func(doc *Document) Value {
		doc.StringValues = append(doc.StringValues, StringValue{
			Content: doc.Input.AppendInputString("foo"),
		})
		doc.StringValues = append(doc.StringValues, StringValue{
			Content: doc.Input.AppendInputString("bar"),
		})
		doc.StringValues = append(doc.StringValues, StringValue{
			Content: doc.Input.AppendInputString("baz"),
		})
		for i := 0; i < 3; i++ {
			doc.Values = append(doc.Values, Value{Kind: ValueKindString, Ref: i})
		}
		doc.IntValues = append(doc.IntValues, IntValue{
			Raw: doc.Input.AppendInputString("123"),
		})
		doc.Values = append(doc.Values, Value{Kind: ValueKindInteger, Ref: 0})
		doc.ListValues = append(doc.ListValues, ListValue{
			Refs: []int{0, 1, 2, 3},
		})
		return Value{
			Kind: ValueKindList,
			Ref:  0,
		}
	}, `["foo","bar","baz",123]`))
	t.Run("ValueKindObject", run(func(doc *Document) Value {
		doc.StringValues = append(doc.StringValues, StringValue{
			Content: doc.Input.AppendInputString("bar"),
		})
		doc.StringValues = append(doc.StringValues, StringValue{
			Content: doc.Input.AppendInputString("bal"),
		})
		doc.ObjectFields = append(doc.ObjectFields, ObjectField{
			Name: doc.Input.AppendInputString("bat"),
			Value: Value{
				Kind: ValueKindString,
				Ref:  1,
			},
		})
		doc.ObjectFields = append(doc.ObjectFields,
			ObjectField{
				Name: doc.Input.AppendInputString("foo"),
				Value: Value{
					Kind: ValueKindString,
					Ref:  0,
				},
			},
			ObjectField{
				Name: doc.Input.AppendInputString("baz"),
				Value: Value{
					Kind: ValueKindObject,
					Ref:  1,
				},
			})
		for i := 0; i < 3; i++ {
			doc.IntValues = append(doc.IntValues, IntValue{
				Raw: doc.Input.AppendInputString(strconv.Itoa(i + 1)),
			})
			doc.Values = append(doc.Values, Value{Kind: ValueKindInteger, Ref: i})
		}
		doc.ListValues = append(doc.ListValues, ListValue{
			Refs: []int{0, 1, 2},
		})
		doc.ObjectFields = append(doc.ObjectFields, ObjectField{
			Name: doc.Input.AppendInputString("list"),
			Value: Value{
				Kind: ValueKindList,
				Ref:  0,
			},
		})
		doc.ObjectValues = append(doc.ObjectValues, ObjectValue{
			Refs: []int{1, 2, 3},
		})
		doc.ObjectValues = append(doc.ObjectValues, ObjectValue{
			Refs: []int{0},
		})
		return Value{
			Kind: ValueKindObject,
			Ref:  0,
		}
	}, `{"foo":"bar","baz":{"bat":"bal"},"list":[1,2,3]}`))
}

func TestDocument_PrintValue(t *testing.T) {
	run := func(prepareDoc func(doc *Document) Value, expectedOutput string) func(t *testing.T) {
		operation := NewDocument()
		return func(t *testing.T) {
			buf := new(bytes.Buffer)
			err := operation.PrintValue(prepareDoc(operation), buf)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, buf.String())
		}
	}
	t.Run("ValueKindString - non-block", run(func(doc *Document) Value {
		doc.StringValues = append(doc.StringValues, StringValue{
			Content: doc.Input.AppendInputString("foo"),
		})
		return Value{
			Kind: ValueKindString,
			Ref:  0,
		}
	}, `"foo"`))
	t.Run("ValueKindString - block", run(func(doc *Document) Value {
		doc.StringValues = append(doc.StringValues, StringValue{
			BlockString: true,
			Content:     doc.Input.AppendInputString("foo"),
		})
		return Value{
			Kind: ValueKindString,
			Ref:  0,
		}
	}, `"""foo"""`))
}
