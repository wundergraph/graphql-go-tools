package graphqldatasourcev2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	plan "github.com/jensneuse/graphql-go-tools/pkg/engine/planv2"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type Planner struct {
	visitor                    *plan.Visitor
	config                     Configuration
	id                         string
	upstreamOperation          *ast.Document
	upstreamVariables          []byte
	nodes                      []ast.Node
	variables                  resolve.Variables
	lastFieldEnclosingTypeName string
}

type Configuration struct {
	URL        string
	HttpMethod string
}

func (c *Configuration) ApplyDefaults() {
	if c.HttpMethod == "" {
		c.HttpMethod = "POST"
	}
}

func (p *Planner) Register(visitor *plan.Visitor, config json.RawMessage) error {
	p.visitor = visitor
	p.visitor.Walker.RegisterDocumentVisitor(p)
	p.visitor.Walker.RegisterFieldVisitor(p)
	p.visitor.Walker.RegisterOperationDefinitionVisitor(p)
	p.visitor.Walker.RegisterSelectionSetVisitor(p)
	p.visitor.Walker.RegisterEnterArgumentVisitor(p)

	err := json.Unmarshal(config, &p.config)
	if err != nil {
		return err
	}

	p.config.ApplyDefaults()

	return nil
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {

	input := httpclient.SetInputBodyWithPath(nil,p.upstreamVariables,"variables")
	input = httpclient.SetInputBodyWithPath(input, p.printOperation(), "query")
	input = httpclient.SetInputURL(input, []byte(p.config.URL))
	input = httpclient.SetInputMethod(input, []byte(p.config.HttpMethod))

	return plan.FetchConfiguration{
		Input:      string(input),
		DataSource: &Source{},
		Variables:  p.variables,
	}
}

func (p *Planner) EnterOperationDefinition(ref int) {
	definition := p.upstreamOperation.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
		OperationType: p.visitor.Operation.OperationDefinitions[ref].OperationType,
	})
	p.nodes = append(p.nodes, definition)
}

func (p *Planner) LeaveOperationDefinition(ref int) {
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) EnterSelectionSet(ref int) {
	parent := p.nodes[len(p.nodes)-1]
	set := p.upstreamOperation.AddSelectionSet()
	switch parent.Kind {
	case ast.NodeKindOperationDefinition:
		p.upstreamOperation.OperationDefinitions[parent.Ref].HasSelections = true
		p.upstreamOperation.OperationDefinitions[parent.Ref].SelectionSet = set.Ref
	case ast.NodeKindField:
		p.upstreamOperation.Fields[parent.Ref].HasSelections = true
		p.upstreamOperation.Fields[parent.Ref].SelectionSet = set.Ref
	case ast.NodeKindInlineFragment:
		p.upstreamOperation.InlineFragments[parent.Ref].HasSelections = true
		p.upstreamOperation.InlineFragments[parent.Ref].SelectionSet = set.Ref
	}
	p.nodes = append(p.nodes, set)
}

func (p *Planner) LeaveSelectionSet(ref int) {
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) EnterField(ref int) {
	p.addField(ref)
	p.lastFieldEnclosingTypeName = p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)
}

