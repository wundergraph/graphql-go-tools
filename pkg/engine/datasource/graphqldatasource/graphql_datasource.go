package graphqldatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

type Planner struct {
	client                 httpclient.Client
	v                      *plan.Visitor
	fetch                  *resolve.SingleFetch
	printer                astprinter.Printer
	operation              *ast.Document
	nodes                  []ast.Node
	buf                    *bytes.Buffer
	operationNormalizer    *astnormalization.OperationNormalizer
	URL                    []byte
	variables              []byte
	headers                []byte
	bufferID               int
	config                 *plan.DataSourceConfiguration
	abortLeaveDocument     bool
	operationType          ast.OperationType
	isNestedRequest        bool
	isFederation           bool
	federationSDL          []byte
	federationSchema       *ast.Document
	federationRootTypeName []byte
}

func NewPlanner(client httpclient.Client) *Planner {
	return &Planner{
		client: client,
	}
}

func (p *Planner) clientOrDefault() httpclient.Client {
	if p.client != nil {
		return p.client
	}
	return httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient)
}

func (p *Planner) Register(visitor *plan.Visitor) {
	p.v = visitor
	visitor.RegisterFieldVisitor(p)
	visitor.RegisterDocumentVisitor(p)
	visitor.RegisterSelectionSetVisitor(p)
}

func (p *Planner) EnterDocument(_, _ *ast.Document) {
	p.abortLeaveDocument = true
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
		p.operationNormalizer = astnormalization.NewNormalizer(true, true)
	}
	p.nodes = p.nodes[:0]
	p.URL = nil
	p.federationSDL = nil
	p.variables = nil
	p.headers = nil
	p.operationType = ast.OperationTypeUnknown
	p.isFederation = false
	p.federationSDL = nil
	p.federationRootTypeName = nil
	if p.federationSchema == nil {
		p.federationSchema = ast.NewDocument()
	} else {
		p.federationSchema.Reset()
	}
}

func (p *Planner) EnterField(ref int) {

	p.abortLeaveDocument = false // EnterField means this planner is activated

	var (
		isRootField bool
		config      *plan.DataSourceConfiguration
	)

	isRootField, config = p.v.IsRootField(ref)

	if isRootField && config != nil {
		p.config = config
		if len(p.nodes) == 0 { // Setup Fetch and root (operation definition)
			p.URL = config.Attributes.ValueForKey("url")
			p.headers = config.Attributes.ValueForKey(httpclient.HEADERS)
			p.federationSDL = config.Attributes.ValueForKey("federation_service_sdl")
			if p.federationSDL != nil {
				p.isFederation = true
				p.federationRootTypeName = p.v.EnclosingTypeDefinition.NameBytes(p.v.Definition)
				parser := astparser.NewParser()
				p.federationSchema.Input.ResetInputBytes(p.federationSDL)
				parser.Parse(p.federationSchema, p.v.Report)
			}
			if len(p.operation.RootNodes) == 0 {
				set := p.operation.AddSelectionSet()
				p.operationType = ast.OperationTypeQuery
				if len(p.v.Ancestors) == 2 {
					p.operationType = p.v.Operation.OperationDefinitions[p.v.Ancestors[0].Ref].OperationType
				} else {
					p.isNestedRequest = true
				}
				// this means OperationType must always be Query for nested fields
				// if Ancestors are more then 2 this is a field nested in another Query
				// OperationType is the same as the downstream Operation only if this is a root field of the actual Query
				definition := p.operation.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
					OperationType: p.operationType,
					SelectionSet:  set.Ref,
					HasSelections: true,
				})
				p.nodes = append(p.nodes, definition, set)
			}
			if p.operationType != ast.OperationTypeSubscription {
				p.bufferID = p.v.NextBufferID()
				p.fetch = &resolve.SingleFetch{
					BufferId: p.bufferID,
				}
				p.v.SetCurrentObjectFetch(p.fetch, config)
			}
		}
		// subsequent root fields get their own fieldset
		// we need to set the buffer for all fields
		// subscriptions don't have their own buffer, the initial data comes from the trigger which has a buffer itself
		if p.operationType != ast.OperationTypeSubscription {
			p.v.SetBufferIDForCurrentFieldSet(p.bufferID)
		}
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
	if p.isFederation && p.isNestedRequest && isRootField {
		p.addFederationVariables()
	}
}

