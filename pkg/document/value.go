package document

// Value as specified in http://facebook.github.io/graphql/draft/#Value
type Value interface {
	isValue()
	ValueType() ValueType
}

// VariableValue as specified in:
// http://facebook.github.io/graphql/draft/#Variable
type VariableValue struct {
	Name []byte
}

func (VariableValue) isValue() {}

// ValueType returns VariableValueType
func (VariableValue) ValueType() ValueType {
	return ValueTypeVariable
}

// IntValue as specified in:
// http://facebook.github.io/graphql/draft/#IntValue
type IntValue struct {
	Val int32
}

func (IntValue) isValue() {}

// ValueType returns IntValueType
func (IntValue) ValueType() ValueType {
	return ValueTypeInt
}

// FloatValue as specified in:
// http://facebook.github.io/graphql/draft/#FloatValue
type FloatValue struct {
	Val float32
}

func (FloatValue) isValue() {
}

// ValueType returns FloatValueType
func (FloatValue) ValueType() ValueType {
	return ValueTypeFloat
}

// StringValue as specified in:
// http://facebook.github.io/graphql/draft/#StringValue
type StringValue struct {
	Val ByteSlice
}

func (StringValue) isValue() {}

// ValueType returns StringValueType
func (StringValue) ValueType() ValueType {
	return ValueTypeString
}

// BooleanValue as specified in:
// http://facebook.github.io/graphql/draft/#BooleanValue
type BooleanValue struct {
	Val bool
}

func (BooleanValue) isValue() {}

// ValueType returns BooleanValueType
func (BooleanValue) ValueType() ValueType {
	return ValueTypeBoolean
}

// NullValue as specified in:
// http://facebook.github.io/graphql/draft/#NullValue
type NullValue struct{}

func (NullValue) isValue() {}

// ValueType returns NullValueType
func (NullValue) ValueType() ValueType {
	return ValueTypeNull
}

// EnumValue as specified in:
// http://facebook.github.io/graphql/draft/#EnumValue
type EnumValue struct {
	Name ByteSlice // but not true or false or null
}

func (EnumValue) isValue() {}

// ValueType returns EnumValueType
func (EnumValue) ValueType() ValueType {
	return ValueTypeEnum
}

// ListValue as specified in:
// http://facebook.github.io/graphql/draft/#ListValue
type ListValue struct {
	ValuesType ValueType
	Values     []Value
}

func (ListValue) isValue() {}

// ValueType returns ListValueType
func (ListValue) ValueType() ValueType {
	return ValueTypeList
}

// ObjectValue as specified in:
// http://facebook.github.io/graphql/draft/#ObjectValue
type ObjectValue struct {
	Val []ObjectField
}

func (ObjectValue) isValue() {}

// ValueType returns ObjectValueType
func (ObjectValue) ValueType() ValueType {
	return ValueTypeObject
}
