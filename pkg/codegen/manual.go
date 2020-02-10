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
	NonNullStringWithDefault      string
	IntWithDefault                int64
	FloatWithDefault              float32
	BooleanWithDefault            bool
	StringWithDefaultOverride     string
	InputWithDefaultChildField    InputWithDefault
}

func (d *DataSourceConfig) Unmarshal(doc *ast.Document, ref int) {
	d.NullableStringWithDefault = doc.DirectiveDefinitionArgumentDefaultValueString("DataSource", "nullableStringWithDefault")
	d.NonNullStringWithDefault = doc.DirectiveDefinitionArgumentDefaultValueString("DataSource", "nonNullStringWithDefault")
	d.IntWithDefault = doc.DirectiveDefinitionArgumentDefaultValueInt64("DataSource", "intWithDefault")
	d.FloatWithDefault = doc.DirectiveDefinitionArgumentDefaultValueFloat32("DataSource", "floatWithDefault")
	d.BooleanWithDefault = doc.DirectiveDefinitionArgumentDefaultValueBool("DataSource", "booleanWithDefault")
	d.StringWithDefaultOverride = doc.DirectiveDefinitionArgumentDefaultValueString("DataSource", "stringWithDefaultOverride")
	for _, ii := range doc.Directives[ref].Arguments.Refs {
		name := doc.ArgumentNameString(ii)
		switch name {
		case "nonNullString":
			val := doc.StringValueContentString(doc.ArgumentValue(ii).Ref)
			d.NonNullString = val
		case "nullableString":
			val := doc.StringValueContentString(doc.ArgumentValue(ii).Ref)
			d.NullableString = &val
		case "nonNullInt":
			val := doc.IntValueAsInt(doc.ArgumentValue(ii).Ref)
			d.NonNullInt = val
		case "nullableInt":
			val := doc.IntValueAsInt(doc.ArgumentValue(ii).Ref)
			d.NullableInt = &val
		case "nonNullBoolean":
			val := bool(doc.BooleanValue(doc.ArgumentValue(ii).Ref))
			d.NonNullBoolean = val
		case "nullableBoolean":
			val := bool(doc.BooleanValue(doc.ArgumentValue(ii).Ref))
			d.NullableBoolean = &val
		case "nonNullFloat":
			val := doc.FloatValueAsFloat32(doc.ArgumentValue(ii).Ref)
			d.NonNullFloat = val
		case "nullableFloat":
			val := doc.FloatValueAsFloat32(doc.ArgumentValue(ii).Ref)
			d.NullableFloat = &val
		case "nullableListOfNullableString":
			list := make([]*string, 0, len(doc.ListValues[doc.ArgumentValue(ii).Ref].Refs))
			for _, ii := range doc.ListValues[doc.ArgumentValue(ii).Ref].Refs {
				val := doc.StringValueContentString(doc.Value(ii).Ref)
				list = append(list, &val)
			}
			d.NullableListOfNullableString = &list
		case "nonNullListOfNullableString":
			list := make([]*string, 0, len(doc.ListValues[doc.ArgumentValue(ii).Ref].Refs))
			for _, ii := range doc.ListValues[doc.ArgumentValue(ii).Ref].Refs {
				val := doc.StringValueContentString(doc.Value(ii).Ref)
				list = append(list, &val)
			}
			d.NonNullListOfNullableString = list
		case "nonNullListOfNonNullString":
			list := make([]string, 0, len(doc.ListValues[doc.ArgumentValue(ii).Ref].Refs))
			for _, ii := range doc.ListValues[doc.ArgumentValue(ii).Ref].Refs {
				val := doc.StringValueContentString(doc.Value(ii).Ref)
				list = append(list, val)
			}
			d.NonNullListOfNonNullString = list
		case "nullableListOfNullableHeader":
			list := make([]*Header, 0, len(doc.ListValues[doc.ArgumentValue(ii).Ref].Refs))
			for _, ii := range doc.ListValues[doc.ArgumentValue(ii).Ref].Refs {
				var val Header
				val.Unmarshal(doc, doc.Value(ii).Ref)
				list = append(list, &val)
			}
			d.NullableListOfNullableHeader = &list
		case "nonNullListOfNullableHeader":
			list := make([]*Header, 0, len(doc.ListValues[doc.ArgumentValue(ii).Ref].Refs))
			for _, ii := range doc.ListValues[doc.ArgumentValue(ii).Ref].Refs {
				var val Header
				val.Unmarshal(doc, doc.Value(ii).Ref)
				list = append(list, &val)
			}
			d.NonNullListOfNullableHeader = list
		case "nonNullListOfNonNullParameter":
			list := make([]Parameter, 0, len(doc.ListValues[doc.ArgumentValue(ii).Ref].Refs))
			for _, ii := range doc.ListValues[doc.ArgumentValue(ii).Ref].Refs {
				var val Parameter
				val.Unmarshal(doc, doc.Value(ii).Ref)
				list = append(list, val)
			}
			d.NonNullListOfNonNullParameter = list
		case "methods":
			var val Methods
			val.Unmarshal(doc, doc.ArgumentValue(ii).Ref)
			d.Methods = val
		case "nullableStringWithDefault":
			val := doc.StringValueContentString(doc.ArgumentValue(ii).Ref)
			d.NullableStringWithDefault = val
		case "nonNullStringWithDefault":
			val := doc.StringValueContentString(doc.ArgumentValue(ii).Ref)
			d.NonNullStringWithDefault = val
		case "intWithDefault":
			val := doc.IntValueAsInt(doc.ArgumentValue(ii).Ref)
			d.IntWithDefault = val
		case "floatWithDefault":
			val := doc.FloatValueAsFloat32(doc.ArgumentValue(ii).Ref)
			d.FloatWithDefault = val
		case "booleanWithDefault":
			val := bool(doc.BooleanValue(doc.ArgumentValue(ii).Ref))
			d.BooleanWithDefault = val
		case "stringWithDefaultOverride":
			val := doc.StringValueContentString(doc.ArgumentValue(ii).Ref)
			d.StringWithDefaultOverride = val
		case "inputWithDefaultChildField":
			var val InputWithDefault
			val.Unmarshal(doc, doc.ArgumentValue(ii).Ref)
			d.InputWithDefaultChildField = val
		}
	}
}

