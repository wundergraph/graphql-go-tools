package graphql_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/federation"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

const (
	UniqueIdentifier = "graphql"
)

type Planner struct {
	visitor                    *plan.Visitor
	config                     Configuration
	upstreamOperation          *ast.Document
	upstreamVariables          []byte
	nodes                      []ast.Node
	variables                  resolve.Variables
	lastFieldEnclosingTypeName string
	disallowSingleFlight       bool
	hasFederationRoot          bool
	extractEntities            bool
	client                     httpclient.Client
	isNested                   bool   // isNested - flags that datasource is nested e.g. field with datasource is not on a query type
	rootTypeName               string // rootTypeName - holds name of top level type
	rootFieldName              string // rootFieldName - holds name of root type field
	rootFieldRef               int    // rootFieldRef - holds ref of root type field
	batchFactory               resolve.DataSourceBatchFactory
}

func (p *Planner) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {

	// If there's no alias but the downstream Query re-uses the same path on different root fields,
	// we rewrite the downstream Query using an alias so that we can have an aliased Query to the upstream
	// while keeping a non aliased Query to the downstream but with a path rewrite on an existing root field.

	fieldName := p.visitor.Operation.FieldNameUnsafeString(downstreamFieldRef)

	if p.visitor.Operation.FieldAliasIsDefined(downstreamFieldRef) {
		return "", false
	}

	typeName := p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)
	for i := range p.visitor.Config.Fields {
		if p.visitor.Config.Fields[i].TypeName == typeName &&
			p.visitor.Config.Fields[i].FieldName == fieldName &&
			len(p.visitor.Config.Fields[i].Path) == 1 {

			if p.visitor.Config.Fields[i].Path[0] != fieldName {
				aliasBytes := p.visitor.Operation.FieldNameBytes(downstreamFieldRef)
				return string(aliasBytes), true
			}
			break
		}
	}
	return "", false
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      true,
		OverrideFieldPathFromAlias: true,
	}
}

type Configuration struct {
	Fetch        FetchConfiguration
	Subscription SubscriptionConfiguration
	Federation   FederationConfiguration
}

func ConfigJson(config Configuration) json.RawMessage {
	out, _ := json.Marshal(config)
	return out
}

type FederationConfiguration struct {
	Enabled    bool
	ServiceSDL string
}

type SubscriptionConfiguration struct {
	URL string
}

type FetchConfiguration struct {
	URL    string
	Method string
	Header http.Header
}

func (c *Configuration) ApplyDefaults() {
	if c.Fetch.Method == "" {
		c.Fetch.Method = "POST"
	}
}

