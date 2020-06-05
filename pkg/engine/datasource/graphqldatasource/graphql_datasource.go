package graphqldatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"log"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/pool"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

type Planner struct {
	client              datasource.Client
	v                   *plan.Visitor
	fetch               *resolve.SingleFetch
	printer             astprinter.Printer
	operation           *ast.Document
	nodes               []ast.Node
	buf                 *bytes.Buffer
	operationNormalizer *astnormalization.OperationNormalizer
	URL                 []byte
	variables           []byte
	bufferID            int
	config              *plan.DataSourceConfiguration
}

func NewPlanner(client datasource.Client) *Planner {
	return &Planner{
		client: client,
	}
}

func (p *Planner) clientOrDefault() datasource.Client {
	if p.client != nil {
		return p.client
	}
	return datasource.NewFastHttpClient(datasource.DefaultFastHttpClient)
}

func (p *Planner) Register(visitor *plan.Visitor) {
	p.v = visitor
	visitor.RegisterFieldVisitor(p)
	visitor.RegisterDocumentVisitor(p)
	visitor.RegisterSelectionSetVisitor(p)
}

func (p *Planner) EnterDocument(_, _ *ast.Document) {
	if p.operation == nil {
		p.operation = ast.NewDocument()
	} else {
		p.operation.Reset()
	}
	if p.buf == nil {
		p.buf = &bytes.Buffer{}
	} else {
		p.buf.Reset()
	}
	if p.operationNormalizer == nil {
		p.operationNormalizer = astnormalization.NewNormalizer(true)
	}
	p.nodes = p.nodes[:0]
	p.URL = nil
	p.variables = nil
}

func (p *Planner) EnterField(ref int) {

	var (
		isRootField bool
		config      *plan.DataSourceConfiguration
	)

	isRootField, config = p.v.IsRootField(ref)

	if isRootField && config != nil {
		p.config = config
		if p.nodes == nil { // Setup Fetch and root (operation definition)
			p.URL = config.Attributes.ValueForKey("url")

			p.bufferID = p.v.NextBufferID()
			p.fetch = &resolve.SingleFetch{
				BufferId: p.bufferID,
			}
			p.v.SetCurrentObjectFetch(p.fetch, config)
			if len(p.operation.RootNodes) == 0 {
				set := p.operation.AddSelectionSet()
				definition := p.operation.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
					OperationType: p.v.Operation.OperationDefinitions[p.v.Ancestors[0].Ref].OperationType,
					SelectionSet:  set.Ref,
					HasSelections: true,
				})
				p.nodes = append(p.nodes, definition, set)
			}
		}
		// subsequent root fields get their own fieldset
		// we need to set the buffer for all fields
		p.v.SetBufferIDForCurrentFieldSet(p.bufferID)
	}
	field := p.addField(ref)
	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  field.Ref,
	}
	upstreamSelectionSet := p.nodes[len(p.nodes)-1].Ref
	p.operation.AddSelection(upstreamSelectionSet, selection)
	p.nodes = append(p.nodes, field)
	if config == nil {
		return
	}
	if arguments := config.Attributes.ValueForKey("arguments"); arguments != nil {
		p.configureFieldArguments(field.Ref, ref, arguments)
	}
}

func (p *Planner) handleFieldDependencies(downstreamField, upstreamSelectionSet int) {
	typeName := p.v.EnclosingTypeDefinition.Name(p.v.Definition)
	fieldName := p.v.Operation.FieldNameString(downstreamField)
	for i := range p.v.Config.FieldDependencies {
		if p.v.Config.FieldDependencies[i].TypeName != typeName {
			continue
		}
		if p.v.Config.FieldDependencies[i].FieldName != fieldName {
			continue
		}
		for j := range p.v.Config.FieldDependencies[i].RequiresFields {
			requiredField := p.v.Config.FieldDependencies[i].RequiresFields[j]
			p.addFieldDependency(requiredField,upstreamSelectionSet)
		}
	}
}

func (p *Planner) addFieldDependency(fieldName string, selectionSet int) {

	if p.operation.SelectionSetHasFieldSelectionWithNameOrAliasString(selectionSet, fieldName) {
		return
	}

	field := ast.Field{
		Alias: ast.Alias{
			IsDefined: true,
			Name:      p.operation.Input.AppendInputString(fieldName),
		},
		Name: p.operation.Input.AppendInputString(fieldName),
	}
	addedField := p.operation.AddField(field)
	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  addedField.Ref,
	}
	p.operation.AddSelection(selectionSet, selection)
}

