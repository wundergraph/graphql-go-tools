package execution

import (
	"bytes"
	"fmt"
	"github.com/buger/jsonparser"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func NewHttpJsonDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *HttpJsonDataSourcePlanner {
	return &HttpJsonDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

type HttpJsonDataSourcePlanner struct {
	BaseDataSourcePlanner
	rootField int
}

func (h *HttpJsonDataSourcePlanner) DirectiveDefinition() []byte {
	data, _ := h.graphqlDefinitions.Find("directives/http_json_datasource.graphql")
	return data
}

func (h *HttpJsonDataSourcePlanner) DirectiveName() []byte {
	return []byte("HttpJsonDataSource")
}

func (h *HttpJsonDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	h.walker, h.operation, h.definition, h.args = walker, operation, definition, args
	h.rootField = -1
}

func (h *HttpJsonDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &HttpJsonDataSource{
		log: h.log,
	}, h.args
}

func (h *HttpJsonDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (h *HttpJsonDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (h *HttpJsonDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (h *HttpJsonDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (h *HttpJsonDataSourcePlanner) EnterField(ref int) {
	if h.rootField == -1 {
		h.rootField = ref
	}
}

func (h *HttpJsonDataSourcePlanner) LeaveField(ref int) {
	if h.rootField != ref {
		return
	}
	definition, exists := h.walker.FieldDefinition(ref)
	if !exists {
		return
	}
	directive, exists := h.definition.FieldDefinitionDirectiveByName(definition, h.DirectiveName())
	if !exists {
		return
	}
	value, exists := h.definition.DirectiveArgumentValueByName(directive, literal.URL)
	if !exists {
		return
	}
	variableValue := h.definition.StringValueContentBytes(value.Ref)
	arg := &StaticVariableArgument{
		Name:  literal.URL,
		Value: make([]byte, len(variableValue)),
	}
	copy(arg.Value, variableValue)
	h.args = append([]Argument{arg}, h.args...)
	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.HOST)
	if !exists {
		return
	}
	variableValue = h.definition.StringValueContentBytes(value.Ref)
	arg = &StaticVariableArgument{
		Name:  literal.HOST,
		Value: make([]byte, len(variableValue)),
	}
	copy(arg.Value, variableValue)
	h.args = append([]Argument{arg}, h.args...)

	// method
	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.METHOD)
	if exists {
		variableValue = h.definition.EnumValueNameBytes(value.Ref)
		arg = &StaticVariableArgument{
			Name:  literal.METHOD,
			Value: make([]byte, len(variableValue)),
		}
		copy(arg.Value, variableValue)
		h.args = append(h.args, arg)
	} else { // must refactor into functions!
		inputValueDefinition := h.definition.DirectiveArgumentInputValueDefinition(h.definition.DirectiveNameBytes(directive), literal.METHOD)
		if inputValueDefinition != -1 {
			if h.definition.InputValueDefinitionHasDefaultValue(inputValueDefinition) {
				defaultValue := h.definition.InputValueDefinitionDefaultValue(inputValueDefinition)
				if defaultValue.Kind == ast.ValueKindEnum {
					value := h.definition.EnumValueNameBytes(defaultValue.Ref)
					arg = &StaticVariableArgument{
						Name:  literal.METHOD,
						Value: make([]byte, len(value)),
					}
					copy(arg.Value, value)
					h.args = append(h.args, arg)
				}
			}
		}
	}

	// body
	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.BODY)
	if exists {
		variableValue = h.definition.StringValueContentBytes(value.Ref)
		arg = &StaticVariableArgument{
			Name:  literal.BODY,
			Value: make([]byte, len(variableValue)),
		}
		copy(arg.Value, variableValue)
		h.args = append(h.args, arg)
	}

	// args
	if h.operation.FieldHasArguments(ref) {
		args := h.operation.FieldArguments(ref)
		for _, i := range args {
			argName := h.operation.ArgumentNameBytes(i)
			value := h.operation.ArgumentValue(i)
			if value.Kind != ast.ValueKindVariable {
				continue
			}
			variableName := h.operation.VariableValueNameBytes(value.Ref)
			name := append([]byte(".arguments."), argName...)
			arg := &ContextVariableArgument{
				VariableName: variableName,
				Name:         make([]byte, len(name)),
			}
			copy(arg.Name, name)
			h.args = append(h.args, arg)
		}
	}

	// headers
	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.HEADERS)
	if exists && value.Kind == ast.ValueKindList {
		listArg := &ListArgument{
			Name: literal.HEADERS,
		}
		for _, i := range h.definition.ListValues[value.Ref].Refs {
			listValue := h.definition.Values[i]
			if listValue.Kind != ast.ValueKindObject {
				continue
			}
			fields := h.definition.ObjectValues[listValue.Ref].Refs
			var key ast.ByteSlice
			var value ast.ByteSlice
			if len(fields) != 2 {
				continue
			}
			for _, j := range fields {
				fieldName := h.definition.ObjectFieldNameBytes(j)
				switch {
				case bytes.Equal(fieldName, literal.KEY):
					key = h.definition.StringValueContentBytes(h.definition.ObjectFieldValue(j).Ref)
				case bytes.Equal(fieldName, literal.VALUE):
					value = h.definition.StringValueContentBytes(h.definition.ObjectFieldValue(j).Ref)
				}
			}
			if key == nil || value == nil {
				continue
			}
			arg := &StaticVariableArgument{
				Name:  make([]byte, len(key)),
				Value: make([]byte, len(value)),
			}
			copy(arg.Name, key)
			copy(arg.Value, value)
			listArg.Arguments = append(listArg.Arguments, arg)
		}

		if len(listArg.Arguments) != 0 {
			h.args = append(h.args, listArg)
		}
	}

	// __typename
	var typeNameValue []byte
	var err error
	fieldDefinitionTypeNode := h.definition.FieldDefinitionTypeNode(definition)
	fieldDefinitionType := h.definition.FieldDefinitionType(definition)
	fieldDefinitionTypeName := h.definition.ResolveTypeName(fieldDefinitionType)
	quotedFieldDefinitionTypeName := append(literal.QUOTE, append(fieldDefinitionTypeName, literal.QUOTE...)...)
	switch fieldDefinitionTypeNode.Kind {
	case ast.NodeKindScalarTypeDefinition:
		return
	case ast.NodeKindUnionTypeDefinition, ast.NodeKindInterfaceTypeDefinition:
		value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.DEFAULT_TYPENAME)
		if exists && value.Kind == ast.ValueKindString {
			defaultTypeName := h.definition.StringValueContentBytes(value.Ref)
			quotedDefaultTypeName := append(literal.QUOTE, append(defaultTypeName, literal.QUOTE...)...)
			typeNameValue, err = sjson.SetRawBytes(typeNameValue, "defaultTypeName", quotedDefaultTypeName)
			if err != nil {
				h.log.Error("HttpJsonDataSourcePlanner set defaultTypeName (switch case union/interface)", log.Error(err))
				return
			}
		}
		value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.STATUS_CODE_TYPENAME_MAPPINGS)
		if exists && value.Kind == ast.ValueKindList {
			for _, i := range h.definition.ListValues[value.Ref].Refs {
				var statusCode []byte
				var typeName []byte
				listItem := h.definition.Value(i)
				if listItem.Kind != ast.ValueKindObject {
					continue
				}
				for _, j := range h.definition.ObjectValues[listItem.Ref].Refs {
					fieldName := h.definition.ObjectFieldNameBytes(j)
					fieldValue := h.definition.ObjectFieldValue(j)
					switch unsafebytes.BytesToString(fieldName) {
					case "statusCode":
						if fieldValue.Kind != ast.ValueKindInteger {
							continue
						}
						statusCode = h.definition.IntValueRaw(fieldValue.Ref)
					case "typeName":
						if fieldValue.Kind != ast.ValueKindString {
							continue
						}
						typeName = append(literal.QUOTE, append(h.definition.StringValueContentBytes(fieldValue.Ref), literal.QUOTE...)...)
					}
				}
				if statusCode != nil && typeName != nil {
					typeNameValue, err = sjson.SetRawBytes(typeNameValue, unsafebytes.BytesToString(statusCode), typeName)
					if err != nil {
						h.log.Error("HttpJsonDataSourcePlanner set statusCodeTypeMapping", log.Error(err))
						return
					}
				}
			}
		}
	default:
		typeNameValue, err = sjson.SetRawBytes(typeNameValue, "defaultTypeName", quotedFieldDefinitionTypeName)
		if err != nil {
			h.log.Error("HttpJsonDataSourcePlanner set defaultTypeName (switch case default)", log.Error(err))
			return
		}
	}
	h.args = append(h.args, &StaticVariableArgument{
		Name:  literal.TYPENAME,
		Value: typeNameValue,
	})
}