func (p *Planner) addFederationVariables() {
	enclosingTypeName := p.v.EnclosingTypeDefinition.NameBytes(p.v.Definition)
	definition, ok := p.federationSchema.Index.FirstNodeByNameBytes(enclosingTypeName)
	if !ok {
		return
	}
	var (
		fieldDefinitions []int
		directives       []int
		keys             [][]byte
	)
	switch definition.Kind {
	case ast.NodeKindObjectTypeDefinition:
		fieldDefinitions = p.federationSchema.ObjectTypeDefinitions[definition.Ref].FieldsDefinition.Refs
		directives = p.federationSchema.ObjectTypeDefinitions[definition.Ref].Directives.Refs
	case ast.NodeKindObjectTypeExtension:
		fieldDefinitions = p.federationSchema.ObjectTypeExtensions[definition.Ref].FieldsDefinition.Refs
		directives = p.federationSchema.ObjectTypeExtensions[definition.Ref].Directives.Refs
	case ast.NodeKindInterfaceTypeDefinition:
		fieldDefinitions = p.federationSchema.InterfaceTypeDefinitions[definition.Ref].FieldsDefinition.Refs
		directives = p.federationSchema.InterfaceTypeDefinitions[definition.Ref].Directives.Refs
	case ast.NodeKindInterfaceTypeExtension:
		fieldDefinitions = p.federationSchema.InterfaceTypeExtensions[definition.Ref].FieldsDefinition.Refs
		directives = p.federationSchema.InterfaceTypeExtensions[definition.Ref].Directives.Refs
	default:
		return
	}

	for _, i := range directives {
		name := p.federationSchema.DirectiveNameBytes(i)
		if !bytes.Equal([]byte("key"), name) {
			continue
		}
		value, ok := p.federationSchema.DirectiveArgumentValueByName(i, []byte("fields"))
		if !ok {
			continue
		}
		if value.Kind != ast.ValueKindString {
			continue
		}
		keyValue := p.federationSchema.StringValueContentBytes(value.Ref)
		keys = append(keys, keyValue)
	}

	variableTemplate := []byte(fmt.Sprintf(`{"__typename":"%s"}`, string(enclosingTypeName)))

	for i := range keys {
		key := keys[i]
		for _, j := range fieldDefinitions {
			fieldDefinitionName := p.federationSchema.FieldDefinitionNameBytes(j)
			if !bytes.Equal(fieldDefinitionName, key) {
				continue
			}
			objectVariableName, exists := p.fetch.Variables.AddVariable(&resolve.ObjectVariable{Path: []string{string(fieldDefinitionName)}}, true)
			if !exists {
				variableTemplate, _ = sjson.SetRawBytes(variableTemplate, string(fieldDefinitionName), []byte(objectVariableName))
			}
		}
	}

	representationsVariable := append([]byte("["), append(variableTemplate, []byte("]")...)...)
	p.variables, _ = sjson.SetRawBytes(p.variables, "representations", representationsVariable)
}

