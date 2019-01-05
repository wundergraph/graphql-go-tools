package document

// Value as specified in http://facebook.github.io/graphql/draft/#Value
type Value struct {
	ValueType     ValueType
	VariableValue string
	IntValue      int32
	FloatValue    float32
	StringValue   string
	BooleanValue  bool
	EnumValue     string
	ListValue     []Value
	ObjectValue   []ObjectField
}
