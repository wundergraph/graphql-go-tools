package graphql_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const removeNullVariablesDirectiveName = "removeNullVariables"

var (
	DefaultPostProcessingConfiguration = resolve.PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data"},
		SelectResponseErrorsPath: []string{"errors"},
	}
	EntitiesPostProcessingConfiguration = resolve.PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data", "_entities"},
		SelectResponseErrorsPath: []string{"errors"},
	}
	SingleEntityPostProcessingConfiguration = resolve.PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data", "_entities", "[0]"},
		SelectResponseErrorsPath: []string{"errors"},
	}
)

type Planner[T Configuration] struct {
	id              int
	debug           bool
	printQueryPlans bool

	visitor                            *plan.Visitor
	dataSourceConfig                   plan.DataSourceConfiguration[T]
	dataSourcePlannerConfig            plan.DataSourcePlannerConfiguration
	config                             Configuration
	upstreamOperation                  *ast.Document
	upstreamVariables                  []byte
	nodes                              []ast.Node
	variables                          resolve.Variables
	lastFieldEnclosingTypeName         string
	fetchClient                        *http.Client
	subscriptionClient                 GraphQLSubscriptionClient
	rootTypeName                       string // rootTypeName - holds name of top level type
	rootFieldName                      string // rootFieldName - holds name of root type field
	rootFieldRef                       int    // rootFieldRef - holds ref of root type field
	argTypeRef                         int    // argTypeRef - holds current argument type ref from the definition
	currentVariableDefinition          int
	addDirectivesToVariableDefinitions map[int][]int
	insideCustomScalarField            bool
	customScalarFieldRef               int
	unnulVariables                     bool
	parentTypeNodes                    []ast.Node

	// federation

	addedInlineFragments map[onTypeInlineFragment]struct{}
	hasFederationRoot    bool

	// tmp
	upstreamSchema *ast.Document
}

type onTypeInlineFragment struct {
	TypeCondition string
	SelectionSet  int
}

func (p *Planner[T]) UpstreamSchema(dataSourceConfig plan.DataSourceConfiguration[T]) (*ast.Document, bool) {
	cfg := Configuration(dataSourceConfig.CustomConfiguration())

	schema, err := cfg.UpstreamSchema()
	if err != nil {
		return nil, false
	}

	return schema, true
}

func (p *Planner[T]) SetID(id int) {
	p.id = id
}

func (p *Planner[T]) ID() (id int) {
	return p.id
}

func (p *Planner[T]) EnableDebug() {
	p.debug = true
}

func (p *Planner[T]) EnableQueryPlanLogging() {
	p.printQueryPlans = true
}

func (p *Planner[T]) parentNodeIsAbstract() bool {
	if len(p.parentTypeNodes) < 2 {
		return false
	}
	parentTypeNode := p.parentTypeNodes[len(p.parentTypeNodes)-2]
	return parentTypeNode.Kind.IsAbstractType()
}

func (p *Planner[T]) EnterVariableDefinition(ref int) {
	p.currentVariableDefinition = ref
}

func (p *Planner[T]) LeaveVariableDefinition(_ int) {
	p.currentVariableDefinition = -1
}

func (p *Planner[T]) EnterDirective(ref int) {
	parent := p.nodes[len(p.nodes)-1]
	if parent.Kind == ast.NodeKindOperationDefinition && p.currentVariableDefinition != -1 {
		p.addDirectivesToVariableDefinitions[p.currentVariableDefinition] = append(p.addDirectivesToVariableDefinitions[p.currentVariableDefinition], ref)
		return
	}
	p.addDirectiveToNode(ref, parent)
}

func (p *Planner[T]) addDirectiveToNode(directiveRef int, node ast.Node) {
	directiveName := p.visitor.Operation.DirectiveNameString(directiveRef)
	operationType := ast.OperationTypeQuery
	if !p.dataSourcePlannerConfig.IsNested {
		operationType = p.visitor.Operation.OperationDefinitions[p.visitor.Walker.Ancestors[0].Ref].OperationType
	}
	if !p.visitor.Definition.DirectiveIsAllowedOnNodeKind(directiveName, node.Kind, operationType) {
		return
	}
	upstreamDirectiveName := p.dataSourceConfig.DirectiveConfigurations().RenameTypeNameOnMatchStr(directiveName)

	upstreamSchema, err := p.config.UpstreamSchema()
	if err != nil {
		p.stopWithError(failedToReadUpstreamSchemaErrMsg, err.Error())
	}

	if !upstreamSchema.DirectiveIsAllowedOnNodeKind(upstreamDirectiveName, node.Kind, operationType) {
		return
	}
	upstreamDirective := p.visitor.Importer.ImportDirectiveWithRename(directiveRef, upstreamDirectiveName, p.visitor.Operation, p.upstreamOperation)
	p.upstreamOperation.AddDirectiveToNode(upstreamDirective, node)

	// The directive is allowed on the node, so we know it exists.
	directive := p.visitor.Operation.Directives[directiveRef]

	var variables []ast.Value

	// Collect all the variable arguments.
	if directive.HasArguments {
		for _, argument := range directive.Arguments.Refs {
			value := p.visitor.Operation.ArgumentValue(argument)
			// TODO: also handle literal values that CONTAIN variables
			if value.Kind == ast.ValueKindVariable {
				variables = append(variables, value)
			}
		}
	}

	// Process each variable, adding it to the upstream operation and
	// variables, if it hasn't already been added. Note: instead of looking
	// up the type of the corresponding argument on the directive definition,
	// this code assumes the type of the variable as defined in the operation
	// is correct and uses the same (possibly mapped) type for the upstream
	// operation.
	for _, value := range variables {
		variableName := p.visitor.Operation.VariableValueNameBytes(value.Ref)

		for _, i := range p.visitor.Operation.OperationDefinitions[p.visitor.Walker.Ancestors[0].Ref].VariableDefinitions.Refs {
			// Find the variable declaration in the downstream operation.
			ref := p.visitor.Operation.VariableDefinitions[i].VariableValue.Ref
			if !p.visitor.Operation.VariableValueNameBytes(ref).Equals(variableName) {
				continue
			}

			// Look up the variable type.
			variableType := p.visitor.Operation.VariableDefinitions[i].Type
			typeName := p.visitor.Operation.ResolveTypeNameString(variableType)

			renderer, err := resolve.NewJSONVariableRendererWithValidationFromTypeRef(p.visitor.Operation, p.visitor.Definition, variableType)
			if err != nil {
				continue
			}

			contextVariable := &resolve.ContextVariable{
				Path:     []string{string(variableName)},
				Renderer: renderer,
			}

			// Try to add the variable to the set of upstream variables.
			contextVariableName, exists := p.variables.AddVariable(contextVariable)

			// If the variable already exists, it also already exists in the
			// upstream operation; there's nothing to add!
			if exists {
				continue
			}

			// Add the variable to the upstream operation. Be sure to map the
			// downstream type to the upstream type, if needed.
			upstreamVariable := p.upstreamOperation.ImportVariableValue(variableName)
			upstreamTypeName := p.visitor.Config.Types.RenameTypeNameOnMatchStr(typeName)
			importedType := p.visitor.Importer.ImportTypeWithRename(p.visitor.Operation.VariableDefinitions[i].Type, p.visitor.Operation, p.upstreamOperation, upstreamTypeName)
			p.upstreamOperation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, upstreamVariable, importedType)

			// Also copy any variable directives in the downstream operation to
			// the upstream operation.
			if add, ok := p.addDirectivesToVariableDefinitions[i]; ok {
				for _, directive := range add {
					p.addDirectiveToNode(directive, ast.Node{Kind: ast.NodeKindVariableDefinition, Ref: i})
				}
			}

			// And finally add the variable to the upstream variables JSON.
			p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, string(variableName), []byte(contextVariableName))
		}
	}
}