func (p *Planner) Register(visitor *plan.Visitor, config json.RawMessage, isNested bool) error {
	p.visitor = visitor
	p.visitor.Walker.RegisterDocumentVisitor(p)
	p.visitor.Walker.RegisterFieldVisitor(p)
	p.visitor.Walker.RegisterOperationDefinitionVisitor(p)
	p.visitor.Walker.RegisterSelectionSetVisitor(p)
	p.visitor.Walker.RegisterEnterArgumentVisitor(p)
	p.visitor.Walker.RegisterInlineFragmentVisitor(p)

	err := json.Unmarshal(config, &p.config)
	if err != nil {
		return err
	}

	p.config.ApplyDefaults()
	p.isNested = isNested

	return nil
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {

	var input []byte
	if p.extractEntities {
		input, _ = sjson.SetRawBytes(input, "extract_entities", []byte("true"))
	}
	input = httpclient.SetInputBodyWithPath(input, p.upstreamVariables, "variables")
	input = httpclient.SetInputBodyWithPath(input, p.printOperation(), "query")

	header, err := json.Marshal(p.config.Fetch.Header)
	if err == nil && len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = httpclient.SetInputHeader(input, header)
	}

	input = httpclient.SetInputURL(input, []byte(p.config.Fetch.URL))
	input = httpclient.SetInputMethod(input, []byte(p.config.Fetch.Method))

	var batchConfig plan.BatchConfig
	// Allow batch query for fetching entities.
	if p.extractEntities && p.batchFactory != nil {
		batchConfig = plan.BatchConfig{
			AllowBatch:   p.extractEntities, // Allow batch query for fetching entities.
			BatchFactory: p.batchFactory,
		}
	}

	return plan.FetchConfiguration{
		Input: string(input),
		DataSource: &Source{
			client: p.client,
		},
		Variables:            p.variables,
		DisallowSingleFlight: p.disallowSingleFlight,
		BatchConfig:          batchConfig,
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {

	input := httpclient.SetInputBodyWithPath(nil, p.upstreamVariables, "variables")
	input = httpclient.SetInputBodyWithPath(input, p.printOperation(), "query")
	input = httpclient.SetInputURL(input, []byte(p.config.Subscription.URL))

	header, err := json.Marshal(p.config.Fetch.Header)
	if err == nil && len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = httpclient.SetInputHeader(input, header)
	}

	return plan.SubscriptionConfiguration{
		Input:                 string(input),
		SubscriptionManagerID: "graphql_websocket_subscription",
		Variables:             p.variables,
	}
}

func (p *Planner) EnterOperationDefinition(ref int) {
	operationType := p.visitor.Operation.OperationDefinitions[ref].OperationType
	if p.isNested {
		operationType = ast.OperationTypeQuery
	}
	definition := p.upstreamOperation.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
		OperationType: operationType,
	})
	p.disallowSingleFlight = operationType == ast.OperationTypeMutation
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
	for _, selectionRef := range p.visitor.Operation.SelectionSets[ref].SelectionRefs {
		if p.visitor.Operation.Selections[selectionRef].Kind == ast.SelectionKindField {
			if p.visitor.Operation.FieldNameUnsafeString(p.visitor.Operation.Selections[selectionRef].Ref) == "__typename" {
				field := p.upstreamOperation.AddField(ast.Field{
					Name: p.upstreamOperation.Input.AppendInputString("__typename"),
				})
				p.upstreamOperation.AddSelection(set.Ref, ast.Selection{
					Ref:  field.Ref,
					Kind: ast.SelectionKindField,
				})
			}
		}
	}
}

func (p *Planner) LeaveSelectionSet(ref int) {
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) EnterInlineFragment(ref int) {

	typeCondition := p.visitor.Operation.InlineFragmentTypeConditionName(ref)
	if typeCondition == nil {
		return
	}

	inlineFragment := p.upstreamOperation.AddInlineFragment(ast.InlineFragment{
		TypeCondition: ast.TypeCondition{
			Type: p.upstreamOperation.AddNamedType(typeCondition),
		},
	})

	selection := ast.Selection{
		Kind: ast.SelectionKindInlineFragment,
		Ref:  inlineFragment,
	}

	// add __typename field to selection set which contains typeCondition
	// so that the resolver can distinguish between the response types
	typeNameField := p.upstreamOperation.AddField(ast.Field{
		Name: p.upstreamOperation.Input.AppendInputBytes([]byte("__typename")),
	})
	p.upstreamOperation.AddSelection(p.nodes[len(p.nodes)-1].Ref, ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  typeNameField.Ref,
	})

	p.upstreamOperation.AddSelection(p.nodes[len(p.nodes)-1].Ref, selection)
	p.nodes = append(p.nodes, ast.Node{Kind: ast.NodeKindInlineFragment, Ref: inlineFragment})
}

func (p *Planner) LeaveInlineFragment(ref int) {
	if p.nodes[len(p.nodes)-1].Kind != ast.NodeKindInlineFragment {
		return
	}
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) EnterField(ref int) {

	fieldName := p.visitor.Operation.FieldNameString(ref)

	// store root field name and ref
	if p.rootFieldName == "" {
		p.rootFieldName = fieldName
		p.rootFieldRef = ref
	}
	// store root type name
	if p.rootTypeName == "" {
		p.rootTypeName = p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)
	}

	p.lastFieldEnclosingTypeName = p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)

	p.handleFederation(ref)
	p.addField(ref)

	upstreamFieldRef := p.nodes[len(p.nodes)-1].Ref
	typeName := p.lastFieldEnclosingTypeName

	fieldConfiguration := p.visitor.Config.Fields.ForTypeField(typeName, fieldName)
	if fieldConfiguration == nil {
		return
	}
	for i := range fieldConfiguration.Arguments {
		argumentConfiguration := fieldConfiguration.Arguments[i]
		p.configureArgument(upstreamFieldRef, ref, *fieldConfiguration, argumentConfiguration)
	}
}

