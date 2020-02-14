package execution

import (
	"bytes"
	"encoding/json"
	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	log "github.com/jensneuse/abstractlogger"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type GraphQLDataSourcePlanner struct {
	BaseDataSourcePlanner

	importer *astimport.Importer

	nodes           []ast.Node
	resolveDocument *ast.Document

	rootFieldRef          int
	rootFieldArgumentRefs []int
	variableDefinitions   []int
}

func NewGraphQLDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *GraphQLDataSourcePlanner {
	return &GraphQLDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
		importer:              astimport.NewImporter(),
	}
}

func (g *GraphQLDataSourcePlanner) DirectiveDefinition() []byte {
	data, _ := g.graphqlDefinitions.Find("directives/graphql_datasource.graphql")
	return data
}

func (g *GraphQLDataSourcePlanner) DirectiveName() []byte {
	return []byte("GraphQLDataSource")
}

func (g *GraphQLDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	g.walker, g.operation, g.definition, g.args = walker, operation, definition, args

	g.resolveDocument = &ast.Document{}
	g.rootFieldArgumentRefs = make([]int, len(resolverParameters))
	g.variableDefinitions = make([]int, len(resolverParameters))
	g.rootFieldRef = -1
	for i := 0; i < len(resolverParameters); i++ {
		g.resolveDocument.VariableValues = append(g.resolveDocument.VariableValues, ast.VariableValue{
			Name: g.resolveDocument.Input.AppendInputBytes(resolverParameters[i].name),
		})
		variableRef := len(g.resolveDocument.VariableValues) - 1
		variableValue := ast.Value{
			Kind: ast.ValueKindVariable,
			Ref:  variableRef,
		}
		g.resolveDocument.Arguments = append(g.resolveDocument.Arguments, ast.Argument{
			Name:  g.resolveDocument.Input.AppendInputBytes(resolverParameters[i].name),
			Value: variableValue,
		})
		g.rootFieldArgumentRefs[i] = len(g.resolveDocument.Arguments) - 1

		typeRef := g.importer.ImportType(resolverParameters[i].variableType,g.resolveDocument)

		g.resolveDocument.VariableDefinitions = append(g.resolveDocument.VariableDefinitions, ast.VariableDefinition{
			VariableValue: variableValue,
			Type:          typeRef,
		})
		g.variableDefinitions[i] = len(g.resolveDocument.VariableDefinitions) - 1
	}
}

func (g *GraphQLDataSourcePlanner) EnterInlineFragment(ref int) {
	if len(g.nodes) == 0 {
		return
	}
	current := g.nodes[len(g.nodes)-1]
	if current.Kind != ast.NodeKindSelectionSet {
		return
	}
	inlineFragmentType := g.resolveDocument.ImportType(g.operation.InlineFragments[ref].TypeCondition.Type, g.operation)
	g.resolveDocument.InlineFragments = append(g.resolveDocument.InlineFragments, ast.InlineFragment{
		TypeCondition: ast.TypeCondition{
			Type: inlineFragmentType,
		},
		SelectionSet: -1,
	})
	inlineFragmentRef := len(g.resolveDocument.InlineFragments) - 1
	g.resolveDocument.Selections = append(g.resolveDocument.Selections, ast.Selection{
		Kind: ast.SelectionKindInlineFragment,
		Ref:  inlineFragmentRef,
	})
	selectionRef := len(g.resolveDocument.Selections) - 1
	g.resolveDocument.SelectionSets[current.Ref].SelectionRefs = append(g.resolveDocument.SelectionSets[current.Ref].SelectionRefs, selectionRef)
	g.nodes = append(g.nodes, ast.Node{
		Kind: ast.NodeKindInlineFragment,
		Ref:  inlineFragmentRef,
	})
}

func (g *GraphQLDataSourcePlanner) LeaveInlineFragment(ref int) {
	g.nodes = g.nodes[:len(g.nodes)-1]
}