func (p *Planner[T]) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
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

func (p *Planner[T]) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      true,
		OverrideFieldPathFromAlias: true,
		IncludeTypeNameFields:      true,
	}
}

func (p *Planner[T]) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration[T], dataSourcePlannerConfiguration plan.DataSourcePlannerConfiguration) error {

	p.visitor = visitor
	p.visitor.Walker.RegisterDocumentVisitor(p)
	p.visitor.Walker.RegisterFieldVisitor(p)
	p.visitor.Walker.RegisterOperationDefinitionVisitor(p)
	p.visitor.Walker.RegisterSelectionSetVisitor(p)
	p.visitor.Walker.RegisterEnterArgumentVisitor(p)
	p.visitor.Walker.RegisterInlineFragmentVisitor(p)
	p.visitor.Walker.RegisterEnterDirectiveVisitor(p)
	p.visitor.Walker.RegisterVariableDefinitionVisitor(p)

	p.dataSourcePlannerConfig = dataSourcePlannerConfiguration
	p.dataSourceConfig = configuration
	p.config = Configuration(configuration.CustomConfiguration())

	return nil
}

func (p *Planner[T]) ConfigureFetch() resolve.FetchConfiguration {
	if p.config.fetch == nil {
		p.stopWithError("fetch configuration is empty")
	}

	var input []byte
	input = httpclient.SetInputBodyWithPath(input, p.upstreamVariables, "variables")
	input = httpclient.SetInputBodyWithPath(input, p.printOperation(), "query")

	if p.unnulVariables {
		input = httpclient.SetInputFlag(input, httpclient.UNNULL_VARIABLES)
	}

	header, err := json.Marshal(p.config.fetch.Header)
	if err == nil && len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = httpclient.SetInputHeader(input, header)
	}

	input = httpclient.SetInputURL(input, []byte(p.config.fetch.URL))
	input = httpclient.SetInputMethod(input, []byte(p.config.fetch.Method))

	postProcessing := DefaultPostProcessingConfiguration
	requiresEntityFetch := p.requiresEntityFetch()
	requiresEntityBatchFetch := p.requiresEntityBatchFetch()

	switch {
	case requiresEntityFetch:
		postProcessing = SingleEntityPostProcessingConfiguration
	case requiresEntityBatchFetch:
		postProcessing = EntitiesPostProcessingConfiguration
	}

	return resolve.FetchConfiguration{
		Input: string(input),
		DataSource: &Source{
			httpClient: p.fetchClient,
		},
		Variables:                             p.variables,
		RequiresEntityFetch:                   requiresEntityFetch,
		RequiresEntityBatchFetch:              requiresEntityBatchFetch,
		PostProcessing:                        postProcessing,
		SetTemplateOutputToNullOnVariableNull: requiresEntityFetch || requiresEntityBatchFetch,
	}
}

func (p *Planner[T]) shouldSelectSingleEntity() bool {
	return p.dataSourcePlannerConfig.HasRequiredFields() &&
		p.dataSourcePlannerConfig.PathType == plan.PlannerPathObject
}

func (p *Planner[T]) requiresEntityFetch() bool {
	return p.dataSourcePlannerConfig.HasRequiredFields() && p.dataSourcePlannerConfig.PathType == plan.PlannerPathObject
}

func (p *Planner[T]) requiresEntityBatchFetch() bool {
	return p.dataSourcePlannerConfig.HasRequiredFields() && p.dataSourcePlannerConfig.PathType != plan.PlannerPathObject
}

func (p *Planner[T]) ConfigureSubscription() plan.SubscriptionConfiguration {
	if p.config.subscription == nil {
		p.stopWithError("subscription configuration is empty")
	}

	input := httpclient.SetInputBodyWithPath(nil, p.upstreamVariables, "variables")
	input = httpclient.SetInputBodyWithPath(input, p.printOperation(), "query")
	input = httpclient.SetInputURL(input, []byte(p.config.subscription.URL))
	if p.config.subscription.UseSSE {
		input = httpclient.SetInputFlag(input, httpclient.USE_SSE)
		if p.config.subscription.SSEMethodPost {
			input = httpclient.SetInputFlag(input, httpclient.SSE_METHOD_POST)
		}
	}
	input = httpclient.SetInputWSSubprotocol(input, []byte(p.config.subscription.WsSubProtocol))

	header, err := json.Marshal(p.config.subscription.Header)
	if err == nil && len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = httpclient.SetInputHeader(input, header)
	}

	if len(p.config.subscription.ForwardedClientHeaderNames) > 0 {
		headers, err := json.Marshal(p.config.subscription.ForwardedClientHeaderNames)
		if err != nil {
			// XXX: Since this is a very unlikely error, to avoid breaking
			// the API we panic here
			panic(err)
		}
		input = httpclient.SetForwardedClientHeaderNames(input, headers)
	}

	if len(p.config.subscription.ForwardedClientHeaderRegularExpressions) > 0 {
		headers, err := json.Marshal(p.config.subscription.ForwardedClientHeaderRegularExpressions)
		if err != nil {
			// XXX: Since this is a very unlikely error, to avoid breaking
			// the API we panic here
			panic(err)
		}
		input = httpclient.SetForwardedClientHeaderRegularExpressions(input, headers)
	}

	return plan.SubscriptionConfiguration{
		Input: string(input),
		DataSource: &SubscriptionSource{
			client: p.subscriptionClient,
		},
		Variables:      p.variables,
		PostProcessing: DefaultPostProcessingConfiguration,
	}
}

