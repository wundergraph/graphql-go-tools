package execution

import (
	"bytes"
	"testing"
)

func TestJSONValueType_writeValue(t *testing.T) {
	run := func(valueType JSONValueType, value []byte, expectErr bool, expectString string) func(t *testing.T) {
		return func(t *testing.T) {
			buf := bytes.Buffer{}
			_, err := valueType.writeValue(value, nil, &buf)
			if expectErr && err == nil {
				t.Fatal("expected err, got nil")
			}
			if !expectErr && err != nil {
				t.Fatalf("expected nil error, got: %s",err)
			}
			if expectString != buf.String() {
				t.Fatalf("expected out: %s, got: %s",expectString,buf.String())
			}
		}
	}

	t.Run("valid string",run(StringValueType,[]byte("foo"),false,`"foo"`))

	t.Run("valid integer",run(IntegerValueType,[]byte("123"),false,`123`))
	t.Run("invalid integer",run(IntegerValueType,[]byte("1.23"),true,""))
	t.Run("invalid integer",run(IntegerValueType,[]byte("true"),true,""))
	t.Run("invalid integer",run(IntegerValueType,[]byte("false"),true,""))
	t.Run("invalid integer",run(IntegerValueType,[]byte(`"123"`),true,""))

	t.Run("valid bool",run(BooleanValueType,[]byte("true"),false,"true"))
	t.Run("valid bool",run(BooleanValueType,[]byte("false"),false,"false"))
	t.Run("invalid bool",run(BooleanValueType,[]byte("1.23"),true,""))
	t.Run("invalid bool",run(BooleanValueType,[]byte("123"),true,""))
	t.Run("invalid bool",run(BooleanValueType,[]byte(`"true"`),true,""))

	t.Run("valid float",run(FloatValueType,[]byte("1.23"),false,"1.23"))
	t.Run("valid float",run(FloatValueType,[]byte("123"),false,"123"))
	t.Run("invalid float",run(FloatValueType,[]byte("1.2.3"),true,""))
	t.Run("invalid float",run(FloatValueType,[]byte("true"),true,""))
	t.Run("invalid float",run(FloatValueType,[]byte(`"123"`),true,""))
}