func (p *Planner) addField(ref int) ast.Node {

	fieldName := p.v.Operation.FieldNameString(ref)

	alias := ast.Alias{
		IsDefined: p.v.Operation.FieldAliasIsDefined(ref),
	}

	if alias.IsDefined {
		aliasBytes := p.v.Operation.FieldAliasBytes(ref)
		alias.Name = p.operation.Input.AppendInputBytes(aliasBytes)
		p.v.SetFieldPathOverride(ref, func(path []string) []string {
			if len(path) != 0 && path[0] == fieldName {
				path[0] = string(aliasBytes)
				return path
			}
			return path
		})
	}

	typeName := p.v.EnclosingTypeDefinition.Name(p.v.Definition)
	for i := range p.v.Config.FieldMappings {
		if p.v.Config.FieldMappings[i].TypeName == typeName &&
			p.v.Config.FieldMappings[i].FieldName == fieldName &&
			len(p.v.Config.FieldMappings[i].Path) == 1 {
			fieldName = p.v.Config.FieldMappings[i].Path[0]
			break
		}
	}

	return p.operation.AddField(ast.Field{
		Name:  p.operation.Input.AppendInputString(fieldName),
		Alias: alias,
	})
}

func (p *Planner) configureFieldArguments(upstreamField, downstreamField int, arguments []byte) {
	var config ArgumentsConfig
	err := json.Unmarshal(arguments, &config)
	if err != nil {
		log.Fatal(err)
		return
	}
	fieldName := p.v.Operation.FieldNameString(downstreamField)
	for i := range config.Fields {
		if config.Fields[i].FieldName != fieldName {
			continue
		}
		for j := range config.Fields[i].Arguments {
			p.applyFieldArgument(upstreamField, downstreamField, config.Fields[i].Arguments[j])
		}
	}
}

func (p *Planner) applyFieldArgument(upstreamField, downstreamField int, arg Argument) {
	switch arg.Source {
	case FieldArgument:
		if fieldArgument, ok := p.v.Operation.FieldArgument(downstreamField, arg.Name); ok {
			value := p.v.Operation.ArgumentValue(fieldArgument)
			if value.Kind != ast.ValueKindVariable {
				return
			}
			variableName := p.v.Operation.VariableValueNameBytes(value.Ref)
			variableNameStr := p.v.Operation.VariableValueNameString(value.Ref)

			variableDefinition, ok := p.v.Operation.VariableDefinitionByNameAndOperation(p.v.Ancestors[0].Ref, variableName)
			if !ok {
				return
			}

			variableDefinitionType := p.v.Operation.VariableDefinitions[variableDefinition].Type
			wrapValueInQuotes := p.v.Operation.TypeValueNeedsQuotes(variableDefinitionType)

			contextVariableName, exists := p.fetch.Variables.AddVariable(&resolve.ContextVariable{Path: append([]string{variableNameStr}, arg.SourcePath...)}, wrapValueInQuotes)
			variableValueRef, argRef := p.operation.AddVariableValueArgument(arg.Name, variableName) // add the argument to the field, but don't redefine it
			p.operation.AddArgumentToField(upstreamField, argRef)

			if exists { // if the variable exists we don't have to put it onto the variables declaration again, skip
				return
			}

			for _, i := range p.v.Operation.OperationDefinitions[p.v.Ancestors[0].Ref].VariableDefinitions.Refs {
				ref := p.v.Operation.VariableDefinitions[i].VariableValue.Ref
				if !p.v.Operation.VariableValueNameBytes(ref).Equals(variableName) {
					continue
				}
				importedType := p.v.Importer.ImportType(p.v.Operation.VariableDefinitions[i].Type, p.v.Operation, p.operation)
				p.operation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, variableValueRef, importedType)
			}

			p.variables, _ = sjson.SetRawBytes(p.variables, variableNameStr, contextVariableName)
		}
	case ObjectField:
		if len(arg.SourcePath) < 1 {
			return
		}

		enclosingTypeName := p.v.EnclosingTypeDefinition.Name(p.v.Definition)
		fieldName := p.v.Operation.FieldNameString(downstreamField)

		for i := range p.v.Config.FieldMappings {
			if p.v.Config.FieldMappings[i].TypeName == enclosingTypeName &&
				p.v.Config.FieldMappings[i].FieldName == fieldName &&
				len(p.v.Config.FieldMappings[i].Path) == 1 {
				fieldName = p.v.Config.FieldMappings[i].Path[0]
			}
		}

		queryTypeDefinition := p.v.Definition.Index.Nodes[xxhash.Sum64(p.v.Definition.Index.QueryTypeName)]
		argumentDefinition := p.v.Definition.NodeFieldDefinitionArgumentDefinitionByName(queryTypeDefinition, []byte(fieldName), arg.Name)
		if argumentDefinition == -1 {
			return
		}

		argumentType := p.v.Definition.InputValueDefinitionType(argumentDefinition)
		variableName := p.operation.GenerateUnusedVariableDefinitionName(p.nodes[0].Ref)
		variableValue, argument := p.operation.AddVariableValueArgument(arg.Name, variableName)
		p.operation.AddArgumentToField(upstreamField, argument)
		importedType := p.v.Importer.ImportType(argumentType, p.v.Definition, p.operation)
		p.operation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, variableValue, importedType)
		wrapVariableInQuotes := p.v.Definition.TypeValueNeedsQuotes(argumentType)

		objectVariableName, exists := p.fetch.Variables.AddVariable(&resolve.ObjectVariable{Path: arg.SourcePath}, wrapVariableInQuotes)
		if !exists {
			p.variables, _ = sjson.SetRawBytes(p.variables, string(variableName), objectVariableName)
		}
	}
}