func (p *Planner) handleFieldDependencies(downstreamField, upstreamSelectionSet int) {
	typeName := p.v.EnclosingTypeDefinition.NameString(p.v.Definition)
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
			p.addFieldDependency(requiredField, upstreamSelectionSet)
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

	typeName := p.v.EnclosingTypeDefinition.NameString(p.v.Definition)
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
		if fieldArgument, ok := p.v.Operation.FieldArgument(downstreamField, arg.NameBytes()); ok {
			value := p.v.Operation.ArgumentValue(fieldArgument)
			if value.Kind != ast.ValueKindVariable {
				p.applyInlineFieldArgument(upstreamField, downstreamField, arg)
				return
			}
			variableName := p.v.Operation.VariableValueNameBytes(value.Ref)
			variableNameStr := p.v.Operation.VariableValueNameString(value.Ref)

			variableDefinition, ok := p.v.Operation.VariableDefinitionByNameAndOperation(p.v.Ancestors[0].Ref, variableName)
			if !ok {
				return
			}

			variableDefinitionType := p.v.Operation.VariableDefinitions[variableDefinition].Type
			wrapValueInQuotes := p.v.Operation.TypeValueNeedsQuotes(variableDefinitionType, p.v.Definition)

			contextVariableName, exists := p.fetch.Variables.AddVariable(&resolve.ContextVariable{Path: append([]string{variableNameStr}, arg.SourcePath...)}, wrapValueInQuotes)
			variableValueRef, argRef := p.operation.AddVariableValueArgument(arg.NameBytes(), variableName) // add the argument to the field, but don't redefine it
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

			p.variables, _ = sjson.SetRawBytes(p.variables, variableNameStr, []byte(contextVariableName))
		}
	case ObjectField:
		if len(arg.SourcePath) < 1 {
			return
		}

		enclosingTypeName := p.v.EnclosingTypeDefinition.NameString(p.v.Definition)
		fieldName := p.v.Operation.FieldNameString(downstreamField)

		for i := range p.v.Config.FieldMappings {
			if p.v.Config.FieldMappings[i].TypeName == enclosingTypeName &&
				p.v.Config.FieldMappings[i].FieldName == fieldName &&
				len(p.v.Config.FieldMappings[i].Path) == 1 {
				fieldName = p.v.Config.FieldMappings[i].Path[0]
			}
		}

		queryTypeDefinition,exists := p.v.Definition.Index.FirstNodeByNameBytes(p.v.Definition.Index.QueryTypeName)
		if !exists {
			return
		}
		argumentDefinition := p.v.Definition.NodeFieldDefinitionArgumentDefinitionByName(queryTypeDefinition, []byte(fieldName), arg.NameBytes())
		if argumentDefinition == -1 {
			return
		}

		argumentType := p.v.Definition.InputValueDefinitionType(argumentDefinition)
		variableName := p.operation.GenerateUnusedVariableDefinitionName(p.nodes[0].Ref)
		variableValue, argument := p.operation.AddVariableValueArgument(arg.NameBytes(), variableName)
		p.operation.AddArgumentToField(upstreamField, argument)
		importedType := p.v.Importer.ImportType(argumentType, p.v.Definition, p.operation)
		p.operation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, variableValue, importedType)
		wrapVariableInQuotes := p.v.Definition.TypeValueNeedsQuotes(argumentType, p.v.Definition)

		objectVariableName, exists := p.fetch.Variables.AddVariable(&resolve.ObjectVariable{Path: arg.SourcePath}, wrapVariableInQuotes)
		if !exists {
			p.variables, _ = sjson.SetRawBytes(p.variables, string(variableName), []byte(objectVariableName))
		}
	}
}

func (p *Planner) applyInlineFieldArgument(upstreamField, downstreamField int, arg Argument) {
	fieldArgument, ok := p.v.Operation.FieldArgument(downstreamField, arg.NameBytes())
	if !ok {
		return
	}
	value := p.v.Operation.ArgumentValue(fieldArgument)
	importedValue := p.v.Importer.ImportValue(value, p.v.Operation, p.operation)
	argRef := p.operation.AddArgument(ast.Argument{
		Name:  p.operation.Input.AppendInputBytes(arg.NameBytes()),
		Value: importedValue,
	})
	p.operation.AddArgumentToField(upstreamField, argRef)
	p.addVariableDefinitionsRecursively(value, arg)
}