func (p *Planner) LeaveField(ref int) {
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) EnterArgument(ref int) {
	if p.visitor.Walker.Ancestors[len(p.visitor.Walker.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}
	if p.nodes[len(p.nodes)-1].Kind != ast.NodeKindField {
		return
	}
	downstreamFieldRef := p.visitor.Walker.Ancestors[len(p.visitor.Walker.Ancestors)-1].Ref
	upstreamFieldRef := p.nodes[len(p.nodes)-1].Ref
	typeName := p.lastFieldEnclosingTypeName
	fieldName := p.visitor.Operation.FieldNameString(downstreamFieldRef)
	argName := p.visitor.Operation.ArgumentNameString(ref)
	fieldConfiguration := p.visitor.Config.Fields.ForTypeField(typeName, fieldName)
	if fieldConfiguration == nil {
		return
	}
	argumentConfig := fieldConfiguration.Arguments.ForName(argName)
	if argumentConfig == nil {
		return
	}
	p.configureArgument(upstreamFieldRef, downstreamFieldRef, *argumentConfig)
}

func (p *Planner) EnterDocument(operation, definition *ast.Document) {
	if p.upstreamOperation == nil {
		p.upstreamOperation = ast.NewDocument()
	} else {
		p.upstreamOperation.Reset()
	}
	p.nodes = p.nodes[:0]
	p.upstreamVariables = nil
	p.variables = p.variables[:0]
}

func (p *Planner) LeaveDocument(operation, definition *ast.Document) {

}

func (p *Planner) configureArgument(upstreamFieldRef, downstreamFieldRef int, config plan.ArgumentConfiguration) {
	switch config.SourceType {
	case plan.FieldArgumentSource:
		p.configureFieldArgumentSource(upstreamFieldRef, downstreamFieldRef, config.Name)
	case plan.ObjectFieldSource:
		p.configureObjectFieldSource(upstreamFieldRef, downstreamFieldRef, config.SourcePath)
	}
}

func (p *Planner) configureFieldArgumentSource(upstreamFieldRef, downstreamFieldRef int, argumentName string) {
	fieldArgument, ok := p.visitor.Operation.FieldArgument(downstreamFieldRef, []byte(argumentName))
	if !ok {
		return
	}
	value := p.visitor.Operation.ArgumentValue(fieldArgument)
	if value.Kind != ast.ValueKindVariable {
		p.applyInlineFieldArgument(upstreamFieldRef, downstreamFieldRef, argumentName)
		return
	}
	variableName := p.visitor.Operation.VariableValueNameBytes(value.Ref)
	variableNameStr := p.visitor.Operation.VariableValueNameString(value.Ref)

	variableDefinition, ok := p.visitor.Operation.VariableDefinitionByNameAndOperation(p.visitor.Walker.Ancestors[0].Ref, variableName)
	if !ok {
		return
	}

	variableDefinitionType := p.visitor.Operation.VariableDefinitions[variableDefinition].Type
	wrapValueInQuotes := p.visitor.Operation.TypeValueNeedsQuotes(variableDefinitionType, p.visitor.Definition)

	contextVariableName, exists := p.variables.AddVariable(&resolve.ContextVariable{Path: append([]string{variableNameStr})}, wrapValueInQuotes)
	variableValueRef, argRef := p.upstreamOperation.AddVariableValueArgument([]byte(argumentName), variableName) // add the argument to the field, but don't redefine it
	p.upstreamOperation.AddArgumentToField(upstreamFieldRef, argRef)

	if exists { // if the variable exists we don't have to put it onto the variables declaration again, skip
		return
	}

	for _, i := range p.visitor.Operation.OperationDefinitions[p.visitor.Walker.Ancestors[0].Ref].VariableDefinitions.Refs {
		ref := p.visitor.Operation.VariableDefinitions[i].VariableValue.Ref
		if !p.visitor.Operation.VariableValueNameBytes(ref).Equals(variableName) {
			continue
		}
		importedType := p.visitor.Importer.ImportType(p.visitor.Operation.VariableDefinitions[i].Type, p.visitor.Operation, p.upstreamOperation)
		p.upstreamOperation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, variableValueRef, importedType)
	}

	p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, variableNameStr, []byte(contextVariableName))
}

func (p *Planner) applyInlineFieldArgument(upstreamField, downstreamField int, argumentName string) {
	fieldArgument, ok := p.visitor.Operation.FieldArgument(downstreamField, []byte(argumentName))
	if !ok {
		return
	}
	value := p.visitor.Operation.ArgumentValue(fieldArgument)
	importedValue := p.visitor.Importer.ImportValue(value, p.visitor.Operation, p.upstreamOperation)
	argRef := p.upstreamOperation.AddArgument(ast.Argument{
		Name:  p.upstreamOperation.Input.AppendInputString(argumentName),
		Value: importedValue,
	})
	p.upstreamOperation.AddArgumentToField(upstreamField, argRef)
	// p.addVariableDefinitionsRecursively(value, )
}