type HttpJsonDataSource struct {
	log log.Logger
}

func (r *HttpJsonDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {

	hostArg := args.ByKey(literal.HOST)
	urlArg := args.ByKey(literal.URL)
	methodArg := args.ByKey(literal.METHOD)
	bodyArg := args.ByKey(literal.BODY)
	headersArg := args.ByKey(literal.HEADERS)
	typeNameArg := args.ByKey(literal.TYPENAME)

	r.log.Debug("HttpJsonDataSource.Resolve.args",
		log.Strings("resolvedArgs", args.Dump()),
	)

	switch {
	case hostArg == nil:
		r.log.Error(fmt.Sprintf("arg '%s' must not be nil", string(literal.HOST)))
		return CloseConnectionIfNotStream
	case urlArg == nil:
		r.log.Error(fmt.Sprintf("arg '%s' must not be nil", string(literal.URL)))
		return CloseConnectionIfNotStream
	case methodArg == nil:
		r.log.Error(fmt.Sprintf("arg '%s' must not be nil", string(literal.METHOD)))
		return CloseConnectionIfNotStream
	}

	httpMethod := http.MethodGet
	switch {
	case bytes.Equal(methodArg, literal.HTTP_METHOD_GET):
		httpMethod = http.MethodGet
	case bytes.Equal(methodArg, literal.HTTP_METHOD_POST):
		httpMethod = http.MethodPost
	case bytes.Equal(methodArg, literal.HTTP_METHOD_PUT):
		httpMethod = http.MethodPut
	case bytes.Equal(methodArg, literal.HTTP_METHOD_DELETE):
		httpMethod = http.MethodDelete
	case bytes.Equal(methodArg, literal.HTTP_METHOD_PATCH):
		httpMethod = http.MethodPatch
	}

	url := string(hostArg) + string(urlArg)
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		url = "https://" + url
	}

	header := make(http.Header)
	if len(headersArg) != 0 {
		err := jsonparser.ObjectEach(headersArg, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			header.Set(string(key), string(value))
			return nil
		})
		if err != nil {
			r.log.Error("accessing headers", log.Error(err))
		}
	}

	r.log.Debug("HttpJsonDataSource.Resolve",
		log.String("url", url),
	)

	client := http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 0 * time.Second,
		},
	}

	var bodyReader io.Reader
	if len(bodyArg) != 0 {
		bodyArg = bytes.ReplaceAll(bodyArg, literal.BACKSLASH, nil)
		bodyReader = bytes.NewReader(bodyArg)
	}

	request, err := http.NewRequest(httpMethod, url, bodyReader)
	if err != nil {
		r.log.Error("HttpJsonDataSource.Resolve.NewRequest",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	request.Header = header

	res, err := client.Do(request)
	if err != nil {
		r.log.Error("HttpJsonDataSource.Resolve.client.Do",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		r.log.Error("HttpJsonDataSource.Resolve.ioutil.ReadAll",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	statusCode := strconv.Itoa(res.StatusCode)
	statusCodeTypeName := gjson.GetBytes(typeNameArg,statusCode)
	if statusCodeTypeName.Exists() {
		data,err = sjson.SetRawBytes(data,"__typename",[]byte(statusCodeTypeName.Raw))
		if err != nil {
			r.log.Error("HttpJsonDataSource.Resolve.setStatusCodeTypeName",
				log.Error(err),
			)
			return CloseConnectionIfNotStream
		}
	} else {
		defaultTypeName := gjson.GetBytes(typeNameArg,"defaultTypeName")
		if defaultTypeName.Exists() {
			data,err = sjson.SetRawBytes(data,"__typename",[]byte(defaultTypeName.Raw))
			if err != nil {
				r.log.Error("HttpJsonDataSource.Resolve.setDefaultTypeName",
					log.Error(err),
				)
				return CloseConnectionIfNotStream
			}
		}
	}

	_, err = out.Write(data)
	if err != nil {
		r.log.Error("HttpJsonDataSource.Resolve.out.Write",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}
	return CloseConnectionIfNotStream
}
