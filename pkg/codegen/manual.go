package codegen

import "github.com/jensneuse/graphql-go-tools/pkg/ast"

type DataSource struct {
	BrokerAddr                    string
	IntField                      int64
	BoolField                     bool
	NullableBool                  *bool
	NullableListOfNullableString  *[]*string
	NonNullListOfNullableString   []*string
	NonNullListOfNonNullString    []string
	NullableListOfNullableHeader  *[]*Header
	NonNullListOfNullableHeader   []*Header
	NonNullListOfNonNullParameter []Parameter
}

func (d *DataSource) Unmarshal(doc *ast.Document, ref int) {
	for _, i := range doc.Directives[ref].Arguments.Refs {
		name := doc.ArgumentNameString(i)
		switch name {
		case "brokerAddr":
			d.BrokerAddr = doc.StringValueContentString(doc.ArgumentValue(i).Ref)
		case "intField":
			d.IntField = doc.IntValueAsInt(doc.ArgumentValue(i).Ref)
		case "boolField":
			d.BoolField = bool(doc.BooleanValue(doc.ArgumentValue(i).Ref))
		case "nullableBool":
			value := bool(doc.BooleanValue(doc.ArgumentValue(i).Ref))
			d.NullableBool = &value
		case "nullableListOfNullableString":
			var list []*string
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				value := doc.StringValueContentString(i)
				list = append(list, &value)
			}
			d.NullableListOfNullableString = &list
		case "nonNullListOfNullableString":
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				value := doc.StringValueContentString(i)
				d.NonNullListOfNullableString = append(d.NonNullListOfNullableString, &value)
			}
		case "nonNullListOfNonNullString":
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				d.NonNullListOfNonNullString = append(d.NonNullListOfNonNullString, doc.StringValueContentString(i))
			}
		case "nullableListOfNullableHeader":
			var value []*Header
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				var header Header
				header.Unmarshal(doc, i)
				value = append(value, &header)
			}
			d.NullableListOfNullableHeader = &value
		case "nonNullListOfNullableHeader":
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				var header Header
				header.Unmarshal(doc, i)
				d.NonNullListOfNullableHeader = append(d.NonNullListOfNullableHeader, &header)
			}
		case "nonNullListOfNonNullParameter":
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				var parameter Parameter
				parameter.Unmarshal(doc, i)
				d.NonNullListOfNonNullParameter = append(d.NonNullListOfNonNullParameter, parameter)
			}
		}
	}
}

type Header struct {
	Key   string
	Value string
}

func (h *Header) Unmarshal(doc *ast.Document, ref int) {
	for _, i := range doc.ObjectValues[ref].Refs {
		name := string(doc.ObjectFieldNameBytes(i))
		switch name {
		case "key":
			h.Key = doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
		case "value":
			h.Value = doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
		}
	}
}

type Parameter struct {
	Name         string
	SourceKind   PARAMETER_SOURCE
	SourceName   string
	VariableType string
}

func (p *Parameter) Unmarshal(doc *ast.Document,ref int){
	for _, i := range doc.ObjectValues[ref].Refs {
		name := string(doc.ObjectFieldNameBytes(i))
		switch name {
		case "name":
			p.Name = doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
		case "sourceKind":
			p.SourceKind.Unmarshal(doc,i)
		case "sourceName":
			p.SourceName = doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
		case "variableType":
			p.VariableType = doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
		}
	}
}

type HTTP_METHOD int

const (
	UNDEFINED_HTTP_METHOD HTTP_METHOD = iota
	HTTP_METHOD_GET
	HTTP_METHOD_POST
	HTTP_METHOD_UPDATE
	HTTP_METHOD_DELETE
)

type PARAMETER_SOURCE int

func (p *PARAMETER_SOURCE) Unmarshal(doc *ast.Document,ref int){

}

const (
	UNDEFINED_PARAMETER_SOURCE PARAMETER_SOURCE = iota
	PARAMETER_SOURCE_CONTEXT_VARIABLE
	PARAMETER_SOURCE_OBJECT_VARIABLE_ARGUMENT
	PARAMETER_SOURCE_FIELD_ARGUMENTS
)
