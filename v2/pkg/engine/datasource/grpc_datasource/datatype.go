package grpcdatasource

import protoref "google.golang.org/protobuf/reflect/protoreflect"

// DataType represents the different types of data that can be stored in a protobuf field.
//
//go:generate stringer -type=DataType -trimprefix=DataType -output=datatype_string.go
type DataType int8

// Protobuf data types supported by the compiler.
// These values intentionally match the google.protobuf.FieldDescriptorProto.Type enum
// (exposed via protoref.Kind) for efficient conversion. The protobuf spec has been
// stable since 2008, so these values are unlikely to change.
const (
	DataTypeUnknown DataType = -1 // Represents an unknown or unsupported type
	DataTypeDouble  DataType = 1  // 64-bit floating point type (protoref.DoubleKind)
	DataTypeFloat   DataType = 2  // 32-bit floating point type (protoref.FloatKind)
	DataTypeInt64   DataType = 3  // 64-bit integer type (protoref.Int64Kind)
	DataTypeUint64  DataType = 4  // 64-bit unsigned integer type (protoref.Uint64Kind)
	DataTypeInt32   DataType = 5  // 32-bit integer type (protoref.Int32Kind)
	DataTypeBool    DataType = 8  // Boolean type (protoref.BoolKind)
	DataTypeString  DataType = 9  // String type (protoref.StringKind)
	DataTypeMessage DataType = 11 // Nested message type (protoref.MessageKind)
	DataTypeBytes   DataType = 12 // Bytes type (protoref.BytesKind)
	DataTypeUint32  DataType = 13 // 32-bit unsigned integer type (protoref.Uint32Kind)
	DataTypeEnum    DataType = 14 // Enumeration type (protoref.EnumKind)
)

// Compile-time assertions to ensure DataType values match protoref.Kind values.
// If the protoref library ever changes these values, compilation will fail here.
func _() {
	_ = [1]struct{}{}[DataTypeDouble-DataType(protoref.DoubleKind)]
	_ = [1]struct{}{}[DataTypeFloat-DataType(protoref.FloatKind)]
	_ = [1]struct{}{}[DataTypeInt64-DataType(protoref.Int64Kind)]
	_ = [1]struct{}{}[DataTypeUint64-DataType(protoref.Uint64Kind)]
	_ = [1]struct{}{}[DataTypeInt32-DataType(protoref.Int32Kind)]
	_ = [1]struct{}{}[DataTypeBool-DataType(protoref.BoolKind)]
	_ = [1]struct{}{}[DataTypeString-DataType(protoref.StringKind)]
	_ = [1]struct{}{}[DataTypeMessage-DataType(protoref.MessageKind)]
	_ = [1]struct{}{}[DataTypeBytes-DataType(protoref.BytesKind)]
	_ = [1]struct{}{}[DataTypeUint32-DataType(protoref.Uint32Kind)]
	_ = [1]struct{}{}[DataTypeEnum-DataType(protoref.EnumKind)]
}

// dataTypeMap maps protoref.Kind to DataType constants.
var dataTypeMap = map[protoref.Kind]DataType{
	protoref.StringKind:  DataTypeString,
	protoref.Int32Kind:   DataTypeInt32,
	protoref.Int64Kind:   DataTypeInt64,
	protoref.Uint32Kind:  DataTypeUint32,
	protoref.Uint64Kind:  DataTypeUint64,
	protoref.FloatKind:   DataTypeFloat,
	protoref.DoubleKind:  DataTypeDouble,
	protoref.BoolKind:    DataTypeBool,
	protoref.BytesKind:   DataTypeBytes,
	protoref.EnumKind:    DataTypeEnum,
	protoref.MessageKind: DataTypeMessage,
}