func (g *GraphQLDataSourcePlanner) EnterSelectionSet(ref int) {

	fieldOrInlineFragment := g.nodes[len(g.nodes)-1]

	set := ast.SelectionSet{}
	g.resolveDocument.SelectionSets = append(g.resolveDocument.SelectionSets, set)
	setRef := len(g.resolveDocument.SelectionSets) - 1

	switch fieldOrInlineFragment.Kind {
	case ast.NodeKindField:
		g.resolveDocument.Fields[fieldOrInlineFragment.Ref].HasSelections = true
		g.resolveDocument.Fields[fieldOrInlineFragment.Ref].SelectionSet = setRef
	case ast.NodeKindInlineFragment:
		g.resolveDocument.InlineFragments[fieldOrInlineFragment.Ref].HasSelections = true
		g.resolveDocument.InlineFragments[fieldOrInlineFragment.Ref].SelectionSet = setRef
	}

	g.nodes = append(g.nodes, ast.Node{
		Kind: ast.NodeKindSelectionSet,
		Ref:  setRef,
	})
}

func (g *GraphQLDataSourcePlanner) LeaveSelectionSet(ref int) {
	g.nodes = g.nodes[:len(g.nodes)-1]
}

func (g *GraphQLDataSourcePlanner) EnterField(ref int) {
	if g.rootFieldRef == -1 {
		g.rootFieldRef = ref

		var fieldName []byte
		objectName, ok := g.walker.FieldDefinitionDirectiveArgumentValueByName(ref, []byte("mapping"), []byte("pathSelector"))
		if ok && objectName.Kind == ast.ValueKindString {
			fieldName = g.definition.StringValueContentBytes(objectName.Ref)
		} else {
			fieldName = g.operation.FieldNameBytes(ref)
		}

		field := ast.Field{
			Name: g.resolveDocument.Input.AppendInputBytes(fieldName),
			Arguments: ast.ArgumentList{
				Refs: g.rootFieldArgumentRefs,
			},
			HasArguments: len(g.rootFieldArgumentRefs) != 0,
		}
		g.resolveDocument.Fields = append(g.resolveDocument.Fields, field)
		fieldRef := len(g.resolveDocument.Fields) - 1
		selection := ast.Selection{
			Kind: ast.SelectionKindField,
			Ref:  fieldRef,
		}
		g.resolveDocument.Selections = append(g.resolveDocument.Selections, selection)
		selectionRef := len(g.resolveDocument.Selections) - 1
		set := ast.SelectionSet{
			SelectionRefs: []int{selectionRef},
		}
		g.resolveDocument.SelectionSets = append(g.resolveDocument.SelectionSets, set)
		setRef := len(g.resolveDocument.SelectionSets) - 1
		operationDefinition := ast.OperationDefinition{
			Name:          g.resolveDocument.Input.AppendInputBytes([]byte("o")),
			OperationType: g.operation.OperationDefinitions[g.walker.Ancestors[0].Ref].OperationType,
			SelectionSet:  setRef,
			HasSelections: true,
			VariableDefinitions: ast.VariableDefinitionList{
				Refs: g.variableDefinitions,
			},
			HasVariableDefinitions: len(g.variableDefinitions) != 0,
		}
		g.resolveDocument.OperationDefinitions = append(g.resolveDocument.OperationDefinitions, operationDefinition)
		operationDefinitionRef := len(g.resolveDocument.OperationDefinitions) - 1
		g.resolveDocument.RootNodes = append(g.resolveDocument.RootNodes, ast.Node{
			Kind: ast.NodeKindOperationDefinition,
			Ref:  operationDefinitionRef,
		})

		g.nodes = append(g.nodes, ast.Node{
			Kind: ast.NodeKindOperationDefinition,
			Ref:  operationDefinitionRef,
		})
		g.nodes = append(g.nodes, ast.Node{
			Kind: ast.NodeKindSelectionSet,
			Ref:  setRef,
		})
		g.nodes = append(g.nodes, ast.Node{
			Kind: ast.NodeKindField,
			Ref:  fieldRef,
		})
	} else {
		field := ast.Field{
			Name: g.resolveDocument.Input.AppendInputBytes(g.operation.FieldNameBytes(ref)),
		}
		g.resolveDocument.Fields = append(g.resolveDocument.Fields, field)
		fieldRef := len(g.resolveDocument.Fields) - 1
		set := g.nodes[len(g.nodes)-1]
		selection := ast.Selection{
			Kind: ast.SelectionKindField,
			Ref:  fieldRef,
		}
		g.resolveDocument.Selections = append(g.resolveDocument.Selections, selection)
		selectionRef := len(g.resolveDocument.Selections) - 1
		g.resolveDocument.SelectionSets[set.Ref].SelectionRefs = append(g.resolveDocument.SelectionSets[set.Ref].SelectionRefs, selectionRef)
		g.nodes = append(g.nodes, ast.Node{
			Kind: ast.NodeKindField,
			Ref:  fieldRef,
		})
	}
}