func (p *Planner[T]) EnterOperationDefinition(ref int) {
	if p.visitor.Operation.OperationDefinitions[ref].HasDirectives &&
		p.visitor.Operation.OperationDefinitions[ref].Directives.HasDirectiveByName(p.visitor.Operation, removeNullVariablesDirectiveName) {
		p.unnulVariables = true
		p.visitor.Operation.OperationDefinitions[ref].Directives.RemoveDirectiveByName(p.visitor.Operation, removeNullVariablesDirectiveName)
	}

	operationType := p.visitor.Operation.OperationDefinitions[ref].OperationType
	if p.dataSourcePlannerConfig.IsNested {
		operationType = ast.OperationTypeQuery
	}
	definition := p.upstreamOperation.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
		OperationType: operationType,
	})
	p.nodes = append(p.nodes, definition)
}

func (p *Planner[T]) LeaveOperationDefinition(_ int) {
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner[T]) EnterSelectionSet(ref int) {
	p.debugPrintOperationOnEnter(ast.NodeKindSelectionSet, ref)

	p.parentTypeNodes = append(p.parentTypeNodes, p.visitor.Walker.EnclosingTypeDefinition)
	if p.insideCustomScalarField {
		return
	}

	parent := p.nodes[len(p.nodes)-1]
	set := p.upstreamOperation.AddSelectionSet()
	switch parent.Kind {
	case ast.NodeKindSelectionSet:
		// this happens when we're inside the root of a nested abstract federated query
		// we want to walk into and out of the selection set because the root field is abstract
		// this allows us to walk out of the inline fragment in the root
		// however, as a nested operation always starts with an Operation Definition and a Selection Set
		// we don't want to add the selection set to the root nodes
		return
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

	if parent.Kind == ast.NodeKindOperationDefinition {
		p.addRepresentationsQuery()
	}

	if p.visitor.Walker.EnclosingTypeDefinition.Kind != ast.NodeKindInterfaceTypeDefinition {
		return
	}

	// handle adding typename for the InterfaceObject
	typeName := p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)
	for _, interfaceObjectCfg := range p.dataSourceConfig.FederationConfiguration().InterfaceObjects {
		if interfaceObjectCfg.InterfaceTypeName == typeName {
			p.addTypenameToSelectionSet(set.Ref)
			return
		}
	}
}

func (p *Planner[T]) addTypenameToSelectionSet(selectionSet int) {
	field := p.upstreamOperation.AddField(ast.Field{
		Name: p.upstreamOperation.Input.AppendInputString("__typename"),
	})
	p.upstreamOperation.AddSelection(selectionSet, ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	})
}

func (p *Planner[T]) LeaveSelectionSet(ref int) {
	p.debugPrintOperationOnLeave(ast.NodeKindSelectionSet, ref)

	p.parentTypeNodes = p.parentTypeNodes[:len(p.parentTypeNodes)-1]
	if p.insideCustomScalarField {
		return
	}

	lastIndex := len(p.nodes) - 1
	if p.nodes[lastIndex].Kind == ast.NodeKindSelectionSet {
		p.nodes = p.nodes[:lastIndex]
	}
}

func (p *Planner[T]) EnterInlineFragment(ref int) {
	p.debugPrintOperationOnEnter(ast.NodeKindInlineFragment, ref)

	if p.insideCustomScalarField {
		return
	}

	if p.config.IsFederationEnabled() && !p.hasFederationRoot && p.isNestedRequest() {
		// if we're inside the nested root of a federated abstract query,
		// we're walking into the inline fragment as the root
		// however, as we're already handling the inline fragment when we walk into the root field,
		// we can skip this one
		return
	}

	hasTypeCondition := p.visitor.Operation.InlineFragmentHasTypeCondition(ref)
	// after normalization the type condition is always present
	IsOfTheSameType := p.visitor.Operation.InlineFragmentIsOfTheSameType(ref)

	// create inline fragment and selection
	inlineFragmentRef := p.upstreamOperation.AddInlineFragment(ast.InlineFragment{
		TypeCondition: ast.TypeCondition{
			Type: ast.InvalidRef,
		},
	})

	currentSelectionSet := p.nodes[len(p.nodes)-1].Ref

	// when fragment has type condition, and it is not of the same type
	// we need to add __typename field to selection set
	if hasTypeCondition && !IsOfTheSameType {
		typeCondition := p.visitor.Operation.InlineFragmentTypeConditionName(ref)

		onTypeName := p.visitor.Config.Types.RenameTypeNameOnMatchBytes(typeCondition)

		// rename type name in case it is required by entity interface
		shouldRenameInterfaceObjectType, newName := p.interfaceObjectTypeShouldBeRenamed(string(onTypeName))
		if shouldRenameInterfaceObjectType {
			onTypeName = []byte(newName)
		}

		fragmentTypeRef := p.upstreamOperation.AddNamedType(onTypeName)

		p.upstreamOperation.InlineFragments[inlineFragmentRef].TypeCondition.Type = fragmentTypeRef

		if !shouldRenameInterfaceObjectType {
			// add __typename field to selection set which contains typeCondition
			// so that the resolver can distinguish between the response types
			p.addTypenameToSelectionSet(currentSelectionSet)
		}
	}

	selection := ast.Selection{
		Kind: ast.SelectionKindInlineFragment,
		Ref:  inlineFragmentRef,
	}
	p.upstreamOperation.AddSelection(currentSelectionSet, selection)

	inlineFragmentNode := ast.Node{Kind: ast.NodeKindInlineFragment, Ref: inlineFragmentRef}
	p.nodes = append(p.nodes, inlineFragmentNode)
}

func (p *Planner[T]) LeaveInlineFragment(ref int) {
	p.debugPrintOperationOnLeave(ast.NodeKindInlineFragment, ref)

	if p.insideCustomScalarField {
		return
	}

	lastIndex := len(p.nodes) - 1
	if p.nodes[lastIndex].Kind == ast.NodeKindInlineFragment {
		p.nodes = p.nodes[:lastIndex]
	}
}

func (p *Planner[T]) EnterField(ref int) {
	p.debugPrintOperationOnEnter(ast.NodeKindField, ref)

	p.lastFieldEnclosingTypeName = p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)

	if !p.allowField(ref) {
		return
	}

	if p.insideCustomScalarField {
		return
	}

	fieldName := p.visitor.Operation.FieldNameString(ref)
	fieldConfiguration := p.visitor.Config.Fields.ForTypeField(p.lastFieldEnclosingTypeName, fieldName)

	for i := range p.config.customScalarTypeFields {
		if p.config.customScalarTypeFields[i].TypeName == p.lastFieldEnclosingTypeName && p.config.customScalarTypeFields[i].FieldName == fieldName {
			p.insideCustomScalarField = true
			p.customScalarFieldRef = ref
			p.addFieldArguments(p.addCustomField(ref), ref, fieldConfiguration)
			return
		}
	}

	// store root field name and ref
	if p.rootFieldName == "" {
		p.rootFieldName = fieldName
		p.rootFieldRef = ref
	}
	// store root type name
	if p.rootTypeName == "" {
		p.rootTypeName = p.lastFieldEnclosingTypeName
	}

	p.handleOnTypeInlineFragment()

	p.addFieldArguments(p.addField(ref), ref, fieldConfiguration)
}