func (p *Planner) LeaveField(ref int) {
	// fmt.Printf("Planner::%s::%s::LeaveField::%s::%d\n", p.id, p.visitor.Walker.Path.DotDelimitedString(), p.visitor.Operation.FieldNameUnsafeString(ref), ref)
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) EnterArgument(ref int) {

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
	p.disallowSingleFlight = false
	p.hasFederationRoot = false
	p.extractEntities = false

	// reset information about root type
	p.rootTypeName = ""
	p.rootFieldName = ""
	p.rootFieldRef = -1
}

func (p *Planner) LeaveDocument(operation, definition *ast.Document) {

}

func (p *Planner) handleFederation(fieldRef int) {
	if !p.config.Federation.Enabled || // federation must be enabled
		p.hasFederationRoot || // should not already have federation root field
		!p.isNestedRequest() { // must be nested, otherwise it's a regular query
		return
	}
	p.hasFederationRoot = true
	// query($representations: [_Any!]!){_entities(representations: $representations){... on Product
	p.addRepresentationsVariableDefinition() // $representations: [_Any!]!
	p.addEntitiesSelectionSet()              // {_entities(representations: $representations)
	p.addOneTypeInlineFragment()             // ... on Product
	p.addRepresentationsVariable()           // "variables\":{\"representations\":[{\"upc\":\"$$0$$\",\"__typename\":\"Product\"}]}}
}

func (p *Planner) addRepresentationsVariable() {
	// "variables\":{\"representations\":[{\"upc\":\"$$0$$\",\"__typename\":\"Product\"}]}}
	parser := astparser.NewParser()
	doc := ast.NewDocument()
	doc.Input.ResetInputString(p.config.Federation.ServiceSDL)
	report := &operationreport.Report{}
	parser.Parse(doc, report)
	if report.HasErrors() {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("GraphQL Planner: failed parsing Federation SDL"))
		return
	}
	directive := -1
	for i := range doc.ObjectTypeExtensions {
		if p.lastFieldEnclosingTypeName == doc.ObjectTypeExtensionNameString(i) {
			for _, j := range doc.ObjectTypeExtensions[i].Directives.Refs {
				if doc.DirectiveNameString(j) == "key" {
					directive = j
					break
				}
			}
			break
		}
	}
	for i := range doc.ObjectTypeDefinitions {
		if p.lastFieldEnclosingTypeName == doc.ObjectTypeDefinitionNameString(i) {
			for _, j := range doc.ObjectTypeDefinitions[i].Directives.Refs {
				if doc.DirectiveNameString(j) == "key" {
					directive = j
					break
				}
			}
			break
		}
	}
	if directive == -1 {
		return
	}
	value, exists := doc.DirectiveArgumentValueByName(directive, []byte("fields"))
	if !exists {
		return
	}
	if value.Kind != ast.ValueKindString {
		return
	}
	fieldsStr := doc.StringValueContentString(value.Ref)
	fields := strings.Split(fieldsStr, " ")
	representationsJson, _ := sjson.SetRawBytes(nil, "__typename", []byte("\""+p.lastFieldEnclosingTypeName+"\""))
	for i := range fields {
		variable, exists := p.variables.AddVariable(&resolve.ObjectVariable{
			Path: []string{fields[i]},
		}, true)
		if exists {
			continue
		}
		representationsJson, _ = sjson.SetRawBytes(representationsJson, fields[i], []byte(variable))
	}
	representationsJson = append([]byte("["), append(representationsJson, []byte("]")...)...)
	p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, "representations", representationsJson)
	p.extractEntities = true
}

