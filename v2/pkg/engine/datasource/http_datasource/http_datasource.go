package http_datasource

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type ObjMap map[string]interface{}

type GlobalConfiguration struct {
	SourceName         string `json:"sourceName"`
	Endpoint           string `json:"endpoint"`
	OperationHeaders   ObjMap `json:"operationHeaders"`
	QueryStringOptions ObjMap `json:"queryStringOptions"`
	QueryParams        ObjMap `json:"queryParams"`
}

type OperationConfiguration struct {
	TypeName                  string `json:"typeName"`
	FieldName                 string `json:"fieldName"`
	Path                      string `json:"path"`
	OperationSpecificHeaders  ObjMap `json:"operationSpecificHeaders"`
	HTTPMethod                string `json:"httpMethod"`
	IsBinary                  bool   `json:"isBinary"`
	RequestBaseBody           ObjMap `json:"requestBaseBody"`
	QueryParamArgMap          ObjMap `json:"queryParamArgMap"`
	QueryStringOptionsByParam ObjMap `json:"queryStringOptionsByParam"`
}

type Configuration struct {
	Global     GlobalConfiguration      `json:"global"`
	Operations []OperationConfiguration `json:"operations"`
}

func ConfigJson(config Configuration) json.RawMessage {
	out, err := json.Marshal(config)
	if err != nil {
		panic(err)
	}
	return out
}

type Planner struct {
	visitor      *plan.Visitor
	variables    resolve.Variables
	rootFieldRef int
	client       *http.Client
	config       Configuration
	current      struct {
		path      string
		operation *OperationConfiguration
	}
}

func (p *Planner) parsePathTemplate(ref int, tmpl string) (string, error) {
	r := bufio.NewReader(strings.NewReader(tmpl))
	var pathBuf bytes.Buffer
	for {
		c, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		switch c {
		case '{':
			// Read until '}' to retrieve the variable name
			s, err := r.ReadString('}')
			if err != nil {
				return "", err
			}
			// Trim the } from the end
			s = s[:len(s)-1]
			if !strings.HasPrefix(s, "args.") {
				return "", fmt.Errorf("variable name must start with args. got %s", s)
			}
			argName := s[len("args."):]
			// // We need to find the argument in the operation
			arg, ok := p.visitor.Operation.FieldArgument(ref, []byte(argName))
			if !ok {
				return "", fmt.Errorf("argument %s not found", argName)
			}
			argValue := p.visitor.Operation.ArgumentValue(arg)
			if argValue.Kind != ast.ValueKindVariable {
				return "", fmt.Errorf("argument %s is not a variable", argName)
			}
			variableName := p.visitor.Operation.VariableValueNameBytes(argValue.Ref)
			variableDefinition, ok := p.visitor.Operation.VariableDefinitionByNameAndOperation(p.visitor.Walker.Ancestors[0].Ref, variableName)
			if !ok {
				return "", fmt.Errorf("variable %s not found", variableName)
			}
			variableTypeRef := p.visitor.Operation.VariableDefinitions[variableDefinition].Type
			renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(p.visitor.Operation, p.visitor.Operation, variableTypeRef, string(variableName))
			if err != nil {
				return "", fmt.Errorf("error creating renderer for variable %s: %w", variableName, err)
			}
			contextVariable := &resolve.ContextVariable{
				Path:     []string{string(variableName)},
				Renderer: renderer,
			}
			variablePlaceHolder, exists := p.variables.AddVariable(contextVariable)
			if exists {
				return "", fmt.Errorf("variable %s already exists", variableName)
			}
			pathBuf.WriteString(variablePlaceHolder)
		case '}':
			return "", errors.New("unexpected '}'")
		default:
			pathBuf.WriteByte(c)
		}
	}
	return pathBuf.String(), nil
}

func (p *Planner) EnterField(ref int) {
	if p.rootFieldRef == -1 {
		p.rootFieldRef = ref
	} else {
		// This is a nested field, we don't need to do anything here
		return
	}
	fieldName := p.visitor.Operation.FieldNameString(ref)
	typeName := p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)
	var config *OperationConfiguration
	for _, cfg := range p.config.Operations {
		if cfg.TypeName == typeName && cfg.FieldName == fieldName {
			config = &cfg
			break
		}
	}
	if config == nil {
		return
	}

	p.current.operation = config
	path, err := p.parsePathTemplate(ref, config.Path)
	if err != nil {
		panic(fmt.Errorf("error parsing path template: %w", err))
	}
	p.current.path = path
}

func (p *Planner) EnterDocument(operation, definition *ast.Document) {
	p.rootFieldRef = -1
	p.current.path = ""
	p.current.operation = nil
}

func (p *Planner) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration, dataSourcePlannerConfiguration plan.DataSourcePlannerConfiguration) error {
	p.visitor = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	visitor.Walker.RegisterEnterDocumentVisitor(p)
	if err := json.Unmarshal(configuration.Custom, &p.config); err != nil {
		return err
	}
	return nil
}

func (p *Planner) ConfigureFetch() resolve.FetchConfiguration {
	op := p.current.operation
	if op == nil {
		panic(errors.New("config is nil, maybe query was not planned?"))
	}
	return resolve.FetchConfiguration{
		Input:      fmt.Sprintf(`{"endpoint": "%s", "path":"%s", "method": "%s"}`, p.config.Global.Endpoint, p.current.path, op.HTTPMethod),
		Variables:  p.variables,
		DataSource: &DataSource{client: p.client},
		PostProcessing: resolve.PostProcessingConfiguration{
			MergePath: []string{op.FieldName},
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{}
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
		IncludeTypeNameFields:      true,
	}
}

func (p *Planner) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	return "", false
}

func (p *Planner) UpstreamSchema(dataSourceConfig plan.DataSourceConfiguration) *ast.Document {
	return nil
}

type Factory struct {
	Client *http.Client
}

func (f *Factory) Planner(ctx context.Context) plan.DataSourcePlanner {
	return &Planner{
		client: f.Client,
	}
}

type DataSource struct {
	client *http.Client
}

var (
	dataSourceKeys = [][]string{
		{"endpoint"},
		{"path"},
		{"method"},
	}
)

const (
	dataSourceKeyEndpointIndex = iota
	dataSourceKeyPathIndex
	dataSourceKeyMethodIndex
)

func (s *DataSource) Load(ctx context.Context, input []byte, w io.Writer) error {
	var (
		endpoint string
		path     string
		method   string
	)
	var parseErr error
	jsonparser.EachKey(input, func(idx int, value []byte, vt jsonparser.ValueType, err error) {
		if parseErr != nil {
			return
		}
		if err != nil {
			parseErr = err
			return
		}
		switch idx {
		case dataSourceKeyEndpointIndex:
			endpoint = string(value)
		case dataSourceKeyPathIndex:
			path = string(value)
		case dataSourceKeyMethodIndex:
			method = string(value)
		}
	}, dataSourceKeys...)

	if parseErr != nil {
		return fmt.Errorf("error parsing input: %w", parseErr)
	}

	base, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("error parsing endpoint: %w", err)
	}

	u, err := base.Parse(path)
	if err != nil {
		return fmt.Errorf("error parsing path: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close()
	_, err = io.Copy(w, resp.Body)
	return err
}
