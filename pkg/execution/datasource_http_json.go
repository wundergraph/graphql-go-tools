package execution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/buger/jsonparser"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
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

// HttpJsonDataSourceConfig is the configuration object for the HttpJsonDataSource
type HttpJsonDataSourceConfig struct {
	// Host is the hostname of the upstream
	Host string
	// URL is the url of the upstream
	URL string
	// Method is the http.Method, e.g. GET, POST, UPDATE, DELETE
	// default is GET
	Method *string
	// Body is the http body to send
	// default is null/nil (no body)
	Body *string
	// Headers defines the header mappings
	Headers []HttpJsonDataSourceConfigHeader
	// DefaultTypeName is the optional variable to define a default type name for the response object
	// This is useful in case the response might be a Union or Interface type which uses StatusCodeTypeNameMappings
	DefaultTypeName *string
	// StatusCodeTypeNameMappings is a slice of mappings from http.StatusCode to GraphQL TypeName
	// This can be used when the TypeName depends on the http.StatusCode
	StatusCodeTypeNameMappings []StatusCodeTypeNameMapping
}

type StatusCodeTypeNameMapping struct {
	StatusCode int
	TypeName   string
}

type HttpJsonDataSourceConfigHeader struct {
	Key   string
	Value string
}

func NewHttpJsonDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *HttpJsonDataSourcePlanner {
	return &HttpJsonDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

type HttpJsonDataSourcePlanner struct {
	BaseDataSourcePlanner
	rootField int
	config    HttpJsonDataSourceConfig
}

func (h *HttpJsonDataSourcePlanner) DataSourceName() string {
	return "HttpJsonDataSource"
}

func (h *HttpJsonDataSourcePlanner) Initialize(config DataSourcePlannerConfiguration) (err error) {
	h.walker, h.operation, h.definition = config.walker, config.operation, config.definition
	h.rootField = -1
	return json.NewDecoder(config.dataSourceConfiguration).Decode(&h.config)
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
	h.args = append(h.args, &StaticVariableArgument{
		Name:  literal.HOST,
		Value: []byte(h.config.Host),
	})
	h.args = append(h.args, &StaticVariableArgument{
		Name:  literal.URL,
		Value: []byte(h.config.URL),
	})
	if h.config.Method == nil {
		h.args = append(h.args, &StaticVariableArgument{
			Name:  literal.METHOD,
			Value: literal.HTTP_METHOD_GET,
		})
	} else {
		h.args = append(h.args, &StaticVariableArgument{
			Name:  literal.METHOD,
			Value: []byte(*h.config.Method),
		})
	}
	if h.config.Body != nil {
		h.args = append(h.args, &StaticVariableArgument{
			Name:  literal.BODY,
			Value: []byte(*h.config.Body),
		})
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

	if len(h.config.Headers) != 0 {
		listArg := &ListArgument{
			Name: literal.HEADERS,
		}
		for i := range h.config.Headers {
			listArg.Arguments = append(listArg.Arguments, &StaticVariableArgument{
				Name:  []byte(h.config.Headers[i].Key),
				Value: []byte(h.config.Headers[i].Value),
			})
		}
		h.args = append(h.args, listArg)
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
		if h.config.DefaultTypeName != nil {
			typeNameValue, err = sjson.SetRawBytes(typeNameValue, "defaultTypeName", []byte("\""+*h.config.DefaultTypeName+"\""))
			if err != nil {
				h.log.Error("HttpJsonDataSourcePlanner set defaultTypeName (switch case union/interface)", log.Error(err))
				return
			}
		}
		for i := range h.config.StatusCodeTypeNameMappings {
			typeNameValue, err = sjson.SetRawBytes(typeNameValue, strconv.Itoa(h.config.StatusCodeTypeNameMappings[i].StatusCode), []byte("\""+h.config.StatusCodeTypeNameMappings[i].TypeName+"\""))
			if err != nil {
				h.log.Error("HttpJsonDataSourcePlanner set statusCodeTypeMapping", log.Error(err))
				return
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
	statusCodeTypeName := gjson.GetBytes(typeNameArg, statusCode)
	if statusCodeTypeName.Exists() {
		data, err = sjson.SetRawBytes(data, "__typename", []byte(statusCodeTypeName.Raw))
		if err != nil {
			r.log.Error("HttpJsonDataSource.Resolve.setStatusCodeTypeName",
				log.Error(err),
			)
			return CloseConnectionIfNotStream
		}
	} else {
		defaultTypeName := gjson.GetBytes(typeNameArg, "defaultTypeName")
		if defaultTypeName.Exists() {
			data, err = sjson.SetRawBytes(data, "__typename", []byte(defaultTypeName.Raw))
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