func (p *Planner) addOneTypeInlineFragment() {
	selectionSet := p.upstreamOperation.AddSelectionSet()
	typeRef := p.upstreamOperation.AddNamedType([]byte(p.lastFieldEnclosingTypeName))
	inlineFragment := p.upstreamOperation.AddInlineFragment(ast.InlineFragment{
		HasSelections: true,
		SelectionSet:  selectionSet.Ref,
		TypeCondition: ast.TypeCondition{
			Type: typeRef,
		},
	})
	p.upstreamOperation.AddSelection(p.nodes[len(p.nodes)-1].Ref, ast.Selection{
		Kind: ast.SelectionKindInlineFragment,
		Ref:  inlineFragment,
	})
	p.nodes = append(p.nodes, selectionSet)
}

func (p *Planner) addEntitiesSelectionSet() {

	// $representations
	representationsLiteral := p.upstreamOperation.Input.AppendInputString("representations")
	representationsVariable := p.upstreamOperation.AddVariableValue(ast.VariableValue{
		Name: representationsLiteral,
	})
	representationsArgument := p.upstreamOperation.AddArgument(ast.Argument{
		Name: representationsLiteral,
		Value: ast.Value{
			Kind: ast.ValueKindVariable,
			Ref:  representationsVariable,
		},
	})

	// _entities
	entitiesSelectionSet := p.upstreamOperation.AddSelectionSet()
	entitiesField := p.upstreamOperation.AddField(ast.Field{
		Name:          p.upstreamOperation.Input.AppendInputString("_entities"),
		HasSelections: true,
		HasArguments:  true,
		Arguments: ast.ArgumentList{
			Refs: []int{representationsArgument},
		},
		SelectionSet: entitiesSelectionSet.Ref,
	})
	p.upstreamOperation.AddSelection(p.nodes[len(p.nodes)-1].Ref, ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  entitiesField.Ref,
	})
	p.nodes = append(p.nodes, entitiesField, entitiesSelectionSet)
}

func (p *Planner) addRepresentationsVariableDefinition() {
	anyType := p.upstreamOperation.AddNamedType([]byte("_Any"))
	nonNullAnyType := p.upstreamOperation.AddType(ast.Type{
		TypeKind: ast.TypeKindNonNull,
		OfType:   anyType,
	})
	listOfNonNullAnyType := p.upstreamOperation.AddType(ast.Type{
		TypeKind: ast.TypeKindList,
		OfType:   nonNullAnyType,
	})
	nonNullListOfNonNullAnyType := p.upstreamOperation.AddType(ast.Type{
		TypeKind: ast.TypeKindNonNull,
		OfType:   listOfNonNullAnyType,
	})
	representationsVariable := p.upstreamOperation.AddVariableValue(ast.VariableValue{
		Name: p.upstreamOperation.Input.AppendInputBytes([]byte("representations")),
	})
	p.upstreamOperation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, representationsVariable, nonNullListOfNonNullAnyType)
}

func (p *Planner) isNestedRequest() bool {
	for i := range p.nodes {
		if p.nodes[i].Kind == ast.NodeKindField {
			return false
		}
	}
	selectionSetAncestors := 0
	for i := range p.visitor.Walker.Ancestors {
		if p.visitor.Walker.Ancestors[i].Kind == ast.NodeKindSelectionSet {
			selectionSetAncestors++
			if selectionSetAncestors == 2 {
				return true
			}
		}
	}
	return false
}

func (p *Planner) configureArgument(upstreamFieldRef, downstreamFieldRef int, fieldConfig plan.FieldConfiguration, argumentConfiguration plan.ArgumentConfiguration) {
	switch argumentConfiguration.SourceType {
	case plan.FieldArgumentSource:
		p.configureFieldArgumentSource(upstreamFieldRef, downstreamFieldRef, argumentConfiguration.Name, argumentConfiguration.SourcePath)
	case plan.ObjectFieldSource:
		p.configureObjectFieldSource(upstreamFieldRef, downstreamFieldRef, fieldConfig, argumentConfiguration)
	}
}