type InputWithDefault struct {
	NullableString     *string
	StringWithDefault  string
	IntWithDefault     int64
	BooleanWithDefault bool
	FloatWithDefault   float32
}

func (i *InputWithDefault) Unmarshal(doc *ast.Document, ref int) {
	i.StringWithDefault = doc.InputObjectTypeDefinitionInputValueDefinitionDefaultValueString("InputWithDefault", "stringWithDefault")
	i.IntWithDefault = doc.InputObjectTypeDefinitionInputValueDefinitionDefaultValueInt64("InputWithDefault", "intWithDefault")
	i.BooleanWithDefault = doc.InputObjectTypeDefinitionInputValueDefinitionDefaultValueBool("InputWithDefault", "booleanWithDefault")
	i.FloatWithDefault = doc.InputObjectTypeDefinitionInputValueDefinitionDefaultValueFloat32("InputWithDefault", "floatWithDefault")
	for _, ii := range doc.ObjectValues[ref].Refs {
		name := string(doc.ObjectFieldNameBytes(ii))
		switch name {
		case "nullableString":
			val := doc.StringValueContentString(doc.ObjectFieldValue(ii).Ref)
			i.NullableString = &val
		case "stringWithDefault":
			val := doc.StringValueContentString(doc.ObjectFieldValue(ii).Ref)
			i.StringWithDefault = val
		case "intWithDefault":
			val := doc.IntValueAsInt(doc.ObjectFieldValue(ii).Ref)
			i.IntWithDefault = val
		case "booleanWithDefault":
			val := bool(doc.BooleanValue(doc.ObjectFieldValue(ii).Ref))
			i.BooleanWithDefault = val
		case "floatWithDefault":
			val := doc.FloatValueAsFloat32(doc.ObjectFieldValue(ii).Ref)
			i.FloatWithDefault = val
		}
	}
}

type Methods struct {
	List []HTTP_METHOD
}

func (m *Methods) Unmarshal(doc *ast.Document, ref int) {
	for _, ii := range doc.ObjectValues[ref].Refs {
		name := string(doc.ObjectFieldNameBytes(ii))
		switch name {
		case "list":
			list := make([]HTTP_METHOD, 0, len(doc.ListValues[doc.ObjectFieldValue(ii).Ref].Refs))
			for _, ii := range doc.ListValues[doc.ObjectFieldValue(ii).Ref].Refs {
				var val HTTP_METHOD
				val.Unmarshal(doc, doc.Value(ii).Ref)
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
	for _, ii := range doc.ObjectValues[ref].Refs {
		name := string(doc.ObjectFieldNameBytes(ii))
		switch name {
		case "key":
			val := doc.StringValueContentString(doc.ObjectFieldValue(ii).Ref)
			h.Key = val
		case "value":
			val := doc.StringValueContentString(doc.ObjectFieldValue(ii).Ref)
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
	for _, ii := range doc.ObjectValues[ref].Refs {
		name := string(doc.ObjectFieldNameBytes(ii))
		switch name {
		case "name":
			val := doc.StringValueContentString(doc.ObjectFieldValue(ii).Ref)
			p.Name = val
		case "sourceKind":
			var val PARAMETER_SOURCE
			val.Unmarshal(doc, doc.ObjectFieldValue(ii).Ref)
			p.SourceKind = val
		case "sourceName":
			val := doc.StringValueContentString(doc.ObjectFieldValue(ii).Ref)
			p.SourceName = val
		case "variableName":
			val := doc.StringValueContentString(doc.ObjectFieldValue(ii).Ref)
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