func (p *Planner[T]) addFieldArguments(upstreamFieldRef int, fieldRef int, fieldConfiguration *plan.FieldConfiguration) {
	if fieldConfiguration != nil {
		for i := range fieldConfiguration.Arguments {
			argumentConfiguration := fieldConfiguration.Arguments[i]
			p.configureArgument(upstreamFieldRef, fieldRef, *fieldConfiguration, argumentConfiguration)
		}
	}
}

func (p *Planner[T]) addCustomField(ref int) (upstreamFieldRef int) {
	fieldName, alias := p.handleFieldAlias(ref)
	fieldNode := p.upstreamOperation.AddField(ast.Field{
		Name:  p.upstreamOperation.Input.AppendInputString(fieldName),
		Alias: alias,
	})
	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  fieldNode.Ref,
	}
	p.upstreamOperation.AddSelection(p.nodes[len(p.nodes)-1].Ref, selection)
	return fieldNode.Ref
}

func (p *Planner[T]) LeaveField(ref int) {
	p.debugPrintOperationOnLeave(ast.NodeKindField, ref)

	if !p.allowField(ref) {
		return
	}

	if p.insideCustomScalarField {
		if p.customScalarFieldRef == ref {
			p.insideCustomScalarField = false
			p.customScalarFieldRef = 0
		}
		return
	}

	p.nodes = p.nodes[:len(p.nodes)-1]
}

// allowField - allows processing a field if datasource has corresponding root or child node
// This check is required cause planner will add parent path as well, but we not always need to add fields from path.
// This is 3rd step of checks in addition to: planning path and skipFor functionality
// if field is __typename, it is always allowed
func (p *Planner[T]) allowField(ref int) bool {
	fieldAliasOrName := p.visitor.Operation.FieldAliasOrNameString(ref)

	// In addition, we skip field if its path are equal to planner parent path
	// This is required to correctly plan on datasource which has corresponding child/root node,
	// but we don't need to add it to the query as we are in the nested request
	currentPath := fmt.Sprintf("%s.%s", p.visitor.Walker.Path.DotDelimitedString(), fieldAliasOrName)
	if p.dataSourcePlannerConfig.ParentPath != "query" && p.dataSourcePlannerConfig.ParentPath == currentPath {
		p.DebugPrint("allowField: false path:", currentPath, "is equal to parent path", p.dataSourcePlannerConfig.ParentPath)
		return false
	}

	return true
}

func (p *Planner[T]) EnterArgument(_ int) {
	if p.insideCustomScalarField {
		return
	}
}

func (p *Planner[T]) EnterDocument(_, _ *ast.Document) {
	if p.upstreamOperation == nil {
		p.upstreamOperation = ast.NewSmallDocument()
	} else {
		p.upstreamOperation.Reset()
	}
	p.nodes = p.nodes[:0]
	p.parentTypeNodes = p.parentTypeNodes[:0]
	p.upstreamVariables = nil
	p.variables = p.variables[:0]
	p.hasFederationRoot = false

	// reset information about root type
	p.rootTypeName = ""
	p.rootFieldName = ""
	p.rootFieldRef = -1

	// reset info about arg type
	p.argTypeRef = -1

	p.addDirectivesToVariableDefinitions = map[int][]int{}
	p.addedInlineFragments = map[onTypeInlineFragment]struct{}{}
}

func (p *Planner[T]) LeaveDocument(_, _ *ast.Document) {
	p.addRepresentationsVariable()
}

func (p *Planner[T]) addRepresentationsVariable() {
	if !p.dataSourcePlannerConfig.HasRequiredFields() {
		return
	}

	variable, _ := p.variables.AddVariable(p.buildRepresentationsVariable())

	p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, "representations", []byte(fmt.Sprintf("[%s]", variable)))
}

func (p *Planner[T]) buildRepresentationsVariable() resolve.Variable {
	objects := make([]*resolve.Object, 0, len(p.dataSourcePlannerConfig.RequiredFields))
	for _, cfg := range p.dataSourcePlannerConfig.RequiredFields {
		node, err := buildRepresentationVariableNode(p.visitor.Definition, cfg, p.dataSourceConfig.FederationConfiguration())
		if err != nil {
			p.visitor.Walker.StopWithInternalErr(err)
			return nil
		}

		objects = append(objects, node)
	}

	return resolve.NewResolvableObjectVariable(
		mergeRepresentationVariableNodes(objects),
	)
}

func (p *Planner[T]) addRepresentationsQuery() {
	isNestedFederationRequest := p.dataSourcePlannerConfig.IsNested && p.config.IsFederationEnabled() && p.dataSourcePlannerConfig.HasRequiredFields()

	if !isNestedFederationRequest {
		return
	}

	p.hasFederationRoot = true
	// query($representations: [_Any!]!){_entities(representations: $representations){... on Product
	p.addRepresentationsVariableDefinition() // $representations: [_Any!]!
	p.addEntitiesSelectionSet()              // {_entities(representations: $representations)
}

func (p *Planner[T]) handleOnTypeInlineFragment() {
	if p.hasFederationRoot {
		if !p.isInEntitiesSelectionSet() {
			// if we quering field of entity type, but we are not in the root of the query
			// we should not add representations variable and should not add inline fragment
			// e.g. query { _entities { ... on Product { price info {name} } } }
			// where Info is an entity type, but it is used as a field of Product
			//
			// At the same time, we may need to update representations variables, but not to add an inline fragment
			// cause, we could have a field which requires an additional field from entity type, which is not a key
			return
		}

		// add inline fragment again, because it could be another entity type
		// e.g. parallel request for a few entities
		// ... on Product { price }
		// ,,, on StockItem { stock }
		if !p.isOnTypeInlineFragmentAllowed() {
			return
		}
		p.addOnTypeInlineFragment() // ... on Product
		return
	}
}

func (p *Planner[T]) fieldDefinition(fieldName, typeName string) *ast.FieldDefinition {
	node, ok := p.visitor.Definition.Index.FirstNodeByNameStr(typeName)
	if !ok {
		return nil
	}
	definition, ok := p.visitor.Definition.NodeFieldDefinitionByName(node, []byte(fieldName))
	if !ok {
		return nil
	}
	return &p.visitor.Definition.FieldDefinitions[definition]
}

// isOnTypeInlineFragmentAllowed returns false if we already have an entity fragment with the same type name
func (p *Planner[T]) isOnTypeInlineFragmentAllowed() bool {
	p.DebugPrint("isOnTypeInlineFragmentAllowed")

	if !(len(p.nodes) > 1 && p.nodes[len(p.nodes)-1].Kind == ast.NodeKindSelectionSet) {
		return true
	}

	fragmentInfo := onTypeInlineFragment{
		TypeCondition: p.lastFieldEnclosingTypeName,
		SelectionSet:  p.nodes[len(p.nodes)-1].Ref,
	}

	_, exists := p.addedInlineFragments[fragmentInfo]
	return !exists
}