func (p *Planner) configureFieldArgumentSource(upstreamFieldRef, downstreamFieldRef int, argumentName string, sourcePath []string) {
	fieldArgument, ok := p.visitor.Operation.FieldArgument(downstreamFieldRef, []byte(argumentName))
	if !ok {
		return
	}
	value := p.visitor.Operation.ArgumentValue(fieldArgument)
	if value.Kind != ast.ValueKindVariable {
		p.applyInlineFieldArgument(upstreamFieldRef, downstreamFieldRef, argumentName, sourcePath)
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

	contextVariableName, exists := p.variables.AddVariable(&resolve.ContextVariable{Path: []string{variableNameStr}}, wrapValueInQuotes)
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

func (p *Planner) applyInlineFieldArgument(upstreamField, downstreamField int, argumentName string, sourcePath []string) {
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
	p.addVariableDefinitionsRecursively(value, argumentName, sourcePath)
}

func (p *Planner) addVariableDefinitionsRecursively(value ast.Value, argumentName string, sourcePath []string) {
	switch value.Kind {
	case ast.ValueKindObject:
		for _, i := range p.visitor.Operation.ObjectValues[value.Ref].Refs {
			p.addVariableDefinitionsRecursively(p.visitor.Operation.ObjectFields[i].Value, argumentName, sourcePath)
		}
		return
	case ast.ValueKindList:
		for _, i := range p.visitor.Operation.ListValues[value.Ref].Refs {
			p.addVariableDefinitionsRecursively(p.visitor.Operation.Values[i], argumentName, sourcePath)
		}
		return
	case ast.ValueKindVariable:
		// continue after switch
	default:
		return
	}

	variableName := p.visitor.Operation.VariableValueNameBytes(value.Ref)
	variableNameStr := p.visitor.Operation.VariableValueNameString(value.Ref)
	variableDefinition, exists := p.visitor.Operation.VariableDefinitionByNameAndOperation(p.visitor.Walker.Ancestors[0].Ref, variableName)
	if !exists {
		return
	}
	importedVariableDefinition := p.visitor.Importer.ImportVariableDefinition(variableDefinition, p.visitor.Operation, p.upstreamOperation)
	p.upstreamOperation.AddImportedVariableDefinitionToOperationDefinition(p.nodes[0].Ref, importedVariableDefinition)

	variableDefinitionType := p.visitor.Operation.VariableDefinitions[variableDefinition].Type
	wrapValueInQuotes := p.visitor.Operation.TypeValueNeedsQuotes(variableDefinitionType, p.visitor.Definition)

	contextVariableName, variableExists := p.variables.AddVariable(&resolve.ContextVariable{Path: append(sourcePath, variableNameStr)}, wrapValueInQuotes)
	if variableExists {
		return
	}
	p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, variableNameStr, []byte(contextVariableName))
}

func (p *Planner) configureObjectFieldSource(upstreamFieldRef, downstreamFieldRef int, fieldConfiguration plan.FieldConfiguration, argumentConfiguration plan.ArgumentConfiguration) {
	if len(argumentConfiguration.SourcePath) < 1 {
		return
	}

	fieldName := p.visitor.Operation.FieldNameUnsafeString(downstreamFieldRef)

	if len(fieldConfiguration.Path) == 1 {
		fieldName = fieldConfiguration.Path[0]
	}

	queryTypeDefinition, exists := p.visitor.Definition.Index.FirstNodeByNameBytes(p.visitor.Definition.Index.QueryTypeName)
	if !exists {
		return
	}
	argumentDefinition := p.visitor.Definition.NodeFieldDefinitionArgumentDefinitionByName(queryTypeDefinition, []byte(fieldName), []byte(argumentConfiguration.Name))
	if argumentDefinition == -1 {
		return
	}

	argumentType := p.visitor.Definition.InputValueDefinitionType(argumentDefinition)
	variableName := p.upstreamOperation.GenerateUnusedVariableDefinitionName(p.nodes[0].Ref)
	variableValue, argument := p.upstreamOperation.AddVariableValueArgument([]byte(argumentConfiguration.Name), variableName)
	p.upstreamOperation.AddArgumentToField(upstreamFieldRef, argument)
	importedType := p.visitor.Importer.ImportType(argumentType, p.visitor.Definition, p.upstreamOperation)
	p.upstreamOperation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, variableValue, importedType)
	wrapVariableInQuotes := p.visitor.Definition.TypeValueNeedsQuotes(argumentType, p.visitor.Definition)

	objectVariableName, exists := p.variables.AddVariable(&resolve.ObjectVariable{Path: argumentConfiguration.SourcePath}, wrapVariableInQuotes)
	if !exists {
		p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, string(variableName), []byte(objectVariableName))
	}
}