func (g *GraphQLDataSourcePlanner) LeaveField(ref int) {

	if g.rootFieldRef == ref {

		buff := bytes.Buffer{}
		err := astprinter.Print(g.resolveDocument, nil, &buff)
		if err != nil {
			g.walker.StopWithInternalErr(err)
			return
		}
		arg := &StaticVariableArgument{
			Name:  literal.QUERY,
			Value: buff.Bytes(),
		}
		g.args = append([]Argument{arg}, g.args...)

		definition, exists := g.walker.FieldDefinition(ref)
		if !exists {
			return
		}
		directive, exists := g.definition.FieldDefinitionDirectiveByName(definition, []byte("GraphQLDataSource"))
		if !exists {
			return
		}
		value, exists := g.definition.DirectiveArgumentValueByName(directive, literal.URL)
		if !exists {
			return
		}
		variableValue := g.definition.StringValueContentBytes(value.Ref)
		arg = &StaticVariableArgument{
			Name:  literal.URL,
			Value: variableValue,
		}
		g.args = append([]Argument{arg}, g.args...)
		value, exists = g.definition.DirectiveArgumentValueByName(directive, literal.HOST)
		if !exists {
			return
		}
		variableValue = g.definition.StringValueContentBytes(value.Ref)
		arg = &StaticVariableArgument{
			Name:  literal.HOST,
			Value: variableValue,
		}
		g.args = append([]Argument{arg}, g.args...)
	}

	g.nodes = g.nodes[:len(g.nodes)-1]
}

func (g *GraphQLDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &GraphQLDataSource{
		log: g.log,
	}, g.args
}

type GraphQLDataSource struct {
	log log.Logger
}

func (g *GraphQLDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {

	hostArg := args.ByKey(literal.HOST)
	urlArg := args.ByKey(literal.URL)
	queryArg := args.ByKey(literal.QUERY)

	g.log.Debug("GraphQLDataSource.Resolve.args",
		log.Strings("resolvedArgs", args.Dump()),
	)

	if hostArg == nil || urlArg == nil || queryArg == nil {
		g.log.Error("GraphQLDataSource.args invalid")
		return CloseConnectionIfNotStream
	}

	url := string(hostArg) + string(urlArg)
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		url = "https://" + url
	}

	variables := map[string]interface{}{}
	for i := 0; i < len(args); i++ {
		key := args[i].Key
		switch {
		case bytes.Equal(key, literal.HOST):
		case bytes.Equal(key, literal.URL):
		case bytes.Equal(key, literal.QUERY):
		default:
			variables[string(key)] = string(args[i].Value)
		}
	}

	variablesJson,err := json.Marshal(variables)
	if err != nil {
		g.log.Error("GraphQLDataSource.json.Marshal(variables)",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	gqlRequest := GraphqlRequest{
		OperationName: "o",
		Variables:     variablesJson,
		Query:         string(queryArg),
	}

	gqlRequestData, err := json.MarshalIndent(gqlRequest, "", "  ")
	if err != nil {
		g.log.Error("GraphQLDataSource.json.MarshalIndent",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	g.log.Debug("GraphQLDataSource.request",
		log.String("url", url),
		log.ByteString("data", gqlRequestData),
	)

	client := http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 0 * time.Second,
		},
	}

	request, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(gqlRequestData))
	if err != nil {
		g.log.Error("GraphQLDataSource.http.NewRequest",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")

	res, err := client.Do(request)
	if err != nil {
		g.log.Error("GraphQLDataSource.client.Do",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		g.log.Error("GraphQLDataSource.ioutil.ReadAll",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	data = bytes.ReplaceAll(data, literal.BACKSLASH, nil)
	data, _, _, err = jsonparser.Get(data, "data")
	if err != nil {
		g.log.Error("GraphQLDataSource.jsonparser.Get",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}
	_, err = out.Write(data)
	if err != nil {
		g.log.Error("GraphQLDataSource.out.Write",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}
	return CloseConnectionIfNotStream
}
