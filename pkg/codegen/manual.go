package codegen

import ast "github.com/jensneuse/graphql-go-tools/pkg/ast"

type DataSourceConfig struct {
	NonNullString                 string
	NullableString                *string
	NonNullInt                    int64
	NullableInt                   *int64
	NonNullBoolean                bool
	NullableBoolean               *bool
	NonNullFloat                  float32
	NullableFloat                 *float32
	NullableListOfNullableString  *[]*string
	NonNullListOfNullableString   []*string
	NonNullListOfNonNullString    []string
	NullableListOfNullableHeader  *[]*Header
	NonNullListOfNullableHeader   []*Header
	NonNullListOfNonNullParameter []Parameter
	Methods                       Methods
	NullableStringWithDefault     string
}

func (d *DataSourceConfig) Unmarshal(doc *ast.Document, ref int) {
	for _, i := range doc.Directives[ref].Arguments.Refs {
		name := doc.ArgumentNameString(i)
		switch name {
		case "nonNullString":
			val := doc.StringValueContentString(doc.ArgumentValue(i).Ref)
			d.NonNullString = val
		case "nullableString":
			val := doc.StringValueContentString(doc.ArgumentValue(i).Ref)
			d.NullableString = &val
		case "nonNullInt":
			val := doc.IntValueAsInt(doc.ArgumentValue(i).Ref)
			d.NonNullInt = val
		case "nullableInt":
			val := doc.IntValueAsInt(doc.ArgumentValue(i).Ref)
			d.NullableInt = &val
		case "nonNullBoolean":
			val := bool(doc.BooleanValue(doc.ArgumentValue(i).Ref))
			d.NonNullBoolean = val
		case "nullableBoolean":
			val := bool(doc.BooleanValue(doc.ArgumentValue(i).Ref))
			d.NullableBoolean = &val
		case "nonNullFloat":
			val := doc.FloatValueAsFloat32(doc.ArgumentValue(i).Ref)
			d.NonNullFloat = val
		case "nullableFloat":
			val := doc.FloatValueAsFloat32(doc.ArgumentValue(i).Ref)
			d.NullableFloat = &val
		case "nullableListOfNullableString":
			list := make([]*string, 0, len(doc.ListValues[doc.ArgumentValue(i).Ref].Refs))
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				val := doc.StringValueContentString(doc.Value(i).Ref)
				list = append(list, &val)
			}
			d.NullableListOfNullableString = &list
		case "nonNullListOfNullableString":
			list := make([]*string, 0, len(doc.ListValues[doc.ArgumentValue(i).Ref].Refs))
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				val := doc.StringValueContentString(doc.Value(i).Ref)
				list = append(list, &val)
			}
			d.NonNullListOfNullableString = list
		case "nonNullListOfNonNullString":
			list := make([]string, 0, len(doc.ListValues[doc.ArgumentValue(i).Ref].Refs))
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				val := doc.StringValueContentString(doc.Value(i).Ref)
				list = append(list, val)
			}
			d.NonNullListOfNonNullString = list
		case "nullableListOfNullableHeader":
			list := make([]*Header, 0, len(doc.ListValues[doc.ArgumentValue(i).Ref].Refs))
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				var val Header
				val.Unmarshal(doc, doc.Value(i).Ref)
				list = append(list, &val)
			}
			d.NullableListOfNullableHeader = &list
		case "nonNullListOfNullableHeader":
			list := make([]*Header, 0, len(doc.ListValues[doc.ArgumentValue(i).Ref].Refs))
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				var val Header
				val.Unmarshal(doc, doc.Value(i).Ref)
				list = append(list, &val)
			}
			d.NonNullListOfNullableHeader = list
		case "nonNullListOfNonNullParameter":
			list := make([]Parameter, 0, len(doc.ListValues[doc.ArgumentValue(i).Ref].Refs))
			for _, i := range doc.ListValues[doc.ArgumentValue(i).Ref].Refs {
				var val Parameter
				val.Unmarshal(doc, doc.Value(i).Ref)
				list = append(list, val)
			}
			d.NonNullListOfNonNullParameter = list
		case "methods":
			var val Methods
			val.Unmarshal(doc, doc.ArgumentValue(i).Ref)
			d.Methods = val
		case "nullableStringWithDefault":
			val := doc.StringValueContentString(doc.ArgumentValue(i).Ref)
			d.NullableStringWithDefault = val
		}
	}
}

type Methods struct {
	List []HTTP_METHOD
}

func (m *Methods) Unmarshal(doc *ast.Document, ref int) {
	for _, i := range doc.ObjectValues[ref].Refs {
		name := string(doc.ObjectFieldNameBytes(i))
		switch name {
		case "list":
			list := make([]HTTP_METHOD, 0, len(doc.ListValues[doc.ObjectFieldValue(i).Ref].Refs))
			for _, i := range doc.ListValues[doc.ObjectFieldValue(i).Ref].Refs {
				var val HTTP_METHOD
				val.Unmarshal(doc, doc.Value(i).Ref)
				list = append(list, val)
			}
			m.List = list
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
			val := doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
			h.Key = val
		case "value":
			val := doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
			h.Value = val
		}
	}
}

type Parameter struct {
	Name         string
	SourceKind   PARAMETER_SOURCE
	SourceName   string
	VariableName string
}

func (p *Parameter) Unmarshal(doc *ast.Document, ref int) {
	for _, i := range doc.ObjectValues[ref].Refs {
		name := string(doc.ObjectFieldNameBytes(i))
		switch name {
		case "name":
			val := doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
			p.Name = val
		case "sourceKind":
			var val PARAMETER_SOURCE
			val.Unmarshal(doc, doc.ObjectFieldValue(i).Ref)
			p.SourceKind = val
		case "sourceName":
			val := doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
			p.SourceName = val
		case "variableName":
			val := doc.StringValueContentString(doc.ObjectFieldValue(i).Ref)
			p.VariableName = val
		}
	}
}

type HTTP_METHOD int

func (h *HTTP_METHOD) Unmarshal(doc *ast.Document, ref int) {
	switch doc.EnumValueNameString(ref) {
	case "GET":
		*h = HTTP_METHOD_GET
	case "POST":
		*h = HTTP_METHOD_POST
	case "UPDATE":
		*h = HTTP_METHOD_UPDATE
	case "DELETE":
		*h = HTTP_METHOD_DELETE
	}
}

const (
	UNDEFINED_HTTP_METHOD HTTP_METHOD = iota
	HTTP_METHOD_GET
	HTTP_METHOD_POST
	HTTP_METHOD_UPDATE
	HTTP_METHOD_DELETE
)

type PARAMETER_SOURCE int

func (p *PARAMETER_SOURCE) Unmarshal(doc *ast.Document, ref int) {
	switch doc.EnumValueNameString(ref) {
	case "CONTEXT_VARIABLE":
		*p = PARAMETER_SOURCE_CONTEXT_VARIABLE
	case "OBJECT_VARIABLE_ARGUMENT":
		*p = PARAMETER_SOURCE_OBJECT_VARIABLE_ARGUMENT
	case "FIELD_ARGUMENTS":
		*p = PARAMETER_SOURCE_FIELD_ARGUMENTS
	}
}

const (
	UNDEFINED_PARAMETER_SOURCE PARAMETER_SOURCE = iota
	PARAMETER_SOURCE_CONTEXT_VARIABLE
	PARAMETER_SOURCE_OBJECT_VARIABLE_ARGUMENT
	PARAMETER_SOURCE_FIELD_ARGUMENTS
)