const (
	normalizationFailedErrMsg = "printOperation: normalization failed"
	parseDocumentFailedErrMsg = "printOperation: parse %s failed"
)

// printOperation - prints normalized upstream operation
func (p *Planner) printOperation() []byte {

	buf := &bytes.Buffer{}

	err := astprinter.Print(p.upstreamOperation, nil, buf)
	if err != nil {
		return nil
	}

	rawQuery := buf.Bytes()

	baseSchema, err := astprinter.PrintString(p.visitor.Definition, nil)
	if err != nil {
		return nil
	}

	federationSchema, err := federation.BuildFederationSchema(baseSchema, p.config.Federation.ServiceSDL)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(err)
		return nil
	}

	// create empty operation and definition documents
	operation := ast.NewDocument()
	definition := ast.NewDocument()
	report := &operationreport.Report{}
	parser := astparser.NewParser()

	// creates a copy of operation and schema to be able to safely modify them
	definition.Input.ResetInputString(federationSchema)
	operation.Input.ResetInputBytes(rawQuery)

	parser.Parse(operation, report)
	if report.HasErrors() {
		p.stopWithError(parseDocumentFailedErrMsg, "operation")
		return nil
	}

	parser.Parse(definition, report)
	if report.HasErrors() {
		p.stopWithError(parseDocumentFailedErrMsg, "definition")
		return nil
	}

	// When datasource is nested and definition query type do not contain operation field
	// we have to replace a query type with a current root type
	p.replaceQueryType(definition)

	// normalize upstream operation
	if !p.normalizeOperation(operation, definition, report) {
		p.stopWithError(normalizationFailedErrMsg)
		return nil
	}

	buf.Reset()

	// print upstream operation
	err = astprinter.Print(operation, p.visitor.Definition, buf)
	if err != nil {
		p.stopWithError(normalizationFailedErrMsg)
		return nil
	}

	return buf.Bytes()
}

func (p *Planner) stopWithError(msg string, args ...interface{}) {
	p.visitor.Walker.StopWithInternalErr(fmt.Errorf(msg, args...))
}

/*
replaceQueryType - sets definition query type to a current root type.
Helps to do a normalization of the upstream query for a nested datasource.
Skips replace when:
1. datasource is not nested;
2. federation is enabled;
3. query type contains an operation field;

Example transformation:
Original schema definition:

type Query {
	serviceOne(serviceOneArg: String): ServiceOneResponse
	serviceTwo(serviceTwoArg: Boolean): ServiceTwoResponse
}
type ServiceOneResponse {
	fieldOne: String!
	countries: [Country!]! # nested datasource without explicit field path
}
type ServiceTwoResponse {
	fieldTwo: String
	serviceOneField: String
	serviceOneResponse: ServiceOneResponse # nested datasource with implicit field path "serviceOne"
}
type Country {
	name: String!
}

`serviceOneResponse` field of a `ServiceTwoResponse` is nested but has a field path that exists on the Query type
- In this case definition will not be modified

`countries` field of a `ServiceOneResponse` is nested and not present on the Query type
- In this case query type of definition will be replaced with a `ServiceOneResponse`

Modified schema definition:

schema {
   query: ServiceOneResponse
}

type ServiceOneResponse {
   fieldOne: String!
   countries: [Country!]!
}

type ServiceTwoResponse {
   fieldTwo: String
   serviceOneField: String
   serviceOneResponse: ServiceOneResponse
}

type Country {
   name: String!
}
Refer to pkg/engine/datasource/graphql_datasource/graphql_datasource_test.go:632
Case name: TestGraphQLDataSource/nested_graphql_engines

If we didn't do this transformation, the normalization would fail because it's not possible
to traverse the AST as there's a mismatch between the upstream Operation and the schema.

If the nested Query can be rewritten so that it's a valid Query against the existing schema, fine.
However, when rewriting the nested Query onto the schema's Query type,
it might be the case that no FieldDefinition exists for the rewritten root field.
In that case, we transform the schema so that normalization and printing of the upstream Query succeeds.
*/
func (p *Planner) replaceQueryType(definition *ast.Document) {
	if !p.isNested || p.config.Federation.Enabled {
		return
	}

	queryTypeName := definition.Index.QueryTypeName
	queryNode, exists := definition.Index.FirstNodeByNameBytes(queryTypeName)
	if !exists || queryNode.Kind != ast.NodeKindObjectTypeDefinition {
		return
	}

	// check that query type has rootFieldName within its fields
	hasField := definition.FieldDefinitionsContainField(definition.ObjectTypeDefinitions[queryNode.Ref].FieldsDefinition.Refs, []byte(p.rootFieldName))
	if hasField {
		return
	}

	definition.RemoveObjectTypeDefinition(definition.Index.QueryTypeName)
	definition.ReplaceRootOperationTypeDefinition(p.rootTypeName, ast.OperationTypeQuery)
}