func (p *Planner[T]) isInEntitiesSelectionSet() bool {
	if !(len(p.nodes) > 2) {
		return false
	}

	if p.nodes[len(p.nodes)-1].Kind != ast.NodeKindSelectionSet {
		return false
	}

	if p.nodes[len(p.nodes)-2].Kind != ast.NodeKindField {
		return false
	}

	fieldName := p.upstreamOperation.FieldNameBytes(p.nodes[len(p.nodes)-2].Ref)

	return bytes.Equal(fieldName, []byte("_entities"))
}

func (p *Planner[T]) interfaceObjectTypeShouldBeRenamed(typeName string) (ok bool, newName string) {
	for _, interfaceObjectCfg := range p.dataSourceConfig.FederationConfiguration().InterfaceObjects {
		if slices.Contains(interfaceObjectCfg.ConcreteTypeNames, typeName) {
			return true, interfaceObjectCfg.InterfaceTypeName
		}
	}
	return false, ""
}

func (p *Planner[T]) addOnTypeInlineFragment() {
	p.DebugPrint("addOnTypeInlineFragment")

	shouldCopyDirectives := false
	copyFromRef := ast.InvalidRef
	if len(p.visitor.Walker.Ancestors) > 2 &&
		p.visitor.Walker.Ancestors[len(p.visitor.Walker.Ancestors)-2].Kind == ast.NodeKindInlineFragment &&
		p.visitor.Operation.InlineFragmentHasDirectives(p.visitor.Walker.Ancestors[len(p.visitor.Walker.Ancestors)-2].Ref) {
		shouldCopyDirectives = true
		copyFromRef = p.visitor.Walker.Ancestors[len(p.visitor.Walker.Ancestors)-2].Ref

		// TODO: check whole ancestor tree for the inline fragment with directives
	}

	selectionSet := p.upstreamOperation.AddSelectionSet()
	onTypeName := p.visitor.Config.Types.RenameTypeNameOnMatchBytes([]byte(p.lastFieldEnclosingTypeName))

	// rename type name in case it is required by entity interface
	shouldRenameInterfaceObjectType, newName := p.interfaceObjectTypeShouldBeRenamed(p.lastFieldEnclosingTypeName)
	if shouldRenameInterfaceObjectType {
		onTypeName = []byte(newName)
	}

	// we should not request a typename of interface object
	if !shouldRenameInterfaceObjectType {
		p.addTypenameToSelectionSet(p.nodes[len(p.nodes)-1].Ref)
	}

	typeRef := p.upstreamOperation.AddNamedType(onTypeName)
	inlineFragment := p.upstreamOperation.AddInlineFragment(ast.InlineFragment{
		HasSelections: true,
		SelectionSet:  selectionSet.Ref,
		TypeCondition: ast.TypeCondition{
			Type: typeRef,
		},
	})

	if shouldCopyDirectives {
		for _, ref := range p.visitor.Operation.InlineFragments[copyFromRef].Directives.Refs {
			p.addDirectiveToNode(ref, ast.Node{Kind: ast.NodeKindInlineFragment, Ref: inlineFragment})
		}
	}

	p.upstreamOperation.AddSelection(p.nodes[len(p.nodes)-1].Ref, ast.Selection{
		Kind: ast.SelectionKindInlineFragment,
		Ref:  inlineFragment,
	})
	p.nodes = append(p.nodes, selectionSet)

	fragmentInfo := onTypeInlineFragment{
		SelectionSet:  selectionSet.Ref,
		TypeCondition: string(onTypeName),
	}
	p.addedInlineFragments[fragmentInfo] = struct{}{}
}

func (p *Planner[T]) addEntitiesSelectionSet() {
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

func (p *Planner[T]) addRepresentationsVariableDefinition() {
	anyType := p.upstreamOperation.AddNamedType([]byte("_Any"))
	nonNullAnyType := p.upstreamOperation.AddNonNullType(anyType)
	listOfNonNullAnyType := p.upstreamOperation.AddListType(nonNullAnyType)
	nonNullListOfNonNullAnyType := p.upstreamOperation.AddNonNullType(listOfNonNullAnyType)

	representationsVariable := p.upstreamOperation.ImportVariableValue([]byte("representations"))
	p.upstreamOperation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, representationsVariable, nonNullListOfNonNullAnyType)
}

