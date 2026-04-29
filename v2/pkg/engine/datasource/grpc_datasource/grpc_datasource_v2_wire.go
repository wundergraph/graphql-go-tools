package grpcdatasource

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/encoding/protowire"
)

type v2PreMarshaledInput struct {
	wire []byte
}

type v2WirePlan struct {
	fields []v2WireField
}

type v2WireField struct {
	tag         []byte
	number      protowire.Number
	dataType    DataType
	jsonPath    string
	staticValue string
	optional    bool
	repeated    bool
	child       *v2WirePlan
}

func compileV2WirePlan(program *v2RequestProgram) (*v2WirePlan, bool) {
	if program == nil || program.context != nil {
		return nil, false
	}

	plan := &v2WirePlan{
		fields: make([]v2WireField, 0, len(program.fields)),
	}

	for i := range program.fields {
		field := &program.fields[i]
		if field.enumName != "" {
			return nil, false
		}

		wireField := v2WireField{
			number:      field.runtime.desc.Number(),
			dataType:    field.runtime.dataType,
			jsonPath:    field.jsonPath,
			staticValue: field.staticValue,
			optional:    field.optional,
			repeated:    field.repeated,
		}

		if field.child != nil {
			child, ok := compileV2WirePlan(field.child)
			if !ok {
				return nil, false
			}
			wireField.child = child
		}

		wireType := v2WireType(field.runtime.dataType, field.child != nil)
		wireField.tag = protowire.AppendTag(nil, wireField.number, wireType)
		plan.fields = append(plan.fields, wireField)
	}

	return plan, true
}

func (p *v2WirePlan) execute(buf []byte, data gjson.Result) ([]byte, error) {
	for i := range p.fields {
		var err error
		buf, err = p.fields[i].appendWire(buf, data)
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

func (f *v2WireField) appendWire(buf []byte, data gjson.Result) ([]byte, error) {
	fieldData := data
	if f.staticValue != "" {
		fieldData = gjson.Parse(f.staticValue)
	} else if f.jsonPath != "" {
		fieldData = data.Get(f.jsonPath)
	}

	if isNullValue(fieldData) {
		if f.optional {
			return buf, nil
		}
		return nil, fmt.Errorf("field %s is required but has no value", f.jsonPath)
	}

	if f.repeated {
		for _, element := range fieldData.Array() {
			var err error
			buf, err = f.appendSingle(buf, element)
			if err != nil {
				return nil, err
			}
		}
		return buf, nil
	}

	return f.appendSingle(buf, fieldData)
}

func (f *v2WireField) appendSingle(buf []byte, data gjson.Result) ([]byte, error) {
	if f.child != nil {
		body, err := f.child.execute(nil, data)
		if err != nil {
			return nil, err
		}
		buf = append(buf, f.tag...)
		buf = protowire.AppendVarint(buf, uint64(len(body)))
		buf = append(buf, body...)
		return buf, nil
	}

	switch f.dataType {
	case DataTypeString, DataTypeBytes:
		value := data.String()
		buf = append(buf, f.tag...)
		buf = protowire.AppendVarint(buf, uint64(len(value)))
		buf = append(buf, value...)
	case DataTypeBool:
		buf = append(buf, f.tag...)
		if data.Bool() {
			buf = protowire.AppendVarint(buf, 1)
		} else {
			buf = protowire.AppendVarint(buf, 0)
		}
	case DataTypeInt32, DataTypeInt64, DataTypeUint32, DataTypeUint64, DataTypeEnum:
		buf = append(buf, f.tag...)
		buf = protowire.AppendVarint(buf, uint64(data.Int()))
	case DataTypeDouble:
		buf = append(buf, f.tag...)
		var bits [8]byte
		binary.LittleEndian.PutUint64(bits[:], math.Float64bits(data.Float()))
		buf = append(buf, bits[:]...)
	case DataTypeFloat:
		buf = append(buf, f.tag...)
		var bits [4]byte
		binary.LittleEndian.PutUint32(bits[:], math.Float32bits(float32(data.Float())))
		buf = append(buf, bits[:]...)
	default:
		return nil, fmt.Errorf("unsupported wire data type %s", f.dataType)
	}

	return buf, nil
}

func v2WireType(dataType DataType, isMessage bool) protowire.Type {
	if isMessage {
		return protowire.BytesType
	}

	switch dataType {
	case DataTypeString, DataTypeBytes:
		return protowire.BytesType
	case DataTypeDouble:
		return protowire.Fixed64Type
	case DataTypeFloat:
		return protowire.Fixed32Type
	default:
		return protowire.VarintType
	}
}