func (p *Planner) addVariableDefinitionsRecursively(value ast.Value, argumentName string) {
	/*switch value.Kind {
	case ast.ValueKindObject:
		for _, i := range p.v.Operation.ObjectValues[value.Ref].Refs {
			p.addVariableDefinitionsRecursively(p.v.Operation.ObjectFields[i].Value, arg)
		}
		return
	case ast.ValueKindList:
		for _, i := range p.v.Operation.ListValues[value.Ref].Refs {
			p.addVariableDefinitionsRecursively(p.v.Operation.Values[i], arg)
		}
		return
	case ast.ValueKindVariable:
		// continue after switch
	default:
		return
	}

	variableName := p.v.Operation.VariableValueNameBytes(value.Ref)
	variableNameStr := p.v.Operation.VariableValueNameString(value.Ref)
	variableDefinition, exists := p.v.Operation.VariableDefinitionByNameAndOperation(p.v.Ancestors[0].Ref, variableName)
	if !exists {
		return
	}
	importedVariableDefinition := p.v.Importer.ImportVariableDefinition(variableDefinition, p.v.Operation, p.operation)
	p.operation.AddImportedVariableDefinitionToOperationDefinition(p.nodes[0].Ref, importedVariableDefinition)

	variableDefinitionType := p.v.Operation.VariableDefinitions[variableDefinition].Type
	wrapValueInQuotes := p.v.Operation.TypeValueNeedsQuotes(variableDefinitionType, p.v.Definition)

	contextVariableName, variableExists := p.fetch.Variables.AddVariable(&resolve.ContextVariable{Path: append(arg.SourcePath, variableNameStr)}, wrapValueInQuotes)
	if variableExists {
		return
	}
	p.variables, _ = sjson.SetRawBytes(p.variables, variableNameStr, []byte(contextVariableName))*/
}

func (p *Planner) configureObjectFieldSource(upstreamFieldRef, downstreamFieldRef int, sourcePath []string) {

}

func (p *Planner) printOperation() []byte {

	buf := &bytes.Buffer{}

	/*if p.isFederation && p.isNestedRequest {
		_, _ = buf.Write(federationQueryHeader)
		_, _ = buf.Write(p.federationRootTypeName)
		_, _ = buf.Write(literal.SPACE)
	}*/

	err := astprinter.Print(p.upstreamOperation, nil, buf)
	if err != nil {
		return nil
	}

	/*if p.isFederation && p.isNestedRequest {
		_, _ = buf.Write(federationQueryTrailer)
	}*/

	rawQuery := buf.Bytes()

	baseSchema, err := astprinter.PrintString(p.visitor.Definition, nil)
	if err != nil {
		return nil
	}

	/*federationSchema, err := federation.BuildFederationSchema(baseSchema, string(p.federationSDL))
	if err != nil {
		return nil, err
	}*/

	federationSchema := baseSchema

	operation := ast.NewDocument()
	definition := ast.NewDocument()
	report := &operationreport.Report{}
	parser := astparser.NewParser()

	definition.Input.ResetInputString(federationSchema)
	operation.Input.ResetInputBytes(rawQuery)

	parser.Parse(operation, report)
	if report.HasErrors() {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("printOperation: parse operation failed"))
		return nil
	}

	parser.Parse(definition, report)
	if report.HasErrors() {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("printOperation: parse definition failed"))
		return nil
	}

	normalizer := astnormalization.NewNormalizer(true, true)
	normalizer.NormalizeOperation(operation, definition, report)

	if report.HasErrors() {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("normalization failed"))
		return nil
	}

	buf.Reset()

	err = astprinter.Print(operation, p.visitor.Definition, buf)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("normalization failed"))
		return nil
	}
	return buf.Bytes()
}

func (p *Planner) addField(ref int) {

	fieldName := p.visitor.Operation.FieldNameString(ref)

	alias := ast.Alias{
		IsDefined: p.visitor.Operation.FieldAliasIsDefined(ref),
	}

	if alias.IsDefined {
		aliasBytes := p.visitor.Operation.FieldAliasBytes(ref)
		alias.Name = p.upstreamOperation.Input.AppendInputBytes(aliasBytes)
	}

	typeName := p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)
	for i := range p.visitor.Config.Fields {
		if p.visitor.Config.Fields[i].TypeName == typeName &&
			p.visitor.Config.Fields[i].FieldName == fieldName &&
			len(p.visitor.Config.Fields[i].Path) == 1 {
			fieldName = p.visitor.Config.Fields[i].Path[0]
			break
		}
	}

	field := p.upstreamOperation.AddField(ast.Field{
		Name:  p.upstreamOperation.Input.AppendInputString(fieldName),
		Alias: alias,
	})

	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  field.Ref,
	}

	p.upstreamOperation.AddSelection(p.nodes[len(p.nodes)-1].Ref, selection)
	p.nodes = append(p.nodes, field)
}

type Factory struct {
	id int
}

func (f *Factory) Planner() plan.DataSourcePlanner {
	f.id++
	return &Planner{
		id: strconv.Itoa(f.id),
	}
}

type Source struct {
}

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	return
}

func (s *Source) UniqueIdentifier() []byte {
	return nil
}