func (p *Planner[T]) isNestedRequest() bool {
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

func (p *Planner[T]) storeArgType(typeName, fieldName, argName string) {
	typeNode, _ := p.visitor.Definition.Index.FirstNodeByNameStr(typeName)

	for _, fieldDefRef := range p.visitor.Definition.ObjectTypeDefinitions[typeNode.Ref].FieldsDefinition.Refs {
		if bytes.Equal(p.visitor.Definition.FieldDefinitionNameBytes(fieldDefRef), []byte(fieldName)) {
			for _, argDefRef := range p.visitor.Definition.FieldDefinitions[fieldDefRef].ArgumentsDefinition.Refs {
				if bytes.Equal(p.visitor.Definition.InputValueDefinitionNameBytes(argDefRef), []byte(argName)) {
					p.argTypeRef = p.visitor.Definition.ResolveListOrNameType(p.visitor.Definition.InputValueDefinitions[argDefRef].Type)
					return
				}
			}
		}
	}
}

func (p *Planner[T]) configureArgument(upstreamFieldRef, downstreamFieldRef int, fieldConfig plan.FieldConfiguration, argumentConfiguration plan.ArgumentConfiguration) {
	p.storeArgType(fieldConfig.TypeName, fieldConfig.FieldName, argumentConfiguration.Name)

	switch argumentConfiguration.SourceType {
	case plan.FieldArgumentSource:
		p.configureFieldArgumentSource(upstreamFieldRef, downstreamFieldRef, argumentConfiguration)
	case plan.ObjectFieldSource:
		p.configureObjectFieldSource(upstreamFieldRef, downstreamFieldRef, fieldConfig, argumentConfiguration)
	}

	p.argTypeRef = -1
}

// configureFieldArgumentSource - creates variables for a plain argument types, in case object or list types goes deep and calls applyInlineFieldArgument
func (p *Planner[T]) configureFieldArgumentSource(upstreamFieldRef, downstreamFieldRef int, argumentConfiguration plan.ArgumentConfiguration) {
	fieldArgument, ok := p.visitor.Operation.FieldArgument(downstreamFieldRef, []byte(argumentConfiguration.Name))
	if !ok {
		return
	}
	value := p.visitor.Operation.ArgumentValue(fieldArgument)
	if value.Kind != ast.ValueKindVariable {
		p.applyInlineFieldArgument(upstreamFieldRef, downstreamFieldRef, argumentConfiguration.Name, argumentConfiguration.SourcePath)
		return
	}
	variableName := p.visitor.Operation.VariableValueNameBytes(value.Ref)
	variableNameStr := p.visitor.Operation.VariableValueNameString(value.Ref)

	fieldName := p.visitor.Operation.FieldNameBytes(downstreamFieldRef)
	argumentDefinition := p.visitor.Definition.NodeFieldDefinitionArgumentDefinitionByName(p.visitor.Walker.EnclosingTypeDefinition, fieldName, []byte(argumentConfiguration.Name))

	if argumentDefinition == -1 {
		return
	}

	argumentType := p.visitor.Definition.InputValueDefinitionType(argumentDefinition)
	renderer, err := resolve.NewJSONVariableRendererWithValidationFromTypeRef(p.visitor.Definition, p.visitor.Definition, argumentType)
	if err != nil {
		return
	}

	contextVariable := &resolve.ContextVariable{
		Path:     []string{variableNameStr},
		Renderer: renderer,
	}

	contextVariableName, exists := p.variables.AddVariable(contextVariable)
	variableValueRef, argRef := p.upstreamOperation.AddVariableValueArgument([]byte(argumentConfiguration.Name), variableName) // add the argument to the field, but don't redefine it
	p.upstreamOperation.AddArgumentToField(upstreamFieldRef, argRef)

	if exists { // if the variable exists we don't have to put it onto the variables declaration again, skip
		return
	}

	for _, i := range p.visitor.Operation.OperationDefinitions[p.visitor.Walker.Ancestors[0].Ref].VariableDefinitions.Refs {
		ref := p.visitor.Operation.VariableDefinitions[i].VariableValue.Ref
		if !p.visitor.Operation.VariableValueNameBytes(ref).Equals(variableName) {
			continue
		}
		typeName := p.visitor.Operation.ResolveTypeNameString(p.visitor.Operation.VariableDefinitions[i].Type)
		typeName = p.visitor.Config.Types.RenameTypeNameOnMatchStr(typeName)
		if argumentConfiguration.RenameTypeTo != "" {
			typeName = argumentConfiguration.RenameTypeTo
		}
		importedType := p.visitor.Importer.ImportTypeWithRename(p.visitor.Operation.VariableDefinitions[i].Type, p.visitor.Operation, p.upstreamOperation, typeName)
		p.upstreamOperation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, variableValueRef, importedType)

		if add, ok := p.addDirectivesToVariableDefinitions[i]; ok {
			for _, directive := range add {
				p.addDirectiveToNode(directive, ast.Node{Kind: ast.NodeKindVariableDefinition, Ref: i})
			}
		}
	}

	p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, variableNameStr, []byte(contextVariableName))
}

// applyInlineFieldArgument - configures arguments for a complex argument of a list or input object type
func (p *Planner[T]) applyInlineFieldArgument(upstreamField, downstreamField int, argumentName string, sourcePath []string) {
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

	p.addVariableDefinitionsRecursively(value, sourcePath, nil)
}

// resolveNestedArgumentType - extracts type of nested field or array element of argument
// fieldName - exists only for ast.ValueKindObject type of argument
func (p *Planner[T]) resolveNestedArgumentType(fieldName []byte) (fieldTypeRef int) {
	if fieldName == nil {
		return p.visitor.Definition.ResolveListOrNameType(p.argTypeRef)
	}

	argTypeName := p.visitor.Definition.ResolveTypeNameString(p.argTypeRef)
	argTypeNode, _ := p.visitor.Definition.Index.FirstNodeByNameStr(argTypeName)

	for _, inputFieldDefRef := range p.visitor.Definition.InputObjectTypeDefinitions[argTypeNode.Ref].InputFieldsDefinition.Refs {
		if bytes.Equal(p.visitor.Definition.InputValueDefinitionNameBytes(inputFieldDefRef), fieldName) {
			return p.visitor.Definition.InputValueDefinitions[inputFieldDefRef].Type
		}
	}

	return -1
}

// addVariableDefinitionsRecursively - recursively configures variables inside a list or an input type
func (p *Planner[T]) addVariableDefinitionsRecursively(value ast.Value, sourcePath []string, fieldName []byte) {
	switch value.Kind {
	case ast.ValueKindObject:
		prevArgTypeRef := p.argTypeRef
		p.argTypeRef = p.resolveNestedArgumentType(fieldName)
		for _, objectFieldRef := range p.visitor.Operation.ObjectValues[value.Ref].Refs {
			p.addVariableDefinitionsRecursively(p.visitor.Operation.ObjectFields[objectFieldRef].Value, sourcePath, p.visitor.Operation.ObjectFieldNameBytes(objectFieldRef))
		}
		p.argTypeRef = prevArgTypeRef
		return
	case ast.ValueKindList:
		for _, i := range p.visitor.Operation.ListValues[value.Ref].Refs {
			p.addVariableDefinitionsRecursively(p.visitor.Operation.Values[i], sourcePath, nil)
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

	variableDefinitionTypeRef := p.visitor.Operation.VariableDefinitions[variableDefinition].Type
	variableDefinitionTypeName := p.visitor.Operation.ResolveTypeNameString(variableDefinitionTypeRef)
	variableDefinitionTypeName = p.visitor.Config.Types.RenameTypeNameOnMatchStr(variableDefinitionTypeName)

	contextVariable := &resolve.ContextVariable{
		Path: append(sourcePath, variableNameStr),
	}
	renderer, err := resolve.NewJSONVariableRendererWithValidationFromTypeRef(p.visitor.Operation, p.visitor.Definition, variableDefinitionTypeRef)
	if err != nil {
		return
	}
	contextVariable.Renderer = renderer
	contextVariableName, variableExists := p.variables.AddVariable(contextVariable)
	if variableExists {
		return
	}

	importedVariableDefinition := p.visitor.Importer.ImportVariableDefinitionWithRename(variableDefinition, p.visitor.Operation, p.upstreamOperation, variableDefinitionTypeName)
	p.upstreamOperation.AddImportedVariableDefinitionToOperationDefinition(p.nodes[0].Ref, importedVariableDefinition)

	p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, variableNameStr, []byte(contextVariableName))
}

// configureObjectFieldSource - configures source of a field when it has variables coming from current object
func (p *Planner[T]) configureObjectFieldSource(upstreamFieldRef, downstreamFieldRef int, fieldConfiguration plan.FieldConfiguration, argumentConfiguration plan.ArgumentConfiguration) {
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

	typeName := p.visitor.Operation.ResolveTypeNameString(argumentType)
	typeName = p.visitor.Config.Types.RenameTypeNameOnMatchStr(typeName)
	if argumentConfiguration.RenameTypeTo != "" {
		typeName = argumentConfiguration.RenameTypeTo
	}

	importedType := p.visitor.Importer.ImportTypeWithRename(argumentType, p.visitor.Definition, p.upstreamOperation, typeName)
	p.upstreamOperation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, variableValue, importedType)

	renderer, err := resolve.NewJSONVariableRendererWithValidationFromTypeRef(p.visitor.Definition, p.visitor.Definition, argumentType)
	if err != nil {
		return
	}

	variable := &resolve.ObjectVariable{
		Path:     argumentConfiguration.SourcePath,
		Renderer: renderer,
	}

	objectVariableName, exists := p.variables.AddVariable(variable)
	if !exists {
		p.upstreamVariables, _ = sjson.SetRawBytes(p.upstreamVariables, string(variableName), []byte(objectVariableName))
	}
}

