package resolve

import (
	"context"
	"io"
	"sync"

	"github.com/buger/jsonparser"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

const (
	VariableRendererKindPlain                 = "plain"
	VariableRendererKindJson                  = "json"
	VariableRendererKindGraphqlWithValidation = "graphqlWithValidation"
	VariableRendererKindGraphqlResolve        = "graphqlResolve"
	VariableRendererKindCsv                   = "csv"
)

// VariableRenderer is the interface to allow custom implementations of rendering Variables
// Depending on where a Variable is being used, a different method for rendering is required
// E.g. a Variable needs to be rendered conforming to the GraphQL specification, when used within a GraphQL Query
// If a Variable is used within a JSON Object, the contents need to be rendered as a JSON Object
type VariableRenderer interface {
	GetKind() string
	RenderVariable(ctx context.Context, data *astjson.Value, out io.Writer) error
}

// JSONVariableRenderer is an implementation of VariableRenderer
// It renders the provided data as JSON
// If configured, it also does a JSON Validation Check before rendering
type JSONVariableRenderer struct {
	Kind          string
	rootValueType JsonRootType
}

func (r *JSONVariableRenderer) GetKind() string {
	return r.Kind
}

func (r *JSONVariableRenderer) RenderVariable(ctx context.Context, data *astjson.Value, out io.Writer) error {
	content := data.MarshalTo(nil)
	_, err := out.Write(content)
	return err
}

func NewJSONVariableRenderer() *JSONVariableRenderer {
	return &JSONVariableRenderer{
		Kind: VariableRendererKindJson,
	}
}

func NewPlainVariableRenderer() *PlainVariableRenderer {
	return &PlainVariableRenderer{
		Kind: VariableRendererKindPlain,
	}
}

// PlainVariableRenderer is an implementation of VariableRenderer
// It renders the provided data as plain text
// E.g. a provided JSON string of "foo" will be rendered as foo, without quotes.
// If a nested JSON Object is provided, it will be rendered as is.
// This renderer can be used e.g. to render the provided scalar into a URL.
type PlainVariableRenderer struct {
	JSONSchema string
	Kind       string
}

func (p *PlainVariableRenderer) GetKind() string {
	return p.Kind
}

func (p *PlainVariableRenderer) RenderVariable(ctx context.Context, data *astjson.Value, out io.Writer) error {
	if data.Type() == astjson.TypeString {
		_, err := out.Write(data.GetStringBytes())
		return err
	}
	content := data.MarshalTo(nil)
	_, err := out.Write(content)
	return err
}

func NewGraphQLVariableRendererFromTypeRefWithoutValidation(operation, definition *ast.Document, variableTypeRef int) (*GraphQLVariableRenderer, error) {
	return &GraphQLVariableRenderer{
		Kind:          VariableRendererKindGraphqlWithValidation,
		rootValueType: getJSONRootType(operation, definition, variableTypeRef),
	}, nil
}

type JsonRootTypeKind int

const (
	JsonRootTypeKindSingle JsonRootTypeKind = iota
	JsonRootTypeKindMultiple
)

type JsonRootType struct {
	Value  jsonparser.ValueType
	Values []jsonparser.ValueType
	Kind   JsonRootTypeKind
}

func (t JsonRootType) Satisfies(dataType jsonparser.ValueType) bool {
	switch t.Kind {
	case JsonRootTypeKindSingle:
		return dataType == t.Value
	case JsonRootTypeKindMultiple:
		for _, valueType := range t.Values {
			if dataType == valueType {
				return true
			}
		}
	}

	return false
}

func getJSONRootType(operation, definition *ast.Document, variableTypeRef int) JsonRootType {
	variableTypeRef = operation.ResolveListOrNameType(variableTypeRef)
	if operation.TypeIsList(variableTypeRef) {
		return JsonRootType{
			Value: jsonparser.Array,
			Kind:  JsonRootTypeKindSingle,
		}
	}

	name := operation.TypeNameString(variableTypeRef)
	node, exists := definition.Index.FirstNodeByNameStr(name)
	if !exists {
		return JsonRootType{
			Value: jsonparser.Unknown,
			Kind:  JsonRootTypeKindSingle,
		}
	}

	defTypeRef := node.Ref

	if node.Kind == ast.NodeKindEnumTypeDefinition {
		return JsonRootType{
			Value: jsonparser.String,
			Kind:  JsonRootTypeKindSingle,
		}
	}
	if node.Kind == ast.NodeKindScalarTypeDefinition {
		typeName := definition.ScalarTypeDefinitionNameString(defTypeRef)
		switch typeName {
		case "Boolean":
			return JsonRootType{
				Value: jsonparser.Boolean,
				Kind:  JsonRootTypeKindSingle,
			}
		case "Int", "Float":
			return JsonRootType{
				Value: jsonparser.Number,
				Kind:  JsonRootTypeKindSingle,
			}
		case "ID":
			return JsonRootType{
				Values: []jsonparser.ValueType{jsonparser.String, jsonparser.Number},
				Kind:   JsonRootTypeKindMultiple,
			}
		case "String", "Date":
			return JsonRootType{
				Value: jsonparser.String,
				Kind:  JsonRootTypeKindSingle,
			}
		case "_Any":
			return JsonRootType{
				Value: jsonparser.Object,
				Kind:  JsonRootTypeKindSingle,
			}
		default:
			return JsonRootType{
				Value: jsonparser.String,
				Kind:  JsonRootTypeKindSingle,
			}
		}
	}

	return JsonRootType{
		Value: jsonparser.Object,
		Kind:  JsonRootTypeKindSingle,
	}
}

// GraphQLVariableRenderer is an implementation of VariableRenderer
// It renders variables according to the GraphQL Specification
type GraphQLVariableRenderer struct {
	JSONSchema    string
	Kind          string
	rootValueType JsonRootType
}

func (g *GraphQLVariableRenderer) GetKind() string {
	return g.Kind
}

// add renderer that renders both variable name and variable value
// before rendering, evaluate if the value contains null values
// if an object contains only null values, set the object to null
// do this recursively until reaching the root of the object

func (g *GraphQLVariableRenderer) RenderVariable(ctx context.Context, data *astjson.Value, out io.Writer) error {
	return g.renderGraphQLValue(data, out)
}

func (g *GraphQLVariableRenderer) renderGraphQLValue(data *astjson.Value, out io.Writer) (err error) {
	if data == nil {
		_, _ = out.Write(literal.NULL)
		return
	}
	switch data.Type() {
	case astjson.TypeString:
		_, _ = out.Write(literal.BACKSLASH)
		_, _ = out.Write(literal.QUOTE)
		b := data.GetStringBytes()
		for i := range b {
			switch b[i] {
			case '"':
				_, _ = out.Write(literal.BACKSLASH)
				_, _ = out.Write(literal.BACKSLASH)
				_, _ = out.Write(literal.QUOTE)
			default:
				_, _ = out.Write(b[i : i+1])
			}
		}
		_, _ = out.Write(literal.BACKSLASH)
		_, _ = out.Write(literal.QUOTE)
	case astjson.TypeObject:
		_, _ = out.Write(literal.LBRACE)
		o := data.GetObject()
		first := true
		o.Visit(func(k []byte, v *astjson.Value) {
			if err != nil {
				return
			}
			if !first {
				_, _ = out.Write(literal.COMMA)
			} else {
				first = false
			}
			_, _ = out.Write(k)
			_, _ = out.Write(literal.COLON)
			err = g.renderGraphQLValue(v, out)
		})
		if err != nil {
			return err
		}
		_, _ = out.Write(literal.RBRACE)
	case astjson.TypeNull:
		_, _ = out.Write(literal.NULL)
	case astjson.TypeTrue:
		_, _ = out.Write(literal.TRUE)
	case astjson.TypeFalse:
		_, _ = out.Write(literal.FALSE)
	case astjson.TypeArray:
		_, _ = out.Write(literal.LBRACK)
		arr := data.GetArray()
		for i, value := range arr {
			if i > 0 {
				_, _ = out.Write(literal.COMMA)
			}
			err = g.renderGraphQLValue(value, out)
			if err != nil {
				return err
			}
		}
		_, _ = out.Write(literal.RBRACK)
	case astjson.TypeNumber:
		b := data.MarshalTo(nil)
		_, _ = out.Write(b)
	}
	return
}

func NewCSVVariableRenderer(arrayValueType JsonRootType) *CSVVariableRenderer {
	return &CSVVariableRenderer{
		Kind:           VariableRendererKindCsv,
		arrayValueType: arrayValueType,
	}
}

func NewCSVVariableRendererFromTypeRef(operation, definition *ast.Document, variableTypeRef int) *CSVVariableRenderer {
	return &CSVVariableRenderer{
		Kind:           VariableRendererKindCsv,
		arrayValueType: getJSONRootType(operation, definition, variableTypeRef),
	}
}

// CSVVariableRenderer is an implementation of VariableRenderer
// It renders the provided list of Values as comma separated Values in plaintext (no JSON encoding of Values)
type CSVVariableRenderer struct {
	Kind           string
	arrayValueType JsonRootType
}

func (c *CSVVariableRenderer) GetKind() string {
	return c.Kind
}

func (c *CSVVariableRenderer) RenderVariable(_ context.Context, data *astjson.Value, out io.Writer) (err error) {
	arr := data.GetArray()
	for i := range arr {
		if i > 0 {
			_, err = out.Write(literal.COMMA)
			if err != nil {
				return err
			}
		}
		if arr[i].Type() == astjson.TypeString {
			b := arr[i].GetStringBytes()
			_, err = out.Write(b)
			if err != nil {
				return err
			}
		} else {
			_, err = out.Write(arr[i].MarshalTo(nil))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type GraphQLVariableResolveRenderer struct {
	Kind string
	Node Node
}

func NewGraphQLVariableResolveRenderer(node Node) *GraphQLVariableResolveRenderer {
	return &GraphQLVariableResolveRenderer{
		Kind: VariableRendererKindGraphqlResolve,
		Node: node,
	}
}

func (g *GraphQLVariableResolveRenderer) GetKind() string {
	return g.Kind
}

var (
	_graphQLVariableResolveRendererPool = &sync.Pool{}
)

func (g *GraphQLVariableResolveRenderer) getResolvable() *Resolvable {
	v := _graphQLVariableResolveRendererPool.Get()
	if v == nil {
		return NewResolvable(nil, ResolvableOptions{})
	}
	return v.(*Resolvable)
}

func (g *GraphQLVariableResolveRenderer) putResolvable(r *Resolvable) {
	r.Reset()
	_graphQLVariableResolveRendererPool.Put(r)
}

func (g *GraphQLVariableResolveRenderer) RenderVariable(ctx context.Context, data *astjson.Value, out io.Writer) error {
	r := g.getResolvable()
	defer g.putResolvable(r)

	// make depth 1 to not render as the root object fields - we need braces
	r.depth = 1
	return r.ResolveNode(g.Node, data, out)
}