func (p *Planner) addVariableDefinitionsRecursively(value ast.Value, arg Argument) {
	switch value.Kind {
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
	p.variables, _ = sjson.SetRawBytes(p.variables, variableNameStr, []byte(contextVariableName))
}

func (p *Planner) LeaveField(_ int) {
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

	upstreamSelectionSet := p.nodes[len(p.nodes)-1].Ref

	for _, i := range p.v.Operation.SelectionSets[ref].SelectionRefs {
		if p.v.Operation.Selections[i].Kind != ast.SelectionKindField {
			continue
		}
		downstreamField := p.v.Operation.Selections[i].Ref
		p.handleFieldDependencies(downstreamField, upstreamSelectionSet)
	}

	p.addFederationSelections(ref)

	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) addFederationSelections(set int) {
	if !p.isFederation {
		return
	}
	enclosingTypeName := p.v.EnclosingTypeDefinition.NameBytes(p.v.Definition)
	refs := p.v.Operation.SelectionSets[set].SelectionRefs
	for _, i := range refs {
		if p.v.Operation.Selections[i].Kind != ast.SelectionKindField {
			continue
		}
		field := p.v.Operation.Selections[i].Ref
		fieldName := p.v.Operation.FieldNameBytes(field)
		p.addFederatedField(set, fieldName, enclosingTypeName)
	}
}

func (p *Planner) addFederatedField(downstreamSelectionSet int, fieldName, enclosingTypeName []byte) {
	upstreamSelectionSet := p.nodes[len(p.nodes)-1].Ref
	definition, ok := p.federationSchema.Index.FirstNodeByNameBytes(enclosingTypeName)
	if !ok {
		return
	}
	var (
		fieldDefinitions []int
		directives       []int
		keys             [][]byte
		addTypeNameField bool
	)
	switch definition.Kind {
	case ast.NodeKindObjectTypeDefinition:
		fieldDefinitions = p.federationSchema.ObjectTypeDefinitions[definition.Ref].FieldsDefinition.Refs
		directives = p.federationSchema.ObjectTypeDefinitions[definition.Ref].Directives.Refs
	case ast.NodeKindObjectTypeExtension:
		fieldDefinitions = p.federationSchema.ObjectTypeExtensions[definition.Ref].FieldsDefinition.Refs
		directives = p.federationSchema.ObjectTypeExtensions[definition.Ref].Directives.Refs
	case ast.NodeKindInterfaceTypeDefinition:
		fieldDefinitions = p.federationSchema.InterfaceTypeDefinitions[definition.Ref].FieldsDefinition.Refs
		directives = p.federationSchema.InterfaceTypeDefinitions[definition.Ref].Directives.Refs
	case ast.NodeKindInterfaceTypeExtension:
		fieldDefinitions = p.federationSchema.InterfaceTypeExtensions[definition.Ref].FieldsDefinition.Refs
		directives = p.federationSchema.InterfaceTypeExtensions[definition.Ref].Directives.Refs
	default:
		return
	}

	for _, i := range directives {
		name := p.federationSchema.DirectiveNameBytes(i)
		if !bytes.Equal([]byte("key"), name) {
			continue
		}
		value, ok := p.federationSchema.DirectiveArgumentValueByName(i, []byte("fields"))
		if !ok {
			continue
		}
		if value.Kind != ast.ValueKindString {
			continue
		}
		keyValue := p.federationSchema.StringValueContentBytes(value.Ref)
		keys = append(keys, keyValue)
	}

	for i := range keys {
		key := keys[i]
		for _, j := range fieldDefinitions {
			fieldDefinitionName := p.federationSchema.FieldDefinitionNameBytes(j)
			if !bytes.Equal(fieldDefinitionName, key) {
				continue
			}
			_, isExternal := p.federationSchema.FieldDefinitionDirectiveByName(j, []byte("external"))
			if isExternal {
				addTypeNameField = true
				externalField := p.operation.AddField(ast.Field{
					Name: p.operation.Input.AppendInputBytes(fieldDefinitionName),
				}).Ref
				p.operation.AddSelection(upstreamSelectionSet, ast.Selection{
					Kind: ast.SelectionKindField,
					Ref:  externalField,
				})
			}
		}
	}
	if addTypeNameField {
		externalField := p.operation.AddField(ast.Field{
			Name: p.operation.Input.AppendInputBytes(literal.TYPENAME),
		}).Ref
		p.operation.AddSelection(upstreamSelectionSet, ast.Selection{
			Kind: ast.SelectionKindField,
			Ref:  externalField,
		})
	}
}

func (p *Planner) LeaveDocument(_, definition *ast.Document) {
	if p.abortLeaveDocument {
		return // planner did not get activated, skip
	}
	if !p.isFederation {
		p.operationNormalizer.NormalizeOperation(p.operation, definition, p.v.Report)
	}

	buf := &bytes.Buffer{}
	if p.isFederation && p.isNestedRequest {
		_, _ = buf.Write(federationQueryHeader)
		_, _ = buf.Write(p.federationRootTypeName)
		_, _ = buf.Write(literal.SPACE)
	}

	err := p.printer.Print(p.operation, nil, buf)
	if err != nil {
		return
	}

	if p.isFederation && p.isNestedRequest {
		_, _ = buf.Write(federationQueryTrailer)
	}

	switch p.operationType {
	case ast.OperationTypeQuery, ast.OperationTypeMutation:
		var input []byte

		if p.isFederation && p.isNestedRequest {
			input, _ = sjson.SetRawBytes(input, "extract_entities", []byte("true"))
		}

		input = httpclient.SetInputBodyWithPath(input, p.variables, "variables")
		input = httpclient.SetInputBodyWithPath(input, buf.Bytes(), "query")
		input = httpclient.SetInputURL(input, p.URL)
		input = httpclient.SetInputMethod(input, literal.HTTP_METHOD_POST)
		input = httpclient.SetInputHeaders(input, p.headers)

		p.fetch.Input = string(input)
		source := DefaultSource()
		source.client = p.clientOrDefault()
		p.fetch.DataSource = source
		p.fetch.DisallowSingleFlight = p.operation.OperationDefinitions[p.nodes[0].Ref].OperationType != ast.OperationTypeQuery
	case ast.OperationTypeSubscription:

		parsedURL, err := url.Parse(string(p.URL))
		if err != nil {
			p.v.StopWithInternalErr(err)
			return
		}

		scheme := []byte("ws")
		if parsedURL.Scheme == "https" {
			scheme = []byte("wss")
		}

		var input []byte

		if p.isFederation && p.isNestedRequest {
			input, _ = sjson.SetRawBytes(input, "extract_entities", []byte("true"))
		}

		input = httpclient.SetInputHeaders(input, p.headers)
		input = httpclient.SetInputBodyWithPath(input, p.variables, "variables")
		input = httpclient.SetInputBodyWithPath(input, buf.Bytes(), "query")
		input = httpclient.SetInputPath(input, []byte(parsedURL.Path))
		input = httpclient.SetInputHost(input, []byte(parsedURL.Host))
		input = httpclient.SetInputScheme(input, scheme)

		p.v.SetSubscriptionTrigger(resolve.GraphQLSubscriptionTrigger{
			Input:     string(input),
			ManagerID: []byte("graphql_websocket_subscription"),
		}, *p.config)
	}
}

type Source struct {
	client httpclient.Client
}

func DefaultSource() *Source {
	return &Source{
		client: httpclient.NewFastHttpClient(httpclient.DefaultFastHttpClient),
	}
}

var (
	uniqueIdentifier = []byte("graphql")
)

func (_ *Source) UniqueIdentifier() []byte {
	return uniqueIdentifier
}

var (
	federationQueryHeader  = []byte(`query($representations: [_Any!]!){_entities(representations: $representations){... on `)
	federationQueryTrailer = []byte(`}}`)
	responsePaths          = [][]string{
		{"errors"},
		{"data"},
	}
	entitiesPath = []string{"_entities", "[0]"}
)

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
			bufPair.Errors.WriteBytes(bytes)
		case 1:
			if extractEntities {
				data,_,_,_ := jsonparser.Get(bytes,entitiesPath...)
				bufPair.Data.WriteBytes(data)
				return
			}
			bufPair.Data.WriteBytes(bytes)
		}
	}, responsePaths...)

	return
}

func ArgumentsConfigJSON(config ArgumentsConfig) []byte {
	out, _ := json.Marshal(config)
	return out
}

type ArgumentsConfig struct {
	Fields []FieldConfig `json:"fields"`
}

type FieldConfig struct {
	FieldName string     `json:"field_name"`
	Arguments []Argument `json:"arguments"`
}

type Argument struct {
	Name       string         `json:"name"`
	Source     ArgumentSource `json:"source"`
	SourcePath []string       `json:"source_path"`
}

func (a Argument) NameBytes() []byte {
	return []byte(a.Name)
}

type ArgumentSource string

const (
	ObjectField   ArgumentSource = "object_field"
	FieldArgument ArgumentSource = "field_argument"
)