const (
	normalizationFailedErrMsg        = "upstream operation: normalization failed: %s"
	printOperationFailedErrMsg       = "upstream operation: failed to print: %s"
	validationFailedErrMsg           = "upstream operation: validation failed: %s"
	parseDocumentFailedErrMsg        = "upstream operation: parse %s failed"
	failedToReadUpstreamSchemaErrMsg = "failed to read upstream schema: %s"
)

func (p *Planner[T]) debugPrintOperationOnEnter(kind ast.NodeKind, ref int) {
	if !p.debug {
		return
	}

	switch kind {
	case ast.NodeKindField:
		fieldName := p.visitor.Operation.FieldNameString(ref)
		p.DebugPrint("EnterField : ", fieldName, " ref: ", ref)
	case ast.NodeKindInlineFragment:
		fragmentTypeCondition := p.visitor.Operation.InlineFragmentTypeConditionNameString(ref)
		p.DebugPrint("EnterInlineFragment : ", fragmentTypeCondition, " ref: ", ref)
	case ast.NodeKindSelectionSet:
		p.DebugPrint("EnterSelectionSet", " ref: ", ref)
	}
	p.debugPrintOperation()
}

func (p *Planner[T]) debugPrintOperationOnLeave(kind ast.NodeKind, ref int) {
	if !p.debug {
		return
	}

	switch kind {
	case ast.NodeKindField:
		fieldName := p.visitor.Operation.FieldNameString(ref)
		p.DebugPrint("LeaveField : ", fieldName, " ref: ", ref)
	case ast.NodeKindInlineFragment:
		fragmentTypeCondition := p.visitor.Operation.InlineFragmentTypeConditionNameString(ref)
		p.DebugPrint("LeaveInlineFragment : ", fragmentTypeCondition, " ref: ", ref)
	case ast.NodeKindSelectionSet:
		p.DebugPrint("LeaveSelectionSet", " ref: ", ref)
	}
	p.debugPrintOperation()
}

func (p *Planner[T]) debugPrintOperation() {
	if !p.debug {
		return
	}

	op, _ := astprinter.PrintStringIndentDebug(p.upstreamOperation, nil, "  ")
	p.DebugPrint("printed operation:\n", op)
}

func (p *Planner[T]) DebugPrint(args ...interface{}) {
	if !p.debug {
		return
	}

	p.debugPrintln(args...)
}

func (p *Planner[T]) debugPrintln(args ...interface{}) {
	allArgs := []interface{}{fmt.Sprintf("[planner_id: %d] [ds_id: %s ds_hash: %d url: %s] ", p.id, p.dataSourceConfig.Id(), p.dataSourceConfig.Hash(), p.config.fetch.URL)}
	allArgs = append(allArgs, args...)
	fmt.Println(allArgs...)
}

func (p *Planner[T]) printQueryPlan(operation *ast.Document) {
	if !p.printQueryPlans {
		return
	}

	printedOperation, err := astprinter.PrintStringIndent(operation, nil, "  ")
	if err != nil {
		return
	}

	args := []interface{}{
		"Execution plan:\n",
		"Planner path: ",
		p.dataSourcePlannerConfig.ParentPath,
		"\n",
	}

	if len(p.upstreamVariables) > 0 {
		args = append(args,
			"variables:\n",
			string(p.upstreamVariables),
			"\n")
	}

	if p.dataSourcePlannerConfig.HasRequiredFields() {
		args = append(args, "Representations:\n")
		for _, cfg := range p.dataSourcePlannerConfig.RequiredFields {
			key, report := plan.RequiredFieldsFragment(cfg.TypeName, cfg.SelectionSet, true)
			if report.HasErrors() {
				continue
			}
			printedKey, err := astprinter.PrintStringIndent(key, nil, "  ")
			if err != nil {
				continue
			}
			args = append(args, printedKey, "\n")
		}
	}

	args = append(args,
		"operation:\n",
		printedOperation)

	p.debugPrintln(args...)
}

// printOperation - prints normalized upstream operation
func (p *Planner[T]) printOperation() []byte {
	buf := &bytes.Buffer{}

	err := astprinter.PrintIndent(p.upstreamOperation, nil, []byte("  "), buf)
	if err != nil {
		return nil
	}

	rawQuery := buf.Bytes()

	// create empty operation and definition documents
	operation := ast.NewSmallDocument()
	definition, err := p.config.UpstreamSchema()
	if err != nil {
		p.stopWithError(failedToReadUpstreamSchemaErrMsg, err.Error())
		return nil
	}

	report := &operationreport.Report{}
	operationParser := astparser.NewParser()

	operation.Input.ResetInputBytes(rawQuery)
	operationParser.Parse(operation, report)
	if report.HasErrors() {
		p.stopWithError(parseDocumentFailedErrMsg, "operation")
		return nil
	}

	// When datasource is nested and definition query type do not contain operation field
	// we have to replace a query type with a current root type
	p.replaceQueryType(definition)

	// normalize upstream operation
	if !p.normalizeOperation(operation, definition, report) {
		p.stopWithError(normalizationFailedErrMsg, report.Error())
		return nil
	}

	validator := astvalidation.DefaultOperationValidator()
	validator.Validate(operation, definition, report)
	if report.HasErrors() {
		p.stopWithError(validationFailedErrMsg, report.Error())
		return nil
	}

	// p.printQueryPlan(p.upstreamOperation) // uncomment to print upstream operation before normalization
	p.printQueryPlan(operation)

	buf.Reset()

	// print upstream operation
	err = astprinter.Print(operation, p.visitor.Definition, buf)
	if err != nil {
		p.stopWithError(printOperationFailedErrMsg, report.Error())
		return nil
	}

	return buf.Bytes()
}