// normalizeOperation - normalizes operation against definition.
func (p *Planner) normalizeOperation(operation, definition *ast.Document, report *operationreport.Report) (ok bool) {

	report.Reset()
	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	normalizer.NormalizeOperation(operation, definition, report)

	return !report.HasErrors()
}

// addField - add a field to an upstream operation
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
		isDesiredField := p.visitor.Config.Fields[i].TypeName == typeName &&
			p.visitor.Config.Fields[i].FieldName == fieldName

		// chech that we are on a desired field and field path contains a single element - mapping is plain
		if isDesiredField && len(p.visitor.Config.Fields[i].Path) == 1 {
			// define alias when mapping path differs from fieldName and no alias has been defined
			if p.visitor.Config.Fields[i].Path[0] != fieldName && !alias.IsDefined {
				alias.IsDefined = true
				aliasBytes := p.visitor.Operation.FieldNameBytes(ref)
				alias.Name = p.upstreamOperation.Input.AppendInputBytes(aliasBytes)
			}

			// override fieldName with mapping path value
			fieldName = p.visitor.Config.Fields[i].Path[0]

			// when provided field is a root type field save new field name
			if ref == p.rootFieldRef {
				p.rootFieldName = fieldName
			}

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
	Client       httpclient.Client
	BatchFactory resolve.DataSourceBatchFactory
}

func (f *Factory) Planner(<-chan struct{}) plan.DataSourcePlanner {
	return &Planner{
		client:       f.Client,
		batchFactory: f.BatchFactory,
	}
}

var (
	responsePaths = [][]string{
		{"errors"},
		{"data"},
	}
	errorPaths = [][]string{
		{"message"},
		{"locations"},
		{"path"},
	}
	entitiesPath     = []string{"_entities"}
	uniqueIdentifier = []byte(UniqueIdentifier)
)

type Source struct {
	client httpclient.Client
}

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)

	err = s.client.Do(ctx, input, buf)
	if err != nil {
		return
	}

	responseData := buf.Bytes()

	extractEntitiesRaw, _, _, _ := jsonparser.Get(input, "extract_entities")
	extractEntities := bytes.Equal(extractEntitiesRaw, literal.TRUE)

	jsonparser.EachKey(responseData, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			_, _ = jsonparser.ArrayEach(bytes, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
				var (
					message, locations, path []byte
				)
				jsonparser.EachKey(value, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
					switch i {
					case 0:
						message = bytes
					case 1:
						locations = bytes
					case 2:
						path = bytes
					}
				}, errorPaths...)
				if message != nil {
					bufPair.WriteErr(message, locations, path)
				}
			})
		case 1:
			if extractEntities {
				data, _, _, _ := jsonparser.Get(bytes, entitiesPath...)
				bufPair.Data.WriteBytes(data)
				return
			}
			bufPair.Data.WriteBytes(bytes)
		}
	}, responsePaths...)

	return
}

func (s *Source) UniqueIdentifier() []byte {
	return uniqueIdentifier
}
