package execution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/introspection"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

type Executor struct {
	context Context
	out     io.Writer
	err     error
}

func (e *Executor) Execute(ctx Context, node Node, w io.Writer) error {
	e.context = ctx
	e.out = w
	e.err = nil
	e.resolveNode(node, nil)
	return e.err
}

func (e *Executor) write(data []byte) {
	if e.err != nil {
		return
	}
	_, e.err = e.out.Write(data)
}

func (e *Executor) resolveNode(node Node, data []byte) {
	switch node := node.(type) {
	case *Object:
		if data != nil && node.Path != nil {
			data, _, _, e.err = jsonparser.Get(data, node.Path...)
			if e.err == jsonparser.KeyPathNotFoundError {
				e.err = nil
				e.write(literal.NULL)
				return
			}
		}
		if bytes.Equal(data, literal.NULL) {
			e.write(literal.NULL)
			return
		}
		e.write(literal.LBRACE)
		for i := 0; i < len(node.Fields); i++ {
			if node.Fields[i].Skip != nil {
				if node.Fields[i].Skip.Evaluate(e.context, data) {
					continue
				}
			}
			if i != 0 {
				e.write(literal.COMMA)
			}
			e.resolveNode(&node.Fields[i], data)
		}
		e.write(literal.RBRACE)
	case *Field:
		if node.Resolve != nil {
			data = node.Resolve.Resolver.Resolve(e.context, e.resolveArgs(node.Resolve.Args, data))
		}
		e.write(literal.QUOTE)
		e.write(node.Name)
		e.write(literal.QUOTE)
		e.write(literal.COLON)
		if len(data) == 0 && !node.Value.HasResolvers() {
			e.write(literal.NULL)
			return
		}
		e.resolveNode(node.Value, data)
	case *Value:
		if bytes.Equal(data, literal.NULL) {
			e.write(literal.NULL)
			return
		}
		if len(node.Path) == 0 {
			if node.QuoteValue {
				e.write(literal.QUOTE)
			}
			e.write(data)
			if node.QuoteValue {
				e.write(literal.QUOTE)
			}
			return
		}
		data, _, _, e.err = jsonparser.Get(data, node.Path...)
		if e.err == jsonparser.KeyPathNotFoundError {
			e.err = nil
			e.write(literal.NULL)
			return
		}
		if node.QuoteValue {
			e.write(literal.QUOTE)
		}
		e.write(data)
		if node.QuoteValue {
			e.write(literal.QUOTE)
		}
	case *List:
		if len(data) == 0 {
			e.write(literal.NULL)
			return
		}
		first := true
		_, e.err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			if first {
				e.write(literal.LBRACK)
				first = !first
			} else {
				e.write(literal.COMMA)
			}
			e.resolveNode(node.Value, value)
		}, node.Path...)
		if first || e.err == jsonparser.KeyPathNotFoundError {
			e.err = nil
			e.write(literal.LBRACK)
		}
		e.write(literal.RBRACK)
	}
}

func (e *Executor) resolveArgs(args []Argument, data []byte) ResolvedArgs {
	resolved := make(ResolvedArgs, len(args))
	for i := 0; i < len(args); i++ {
		switch arg := args[i].(type) {
		case *StaticVariableArgument:
			resolved[i].Key = arg.Name
			resolved[i].Value = arg.Value
		case *ObjectVariableArgument:
			resolved[i].Key = arg.Name
			resolved[i].Value, _, _, _ = jsonparser.Get(data, arg.Path...)
		case *ContextVariableArgument:
			resolved[i].Key = arg.Name
			resolved[i].Value = e.context.Variables[xxhash.Sum64(arg.VariableName)]
		}
	}
	return resolved
}

const (
	ObjectKind NodeKind = iota + 1
	FieldKind
	ListKind
	ValueKind
)

type NodeKind int

type Node interface {
	Kind() NodeKind
	HasResolvers() bool
}

type Context struct {
	Variables Variables
}

type Variables map[uint64][]byte

type Argument interface {
	ArgName() []byte
}

type ResolvedArgument struct {
	Key   []byte
	Value []byte
}

type ResolvedArgs []ResolvedArgument

func (a ResolvedArgs) ByKey(key []byte) []byte {
	for i := 0; i < len(a); i++ {
		if bytes.Equal(a[i].Key, key) {
			return a[i].Value
		}
	}
	return nil
}

type ContextVariableArgument struct {
	Name         []byte
	VariableName []byte
}