func (p *Planner) LeaveField(ref int) {
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) EnterSelectionSet(ref int) {
	parent := p.nodes[len(p.nodes)-1]
	set := p.operation.AddSelectionSet()
	switch parent.Kind {
	case ast.NodeKindField:
		p.operation.Fields[parent.Ref].HasSelections = true
		p.operation.Fields[parent.Ref].SelectionSet = set.Ref
	case ast.NodeKindInlineFragment:
		p.operation.InlineFragments[parent.Ref].HasSelections = true
		p.operation.InlineFragments[parent.Ref].SelectionSet = set.Ref
	}
	p.nodes = append(p.nodes, set)
}

func (p *Planner) LeaveSelectionSet(ref int) {

	upstreamSelectionSet := p.nodes[len(p.nodes) -1].Ref

	for _,i := range p.v.Operation.SelectionSets[ref].SelectionRefs {
		if p.v.Operation.Selections[i].Kind != ast.SelectionKindField {
			continue
		}
		downstreamField := p.v.Operation.Selections[i].Ref
		p.handleFieldDependencies(downstreamField,upstreamSelectionSet)
	}

	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) LeaveDocument(operation, definition *ast.Document) {
	p.operationNormalizer.NormalizeOperation(p.operation, definition, p.v.Report)
	buf := &bytes.Buffer{}
	err := p.printer.Print(p.operation, nil, buf)
	if err != nil {
		return
	}
	if p.variables != nil {
		p.fetch.Input, _ = sjson.SetRawBytes(p.fetch.Input, "body.variables", p.variables)
	}
	p.fetch.Input, _ = sjson.SetRawBytes(p.fetch.Input, "body.query", append([]byte{'"'}, append(buf.Bytes(), '"')...))
	p.fetch.Input, _ = sjson.SetRawBytes(p.fetch.Input, "url", append([]byte{'"'}, append(p.URL, '"')...))
	source := DefaultSource()
	source.client = p.clientOrDefault()
	p.fetch.DataSource = source
	p.fetch.DisallowSingleFlight = p.operation.OperationDefinitions[p.nodes[0].Ref].OperationType != ast.OperationTypeQuery
}

type Source struct {
	client datasource.Client
}

func DefaultSource() *Source {
	return &Source{
		client: datasource.NewFastHttpClient(datasource.DefaultFastHttpClient),
	}
}

var (
	uniqueIdentifier = []byte("graphql")
)

func (_ *Source) UniqueIdentifier() []byte {
	return uniqueIdentifier
}

var (
	method = []byte("POST")
	inputPaths       = [][]string{
		{datasource.URL},
		{datasource.METHOD},
		{datasource.BODY},
		{datasource.HEADERS},
		{datasource.QUERYPARAMS},
	}
	responsePaths = [][]string{
		{"errors"},
		{"data"},
	}
)

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	var (
		url, body,headers  []byte
	)
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			url = bytes
		case 1:
			body = bytes
		case 2:
			headers = bytes
		}
	}, inputPaths...)

	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)

	err = s.client.Do(ctx, url, nil, method, headers, body, buf)
	if err != nil {
		return
	}

	responseData := buf.Bytes()

	jsonparser.EachKey(responseData, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			bufPair.Errors.Write(bytes)
		case 1:
			bufPair.Data.Write(bytes)
		}
	}, responsePaths...)

	return
}

func ArgumentsConfigJSON(config ArgumentsConfig) []byte {
	out, _ := json.Marshal(config)
	return out
}

type ArgumentsConfig struct {
	Fields []FieldConfig
}

type FieldConfig struct {
	FieldName string
	Arguments []Argument
}

type Argument struct {
	Name       []byte
	Source     ArgumentSource
	SourcePath []string
}

type ArgumentSource string

const (
	ObjectField   ArgumentSource = "object_field"
	FieldArgument ArgumentSource = "field_argument"
)