func (p *Planner[T]) stopWithError(msg string, args ...interface{}) {
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
func (p *Planner[T]) replaceQueryType(definition *ast.Document) {
	if !p.dataSourcePlannerConfig.IsNested || p.config.IsFederationEnabled() {
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
func (p *Planner[T]) normalizeOperation(operation, definition *ast.Document, report *operationreport.Report) (ok bool) {
	report.Reset()
	normalizer := astnormalization.NewWithOpts(
		// we should not extract variables from the upstream operation as they will be lost
		// cause when we are building an input we use our own variables
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
		astnormalization.WithInlineFragmentSpreads(),
	)
	normalizer.NormalizeOperation(operation, definition, report)

	return !report.HasErrors()
}

func (p *Planner[T]) handleFieldAlias(ref int) (newFieldName string, alias ast.Alias) {
	fieldName := p.visitor.Operation.FieldNameString(ref)
	alias = ast.Alias{
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

		// check that we are on a desired field and field path contains a single element - mapping is plain
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
	return fieldName, alias
}

// addField - add a field to an upstream operation
func (p *Planner[T]) addField(ref int) (upstreamFieldRef int) {
	fieldName, alias := p.handleFieldAlias(ref)

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

	return field.Ref
}

type OnWsConnectionInitCallback func(ctx context.Context, url string, header http.Header) (json.RawMessage, error)

type Factory[T Configuration] struct {
	executionContext   context.Context
	httpClient         *http.Client
	subscriptionClient GraphQLSubscriptionClient
}

// NewFactory creates a new factory for the GraphQL datasource planner
// Graphql Datasource could be stateful in case you are using subscriptions,
// make sure you are using the same execution context for all datasources
func NewFactory(executionContext context.Context, httpClient *http.Client, subscriptionClient GraphQLSubscriptionClient) (*Factory[Configuration], error) {
	if executionContext == nil {
		return nil, fmt.Errorf("execution context is required")
	}
	if httpClient == nil {
		return nil, fmt.Errorf("http client is required")
	}
	if subscriptionClient == nil {
		return nil, fmt.Errorf("subscription client is required")
	}

	return &Factory[Configuration]{
		executionContext:   executionContext,
		httpClient:         httpClient,
		subscriptionClient: subscriptionClient,
	}, nil
}

func (f *Factory[T]) Planner(logger abstractlogger.Logger) plan.DataSourcePlanner[T] {
	return &Planner[T]{
		fetchClient:        f.httpClient,
		subscriptionClient: f.subscriptionClient,
	}
}

func (f *Factory[T]) Context() context.Context {
	return f.executionContext
}

type Source struct {
	httpClient *http.Client
}

func (s *Source) compactAndUnNullVariables(input []byte) []byte {
	undefinedVariables := httpclient.UndefinedVariables(input)
	variables, _, _, err := jsonparser.Get(input, "body", "variables")
	if err != nil {
		return input
	}
	if bytes.Equal(variables, []byte("null")) || bytes.Equal(variables, []byte("{}")) {
		return input
	}
	if bytes.ContainsAny(variables, " \t\n\r") {
		buf := bytes.NewBuffer(make([]byte, 0, len(variables)))
		if err := json.Compact(buf, variables); err != nil {
			panic(fmt.Errorf("compacting variables: %w", err))
		}
		variables = buf.Bytes()
	}

	removeNullVariables := httpclient.IsInputFlagSet(input, httpclient.UNNULL_VARIABLES)
	variables = s.cleanupVariables(variables, removeNullVariables, undefinedVariables)

	input, _ = jsonparser.Set(input, variables, "body", "variables")
	return input
}

// cleanupVariables removes null variables and empty objects from the input if removeNullVariables is true
// otherwise returns the input as is
func (s *Source) cleanupVariables(variables []byte, removeNullVariables bool, undefinedVariables []string) []byte {
	cp := make([]byte, len(variables))
	copy(cp, variables)

	// remove null variables from JSON: {"a":null,"b":1} -> {"b":1}
	err := jsonparser.ObjectEach(variables, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		if dataType == jsonparser.Null {
			stringKey := unsafebytes.BytesToString(key)
			if removeNullVariables || slices.Contains(undefinedVariables, stringKey) {
				cp = jsonparser.Delete(cp, stringKey)
			}
		}
		return nil
	})
	if err != nil {
		return variables
	}

	// remove empty objects
	if removeNullVariables {
		cp = s.removeEmptyObjects(cp)
	}

	return cp
}

// removeEmptyObjects removes empty objects from JSON: {"b": "b", "c": {}} -> {"b": "b"}
func (s *Source) removeEmptyObjects(variables []byte) []byte {
	var changed bool
	for {
		variables, changed = s.replaceEmptyObject(variables)
		if !changed {
			break
		}
	}
	return variables
}

func (s *Source) replaceEmptyObject(variables []byte) ([]byte, bool) {
	if i := bytes.Index(variables, []byte(":{}")); i != -1 {
		end := i + 3
		hasTrailingComma := false
		if variables[end] == ',' {
			end++
			hasTrailingComma = true
		}
		startQuote := bytes.LastIndex(variables[:i-2], []byte("\""))
		if !hasTrailingComma && variables[startQuote-1] == ',' {
			startQuote--
		}
		return append(variables[:startQuote], variables[end:]...), true
	}

	return variables, false
}

func (s *Source) Load(ctx context.Context, input []byte, writer io.Writer) (err error) {
	input = s.compactAndUnNullVariables(input)
	return httpclient.Do(s.httpClient, ctx, input, writer)
}

type GraphQLSubscriptionClient interface {
	Subscribe(ctx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error
	UniqueRequestID(ctx *resolve.Context, options GraphQLSubscriptionOptions, hash *xxhash.Digest) (err error)
}

type GraphQLSubscriptionOptions struct {
	URL                                     string           `json:"url"`
	InitialPayload                          json.RawMessage  `json:"initial_payload"`
	Body                                    GraphQLBody      `json:"body"`
	Header                                  http.Header      `json:"header"`
	UseSSE                                  bool             `json:"use_sse"`
	SSEMethodPost                           bool             `json:"sse_method_post"`
	ForwardedClientHeaderNames              []string         `json:"forwarded_client_header_names"`
	ForwardedClientHeaderRegularExpressions []*regexp.Regexp `json:"forwarded_client_header_regular_expressions"`
	WsSubProtocol                           string           `json:"ws_sub_protocol"`
}

type GraphQLBody struct {
	Query         string          `json:"query,omitempty"`
	OperationName string          `json:"operationName,omitempty"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Extensions    json.RawMessage `json:"extensions,omitempty"`
}

type SubscriptionSource struct {
	client GraphQLSubscriptionClient
}

func (s *SubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	var options GraphQLSubscriptionOptions
	err := json.Unmarshal(input, &options)
	if err != nil {
		return err
	}
	if options.Body.Query == "" {
		return resolve.ErrUnableToResolve
	}
	return s.client.Subscribe(ctx, options, updater)
}

var (
	dataSouceName = []byte("graphql")
)

func (s *SubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) (err error) {
	_, err = xxh.Write(dataSouceName)
	if err != nil {
		return err
	}
	var options GraphQLSubscriptionOptions
	err = json.Unmarshal(input, &options)
	if err != nil {
		return err
	}
	return s.client.UniqueRequestID(ctx, options, xxh)
}