func (c *ContextVariableArgument) ArgName() []byte {
	return c.Name
}

type ObjectVariableArgument struct {
	Name []byte
	Path []string
}

func (o *ObjectVariableArgument) ArgName() []byte {
	return o.Name
}

type StaticVariableArgument struct {
	Name  []byte
	Value []byte
}

func (s *StaticVariableArgument) ArgName() []byte {
	return s.Name
}

type Object struct {
	Fields []Field
	Path   []string
}

func (o *Object) HasResolvers() bool {
	for i := 0; i < len(o.Fields); i++ {
		if o.Fields[i].HasResolvers() {
			return true
		}
	}
	return false
}

func (*Object) Kind() NodeKind {
	return ObjectKind
}

type BooleanCondition interface {
	Evaluate(ctx Context, data []byte) bool
}

type Field struct {
	Name    []byte
	Value   Node
	Resolve *Resolve
	Skip    BooleanCondition
}

func (f *Field) HasResolvers() bool {
	if f.Resolve != nil {
		return true
	}
	return f.Value.HasResolvers()
}

type IfEqual struct {
	Left, Right Argument
}

func (i *IfEqual) Evaluate(ctx Context, data []byte) bool {
	var left []byte
	var right []byte

	switch value := i.Left.(type) {
	case *ContextVariableArgument:
		left = ctx.Variables[xxhash.Sum64(value.VariableName)]
	case *ObjectVariableArgument:
		left, _, _, _ = jsonparser.Get(data, value.Path...)
	case *StaticVariableArgument:
		left = value.Value
	}

	switch value := i.Right.(type) {
	case *ContextVariableArgument:
		right = ctx.Variables[xxhash.Sum64(value.VariableName)]
	case *ObjectVariableArgument:
		right, _, _, _ = jsonparser.Get(data, value.Path...)
	case *StaticVariableArgument:
		right = value.Value
	}

	return bytes.Equal(left, right)
}

type IfNotEqual struct {
	Left, Right Argument
}

func (i *IfNotEqual) Evaluate(ctx Context, data []byte) bool {
	equal := IfEqual{
		Left:  i.Left,
		Right: i.Right,
	}
	return !equal.Evaluate(ctx, data)
}

func (*Field) Kind() NodeKind {
	return FieldKind
}

type Value struct {
	Path       []string
	QuoteValue bool
}

func (value *Value) HasResolvers() bool {
	return false
}

func (*Value) Kind() NodeKind {
	return ValueKind
}

type List struct {
	Path  []string
	Value Node
}

func (l *List) HasResolvers() bool {
	return l.Value.HasResolvers()
}

func (*List) Kind() NodeKind {
	return ListKind
}

type Resolve struct {
	Args     []Argument
	Resolver Resolver
}

type Resolver interface {
	Resolve(ctx Context, args ResolvedArgs) []byte
	DirectiveName() []byte
}

type TypeResolver struct {
}

func (t *TypeResolver) DirectiveName() []byte {
	return []byte("resolveType")
}

type SchemaResolver struct {
	schemaBytes []byte
}

func NewSchemaResolver(definition *ast.Document, report *operationreport.Report) *SchemaResolver {
	gen := introspection.NewGenerator()
	var data introspection.Data
	gen.Generate(definition, report, &data)
	schemaBytes, err := json.Marshal(data)
	if err != nil {
		report.AddInternalError(err)
	}
	return &SchemaResolver{
		schemaBytes: schemaBytes,
	}
}

func (s *SchemaResolver) Resolve(ctx Context, args ResolvedArgs) []byte {
	return s.schemaBytes
}

func (s *SchemaResolver) DirectiveName() []byte {
	return []byte("resolveSchema")
}

var userType = []byte(`{
			  "__type": {
				"name": "User",
				"fields": [
				  {
					"name": "id",
					"type": { "name": "String" }
				  },
				  {
					"name": "name",
					"type": { "name": "String" }
				  },
				  {
					"name": "birthday",
					"type": { "name": "Date" }
				  }
				]
			  }
			}`)

var userData = []byte(`
		{
			"data":	{
				"user":	{
					"id":1,
					"name":"Jens",
					"birthday":"08.02.1988"
				}
			}
		}`)

var userRestData = []byte(`
{
	"id":1,
	"name":"Jens",
	"birthday":"08.02.1988"
}`)

