package execution

import (
	"bytes"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"net/http"
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
}

type HttpJsonDataSource struct {
	log *zap.Logger
}

func (r *HttpJsonDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {

	hostArg := args.ByKey(literal.HOST)
	urlArg := args.ByKey(literal.URL)
	methodArg := args.ByKey(literal.METHOD)
	bodyArg := args.ByKey(literal.BODY)
	headersArg := args.ByKey(literal.HEADERS)

	r.log.Debug("HttpJsonDataSource.Resolve.args",
		zap.Strings("resolvedArgs", args.Dump()),
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
			r.log.Error("accessing headers", zap.Error(err))
		}
	}

	r.log.Debug("HttpJsonDataSource.Resolve",
		zap.String("url", url),
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
			zap.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	request.Header = header

	res, err := client.Do(request)
	if err != nil {
		r.log.Error("HttpJsonDataSource.Resolve.client.Do",
			zap.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		r.log.Error("HttpJsonDataSource.Resolve.ioutil.ReadAll",
			zap.Error(err),
		)
		return CloseConnectionIfNotStream
	}
	_, err = out.Write(data)
	if err != nil {
		r.log.Error("HttpJsonDataSource.Resolve.out.Write",
			zap.Error(err),
		)
		return CloseConnectionIfNotStream
	}
	return CloseConnectionIfNotStream
}
