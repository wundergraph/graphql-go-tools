package resolve

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/buger/jsonparser"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastbuffer"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type SimpleResolver struct {
}

func NewSimpleResolver() *SimpleResolver {
	return &SimpleResolver{}
}

func (r *SimpleResolver) resolveNode(node Node, data []byte, buf *fastbuffer.FastBuffer) (err error) {
	switch n := node.(type) {
	case *Object:
		return r.resolveObject(n, data, buf)
	case *Array:
		return r.resolveArray(n, data, buf)
	case *Null:
		r.resolveNull(buf)
		return
	case *String:
		return r.resolveString(n, data, buf)
	case *Boolean:
		return r.resolveBoolean(n, data, buf)
	case *Integer:
		return r.resolveInteger(n, data, buf)
	case *Float:
		return r.resolveFloat(n, data, buf)
	case *EmptyObject:
		r.resolveEmptyObject(buf)
		return
	case *EmptyArray:
		r.resolveEmptyArray(buf)
		return
	default:
		return
	}
}

func (r *SimpleResolver) resolveObject(object *Object, data []byte, resolveBuf *fastbuffer.FastBuffer) (err error) {
	if len(object.Path) != 0 {
		data, _, _, _ = jsonparser.Get(data, object.Path...)

		if len(data) == 0 || bytes.Equal(data, literal.NULL) {
			if object.Nullable {
				// write empty object to resolve buffer
				r.resolveNull(resolveBuf)
				return
			}

			return errNonNullableFieldValueIsNull
		}
	}

	fieldBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(fieldBuf)

	objectBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(objectBuf)

	first := true
	for i := range object.Fields {
		fieldData := data
		if first {
			objectBuf.WriteBytes(lBrace)
			first = false
		} else {
			objectBuf.WriteBytes(comma)
		}
		objectBuf.WriteBytes(quote)
		objectBuf.WriteBytes(object.Fields[i].Name)
		objectBuf.WriteBytes(quote)
		objectBuf.WriteBytes(colon)
		err = r.resolveNode(object.Fields[i].Value, fieldData, fieldBuf)
		if err != nil {
			if errors.Is(err, errNonNullableFieldValueIsNull) {
				objectBuf.Reset()

				if object.Nullable {
					// write empty object to resolve buffer
					r.resolveNull(resolveBuf)
					return nil
				}
			}
			return err
		}
		objectBuf.WriteBytes(fieldBuf.Bytes())
		fieldBuf.Reset()
	}

	if first {
		if !object.Nullable {
			return errNonNullableFieldValueIsNull
		}
		// write empty object to resolve buffer
		r.resolveNull(resolveBuf)
		return
	}
	objectBuf.WriteBytes(rBrace)

	// write full object to resolve buffer
	resolveBuf.WriteBytes(objectBuf.Bytes())

	return
}

func (r *SimpleResolver) resolveArray(array *Array, data []byte, arrayBuf *fastbuffer.FastBuffer) (err error) {
	if len(array.Path) != 0 {
		data, _, _, _ = jsonparser.Get(data, array.Path...)
	}

	if bytes.Equal(data, emptyArray) {
		r.resolveEmptyArray(arrayBuf)
		return
	}

	var arrayItems [][]byte

	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if err == nil && dataType == jsonparser.String {
			value = data[offset-2 : offset+len(value)] // add quotes to string values
		}

		arrayItems = append(arrayItems, value)
	})

	if len(arrayItems) == 0 {
		if !array.Nullable {
			r.resolveEmptyArray(arrayBuf)
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(arrayBuf)
		return nil
	}

	return r.resolveArraySynchronous(array, &arrayItems, arrayBuf)
}

func (r *SimpleResolver) resolveArraySynchronous(array *Array, arrayItems *[][]byte, resolveBuf *fastbuffer.FastBuffer) (err error) {
	itemBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(itemBuf)

	arrayBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(arrayBuf)

	arrayBuf.WriteBytes(lBrack)

	var (
		hasPreviousItem bool
		dataWritten     int
	)

	for i := range *arrayItems {
		err = r.resolveNode(array.Item, (*arrayItems)[i], itemBuf)
		if err != nil {
			if errors.Is(err, errNonNullableFieldValueIsNull) {
				if !array.Nullable {
					return err
				}
				r.resolveNull(resolveBuf)
				return nil
			}
			return err
		}
		dataWritten += itemBuf.Len()
		arrayBuf.WriteBytes(itemBuf.Bytes())
		if !hasPreviousItem && dataWritten != 0 {
			hasPreviousItem = true
		}
		itemBuf.Reset()
	}

	arrayBuf.WriteBytes(rBrack)
	resolveBuf.WriteBytes(arrayBuf.Bytes())
	return
}

func (r *SimpleResolver) resolveNull(b *fastbuffer.FastBuffer) {
	b.WriteBytes(null)
}

func (r *SimpleResolver) resolveInteger(integer *Integer, data []byte, integerBuf *fastbuffer.FastBuffer) error {
	value, dataType, _, err := jsonparser.Get(data, integer.Path...)
	if err != nil || dataType != jsonparser.Number {
		if !integer.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(integerBuf)
		return nil
	}
	integerBuf.WriteBytes(value)
	return nil
}

func (r *SimpleResolver) resolveFloat(floatValue *Float, data []byte, floatBuf *fastbuffer.FastBuffer) error {
	value, dataType, _, err := jsonparser.Get(data, floatValue.Path...)
	if err != nil || dataType != jsonparser.Number {
		if !floatValue.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(floatBuf)
		return nil
	}
	floatBuf.WriteBytes(value)
	return nil
}

func (r *SimpleResolver) resolveBoolean(boolean *Boolean, data []byte, booleanBuf *fastbuffer.FastBuffer) error {
	value, valueType, _, err := jsonparser.Get(data, boolean.Path...)
	if err != nil || valueType != jsonparser.Boolean {
		if !boolean.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(booleanBuf)
		return nil
	}
	booleanBuf.WriteBytes(value)
	return nil
}

func (r *SimpleResolver) resolveString(str *String, data []byte, stringBuf *fastbuffer.FastBuffer) error {
	var (
		value     []byte
		valueType jsonparser.ValueType
		err       error
	)

	value, valueType, _, err = jsonparser.Get(data, str.Path...)
	if err != nil || valueType != jsonparser.String {
		if value != nil && valueType != jsonparser.Null {
			return fmt.Errorf("invalid value type '%s' for path %s, expecting string, got: %v", valueType, str.Path, string(value))
		}
		if !str.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(stringBuf)
		return nil
	}

	if value == nil {
		if !str.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(stringBuf)
		return nil
	}

	// value = r.renameTypeName(str, value)

	stringBuf.WriteBytes(quote)
	stringBuf.WriteBytes(value)
	stringBuf.WriteBytes(quote)
	return nil
}

func (r *SimpleResolver) resolveEmptyArray(b *fastbuffer.FastBuffer) {
	b.WriteBytes(lBrack)
	b.WriteBytes(rBrack)
}

func (r *SimpleResolver) resolveEmptyObject(b *fastbuffer.FastBuffer) {
	b.WriteBytes(lBrace)
	b.WriteBytes(rBrace)
}