var friendsData = []byte(`[
   {
      "id":2,
      "name":"Yaara",
      "birthday":"1990 I guess? ;-)"
   },
   {
      "id":3,
      "name":"Ahmet",
      "birthday":"1980"
   }]`)

var petsData = []byte(`{
   "data":{
      "userPets":[{
            "__typename":"Dog",
            "name":"Paw",
            "nickname":"Pawie",
            "woof":"Woof! Woof!"
         },
         {
            "__typename":"Cat",
            "name":"Mietz",
            "nickname":"Mietzie",
            "meow":"Meow meow!"
         }]}
}`)

func (t *TypeResolver) Resolve(ctx Context, args ResolvedArgs) []byte {
	return userType
}

type GraphQLResolver struct {
	Upstream string
	URL      string
}

func (g *GraphQLResolver) DirectiveName() []byte {
	return []byte("GraphQLDataSource")
}

func (g *GraphQLResolver) Resolve(ctx Context, args ResolvedArgs) []byte {

	hostArg := args.ByKey(literal.HOST)
	urlArg := args.ByKey(literal.URL)
	queryArg := args.ByKey(literal.QUERY)

	if hostArg == nil || urlArg == nil || queryArg == nil {
		log.Fatal("one of host,url,query arg nil")
		return nil
	}

	url := "https://" + string(hostArg) + string(urlArg)

	fmt.Printf("GraphQLDataSource - url: %s\n", url)

	variables := map[string]json.RawMessage{}
	for i := 0; i < len(args); i++ {
		key := args[i].Key
		switch {
		case bytes.Equal(key, literal.HOST):
		case bytes.Equal(key, literal.URL):
		case bytes.Equal(key, literal.QUERY):
		default:
			variables[string(key)] = args[i].Value
		}
	}

	gqlRequest := GraphqlRequest{
		OperationName: "o",
		Variables:     variables,
		Query:         string(queryArg),
	}

	gqlRequestData, err := json.MarshalIndent(gqlRequest, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("GraphQLDataSource - request:\n%s\n", string(gqlRequestData))

	client := http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 0 * time.Second,
		},
	}

	request, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(gqlRequestData))
	if err != nil {
		log.Fatal(err)
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")

	res, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("GraphQLDataSource - response:\n%s\n", string(data))

	data = bytes.ReplaceAll(data, literal.BACKSLASH, nil)
	data, _, _, err = jsonparser.Get(data, "data")
	if err != nil {
		log.Fatal(err)
	}
	return data
}

type RESTResolver struct{}

func (r *RESTResolver) DirectiveName() []byte {
	return []byte("RESTDataSource")
}

func (r *RESTResolver) Resolve(ctx Context, args ResolvedArgs) []byte {

	hostArg := args.ByKey(literal.HOST)
	urlArg := args.ByKey(literal.URL)

	if hostArg == nil || urlArg == nil {
		return nil
	}

	url := "https://" + string(hostArg) + string(urlArg)

	if strings.Contains(url, "{{") {
		tmpl, err := template.New("url").Parse(url)
		if err != nil {
			log.Fatal(err)
		}
		out := bytes.Buffer{}
		data := make(map[string]string, len(args))
		for i := 0; i < len(args); i++ {
			data[string(args[i].Key)] = string(args[i].Value)
		}
		err = tmpl.Execute(&out, data)
		if err != nil {
			log.Fatal(err)
		}
		url = out.String()
	}

	fmt.Printf("RESTDataSource - url: %s\n", url)

	client := http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 0 * time.Second,
		},
	}

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return []byte(err.Error())
	}

	request.Header.Add("Accept", "application/json")

	res, err := client.Do(request)
	if err != nil {
		return []byte(err.Error())
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return []byte(err.Error())
	}
	return bytes.ReplaceAll(data, literal.BACKSLASH, nil)

	/*if len(args) < 1 {
		return []byte("expect arg 1: url")
	}

	if !bytes.Equal(args[0].ArgName(), literal.URL) {
		return []byte("first arg must be named url")
	}

	if bytes.Equal(args[0].(*StaticVariableArgument).Value, []byte("/user/:id")) {
		return userRestData
	}

	if bytes.Equal(args[0].(*StaticVariableArgument).Value, []byte("/user/:id/friends")) {
		return friendsData
	}

	return nil*/
}

type StaticDataSource struct {
}

func (s StaticDataSource) Resolve(ctx Context, args ResolvedArgs) []byte {
	return args[0].Value
}

func (s StaticDataSource) DirectiveName() []byte {
	return []byte("StaticDataSource")
}
