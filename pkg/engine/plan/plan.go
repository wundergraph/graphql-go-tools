package plan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type Planner struct {
	config                Configuration
	configurationWalker   *astvisitor.Walker
	configurationVisitor  *configurationVisitor
	planningWalker        *astvisitor.Walker
	planningVisitor       *Visitor
	requiredFieldsWalker  *astvisitor.Walker
	requiredFieldsVisitor *requiredFieldsVisitor
}

type Configuration struct {
	DefaultFlushIntervalMillis int64
	DataSources                []DataSourceConfiguration
	Fields                     FieldConfigurations
	Types                      TypeConfigurations
	// DisableResolveFieldPositions should be set to true for testing purposes
	// This setting removes position information from all fields
	// In production, this should be set to false so that error messages are easier to understand
	DisableResolveFieldPositions bool
	CustomResolveMap             map[string]resolve.CustomResolve
}

type DirectiveConfigurations []DirectiveConfiguration

func (d *DirectiveConfigurations) RenameTypeNameOnMatchStr(directiveName string) string {
	for i := range *d {
		if (*d)[i].DirectiveName == directiveName {
			return (*d)[i].RenameTo
		}
	}
	return directiveName
}

func (d *DirectiveConfigurations) RenameTypeNameOnMatchBytes(directiveName []byte) []byte {
	str := string(directiveName)
	for i := range *d {
		if (*d)[i].DirectiveName == str {
			return []byte((*d)[i].RenameTo)
		}
	}
	return directiveName
}

type DirectiveConfiguration struct {
	DirectiveName string
	RenameTo      string
}

type TypeConfigurations []TypeConfiguration

func (t *TypeConfigurations) RenameTypeNameOnMatchStr(typeName string) string {
	for i := range *t {
		if (*t)[i].TypeName == typeName {
			return (*t)[i].RenameTo
		}
	}
	return typeName
}

func (t *TypeConfigurations) RenameTypeNameOnMatchBytes(typeName []byte) []byte {
	str := string(typeName)
	for i := range *t {
		if (*t)[i].TypeName == str {
			return []byte((*t)[i].RenameTo)
		}
	}
	return typeName
}

type TypeConfiguration struct {
	TypeName string
	// RenameTo modifies the TypeName
	// so that a downstream Operation can contain a different TypeName than the upstream Schema
	// e.g. if the downstream Operation contains { ... on Human_api { height } }
	// the upstream Operation can be rewritten to { ... on Human { height }}
	// by setting RenameTo to Human
	// This way, Types can be suffixed / renamed in downstream Schemas while keeping the contract with the upstream ok
	RenameTo string
}

type FieldConfigurations []FieldConfiguration

func (f FieldConfigurations) ForTypeField(typeName, fieldName string) *FieldConfiguration {
	for i := range f {
		if f[i].TypeName == typeName && f[i].FieldName == fieldName {
			return &f[i]
		}
	}
	return nil
}

type FieldConfiguration struct {
	TypeName  string
	FieldName string
	// DisableDefaultMapping - instructs planner whether to use path mapping coming from Path field
	DisableDefaultMapping bool
	// Path - represents a json path to lookup for a field value in response json
	Path           []string
	Arguments      ArgumentsConfigurations
	RequiresFields []string
	// UnescapeResponseJson set to true will allow fields (String,List,Object)
	// to be resolved from an escaped JSON string
	// e.g. {"response":"{\"foo\":\"bar\"}"} will be returned as {"foo":"bar"} when path is "response"
	// This way, it is possible to resolve a JSON string as part of the response without extra String encoding of the JSON
	UnescapeResponseJson bool
}

type ArgumentsConfigurations []ArgumentConfiguration

func (a ArgumentsConfigurations) ForName(argName string) *ArgumentConfiguration {
	for i := range a {
		if a[i].Name == argName {
			return &a[i]
		}
	}
	return nil
}

type SourceType string
type ArgumentRenderConfig string

const (
	ObjectFieldSource            SourceType           = "object_field"
	FieldArgumentSource          SourceType           = "field_argument"
	RenderArgumentDefault        ArgumentRenderConfig = ""
	RenderArgumentAsArrayCSV     ArgumentRenderConfig = "render_argument_as_array_csv"
	RenderArgumentAsGraphQLValue ArgumentRenderConfig = "render_argument_as_graphql_value"
	RenderArgumentAsJSONValue    ArgumentRenderConfig = "render_argument_as_json_value"
)

type ArgumentConfiguration struct {
	Name         string
	SourceType   SourceType
	SourcePath   []string
	RenderConfig ArgumentRenderConfig
	RenameTypeTo string
}

type DataSourceConfiguration struct {
	// RootNodes - defines the nodes where the responsibility of the DataSource begins
	// When you enter a node, and it is not a child node
	// when you have entered into a field which representing data source - it means that we starting a new planning stage
	RootNodes []TypeField
	// ChildNodes - describes additional fields which will be requested along with fields which has a datasources
	// They are always required for the Graphql datasources cause each field could have it's own datasource
	// For any single point datasource like HTTP/REST or GRPC we could not request less fields, as we always get a full response
	ChildNodes []TypeField
	Directives DirectiveConfigurations
	Factory    PlannerFactory
	Custom     json.RawMessage
}

func (d *DataSourceConfiguration) HasRootNode(typeName, fieldName string) bool {
	for i := range d.RootNodes {
		if typeName != d.RootNodes[i].TypeName {
			continue
		}
		for j := range d.RootNodes[i].FieldNames {
			if fieldName == d.RootNodes[i].FieldNames[j] {
				return true
			}
		}
	}
	return false
}

type PlannerFactory interface {
	// Planner should return the DataSourcePlanner
	// closer is the closing channel for all stateful DataSources
	// At runtime, the Execution Engine will be instantiated with one global resolve.Closer.
	// Once the Closer gets closed, all stateful DataSources must close their connections and cleanup themselves.
	// They can do so by starting a goroutine on instantiation time that blocking reads on the resolve.Closer.
	// Once the Closer emits the close event, they have to terminate (e.g. close database connections).
	Planner(ctx context.Context) DataSourcePlanner
}

type TypeField struct {
	TypeName   string
	FieldNames []string
}

type FieldMapping struct {
	TypeName              string
	FieldName             string
	DisableDefaultMapping bool
	Path                  []string
}

// NewPlanner creates a new Planner from the Configuration and a ctx object
// The context.Context object is used to determine the lifecycle of stateful DataSources
// It's important to note that stateful DataSources must be closed when they are no longer being used
// Stateful DataSources could be those that initiate a WebSocket connection to an origin, a database client, a streaming client, etc...
// All DataSources are initiated with the same context.Context object provided to the Planner.
// To ensure that there are no memory leaks, it's therefore important to add a cancel func or timeout to the context.Context
// At the time when the resolver and all operations should be garbage collected, ensure to first cancel or timeout the ctx object
// If you don't cancel the context.Context, the goroutines will run indefinitely and there's no reference left to stop them
func NewPlanner(ctx context.Context, config Configuration) *Planner {

	// required fields pre-processing

	requiredFieldsWalker := astvisitor.NewWalker(48)
	requiredFieldsV := &requiredFieldsVisitor{
		walker: &requiredFieldsWalker,
	}

	requiredFieldsWalker.RegisterEnterDocumentVisitor(requiredFieldsV)
	requiredFieldsWalker.RegisterEnterOperationVisitor(requiredFieldsV)
	requiredFieldsWalker.RegisterEnterFieldVisitor(requiredFieldsV)
	requiredFieldsWalker.RegisterLeaveDocumentVisitor(requiredFieldsV)

	// configuration

	configurationWalker := astvisitor.NewWalker(48)
	configVisitor := &configurationVisitor{
		walker: &configurationWalker,
		ctx:    ctx,
	}

	configurationWalker.RegisterEnterDocumentVisitor(configVisitor)
	configurationWalker.RegisterFieldVisitor(configVisitor)
	configurationWalker.RegisterEnterOperationVisitor(configVisitor)
	configurationWalker.RegisterSelectionSetVisitor(configVisitor)

	// planning

	planningWalker := astvisitor.NewWalker(48)
	planningVisitor := &Visitor{
		Walker:                       &planningWalker,
		fieldConfigs:                 map[int]*FieldConfiguration{},
		disableResolveFieldPositions: config.DisableResolveFieldPositions,
	}

	p := &Planner{
		config:                config,
		configurationWalker:   &configurationWalker,
		configurationVisitor:  configVisitor,
		planningWalker:        &planningWalker,
		planningVisitor:       planningVisitor,
		requiredFieldsWalker:  &requiredFieldsWalker,
		requiredFieldsVisitor: requiredFieldsV,
	}

	return p
}

func (p *Planner) SetConfig(config Configuration) {
	p.config = config
}

func (p *Planner) Plan(operation, definition *ast.Document, operationName string, report *operationreport.Report) (plan Plan) {

	// make a copy of the config as the pre-processor modifies it

	config := p.config

	// clean objects and current fields from previous invocation
	p.planningVisitor.objects = p.planningVisitor.objects[:0]
	p.planningVisitor.currentFields = p.planningVisitor.currentFields[:0]

	// select operation

	p.selectOperation(operation, operationName, report)
	if report.HasErrors() {
		return
	}

	// pre-process required fields

	p.preProcessRequiredFields(&config, operation, definition, report)

	// find planning paths

	p.configurationVisitor.config = config
	p.configurationWalker.Walk(operation, definition, report)

	// configure planning visitor

	p.planningVisitor.planners = p.configurationVisitor.planners
	p.planningVisitor.Config = config
	p.planningVisitor.fetchConfigurations = p.configurationVisitor.fetches
	p.planningVisitor.fieldBuffers = p.configurationVisitor.fieldBuffers
	p.planningVisitor.skipFieldPaths = p.requiredFieldsVisitor.skipFieldPaths

	p.planningWalker.ResetVisitors()
	p.planningWalker.SetVisitorFilter(p.planningVisitor)
	p.planningWalker.RegisterDocumentVisitor(p.planningVisitor)
	p.planningWalker.RegisterEnterOperationVisitor(p.planningVisitor)
	p.planningWalker.RegisterFieldVisitor(p.planningVisitor)
	p.planningWalker.RegisterSelectionSetVisitor(p.planningVisitor)
	p.planningWalker.RegisterEnterDirectiveVisitor(p.planningVisitor)
	p.planningWalker.RegisterInlineFragmentVisitor(p.planningVisitor)

	for key := range p.planningVisitor.planners {
		config := p.planningVisitor.planners[key].dataSourceConfiguration
		isNested := p.planningVisitor.planners[key].isNestedPlanner()
		err := p.planningVisitor.planners[key].planner.Register(p.planningVisitor, config, isNested)
		if err != nil {
			report.AddInternalError(err)
			return
		}
	}

	// process the plan

	p.planningWalker.Walk(operation, definition, report)

	return p.planningVisitor.plan
}

func (p *Planner) selectOperation(operation *ast.Document, operationName string, report *operationreport.Report) {

	numOfOperations := operation.NumOfOperationDefinitions()
	operationName = strings.TrimSpace(operationName)
	if len(operationName) == 0 && numOfOperations > 1 {
		report.AddExternalError(operationreport.ErrRequiredOperationNameIsMissing())
		return
	}

	if len(operationName) == 0 && numOfOperations == 1 {
		operationName = operation.OperationDefinitionNameString(0)
	}

	if !operation.OperationNameExists(operationName) {
		report.AddExternalError(operationreport.ErrOperationWithProvidedOperationNameNotFound(operationName))
		return
	}

	p.requiredFieldsVisitor.operationName = operationName
	p.configurationVisitor.operationName = operationName
	p.planningVisitor.OperationName = operationName
}

func (p *Planner) preProcessRequiredFields(config *Configuration, operation, definition *ast.Document, report *operationreport.Report) {
	if !p.hasRequiredFields(config) {
		return
	}

	p.requiredFieldsVisitor.config = config
	p.requiredFieldsVisitor.operation = operation
	p.requiredFieldsVisitor.definition = definition
	p.requiredFieldsWalker.Walk(operation, definition, report)
}

func (p *Planner) hasRequiredFields(config *Configuration) bool {
	for i := range config.Fields {
		if len(config.Fields[i].RequiresFields) != 0 {
			return true
		}
	}
	return false
}

type Visitor struct {
	Operation, Definition        *ast.Document
	Walker                       *astvisitor.Walker
	Importer                     astimport.Importer
	Config                       Configuration
	plan                         Plan
	OperationName                string
	operationDefinition          int
	objects                      []*resolve.Object
	currentFields                []objectFields
	currentField                 *resolve.Field
	planners                     []plannerConfiguration
	fetchConfigurations          []objectFetchConfiguration
	fieldBuffers                 map[int]int
	skipFieldPaths               []string
	fieldConfigs                 map[int]*FieldConfiguration
	exportedVariables            map[string]struct{}
	skipIncludeFields            map[int]skipIncludeField
	disableResolveFieldPositions bool
}

type skipIncludeField struct {
	skip                bool
	skipVariableName    string
	include             bool
	includeVariableName string
}

type objectFields struct {
	popOnField int
	fields     *[]*resolve.Field
}

type objectFetchConfiguration struct {
	object             *resolve.Object
	trigger            *resolve.GraphQLSubscriptionTrigger
	planner            DataSourcePlanner
	bufferID           int
	isSubscription     bool
	fieldRef           int
	fieldDefinitionRef int
}

func (v *Visitor) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor interface{}) bool {
	if visitor == v {
		return true
	}
	path := v.Walker.Path.DotDelimitedString()
	switch kind {
	case astvisitor.EnterField, astvisitor.LeaveField:
		fieldAliasOrName := v.Operation.FieldAliasOrNameString(ref)
		path = path + "." + fieldAliasOrName
	}
	if !strings.Contains(path, ".") {
		return true
	}
	for _, config := range v.planners {
		if config.planner == visitor && config.hasPath(path) {
			switch kind {
			case astvisitor.EnterField, astvisitor.LeaveField:
				return config.shouldWalkFieldsOnPath(path)
			case astvisitor.EnterSelectionSet, astvisitor.LeaveSelectionSet:
				return !config.isExitPath(path)
			default:
				return true
			}
		}
	}
	return false
}

func (v *Visitor) currentFullPath() string {
	path := v.Walker.Path.DotDelimitedString()
	if v.Walker.CurrentKind == ast.NodeKindField {
		fieldAliasOrName := v.Operation.FieldAliasOrNameString(v.Walker.CurrentRef)
		path = path + "." + fieldAliasOrName
	}
	return path
}

func (v *Visitor) EnterDirective(ref int) {
	directiveName := v.Operation.DirectiveNameString(ref)
	ancestor := v.Walker.Ancestors[len(v.Walker.Ancestors)-1]
	switch ancestor.Kind {
	case ast.NodeKindOperationDefinition:
		switch directiveName {
		case "flushInterval":
			if value, ok := v.Operation.DirectiveArgumentValueByName(ref, literal.MILLISECONDS); ok {
				if value.Kind == ast.ValueKindInteger {
					v.plan.SetFlushInterval(v.Operation.IntValueAsInt(value.Ref))
				}
			}
		}
	case ast.NodeKindField:
		switch directiveName {
		case "stream":
			initialBatchSize := 0
			if value, ok := v.Operation.DirectiveArgumentValueByName(ref, literal.INITIAL_BATCH_SIZE); ok {
				if value.Kind == ast.ValueKindInteger {
					initialBatchSize = int(v.Operation.IntValueAsInt32(value.Ref))
				}
			}
			v.currentField.Stream = &resolve.StreamField{
				InitialBatchSize: initialBatchSize,
			}
		case "defer":
			v.currentField.Defer = &resolve.DeferField{}
		}
	}
}

func (v *Visitor) EnterInlineFragment(ref int) {
	directives := v.Operation.InlineFragments[ref].Directives.Refs
	skip, skipVariableName := v.resolveSkip(directives)
	include, includeVariableName := v.resolveInclude(directives)
	set := v.Operation.InlineFragments[ref].SelectionSet
	if set == -1 {
		return
	}
	for _, selection := range v.Operation.SelectionSets[set].SelectionRefs {
		switch v.Operation.Selections[selection].Kind {
		case ast.SelectionKindField:
			ref := v.Operation.Selections[selection].Ref
			if skip || include {
				v.skipIncludeFields[ref] = skipIncludeField{
					skip:                skip,
					skipVariableName:    skipVariableName,
					include:             include,
					includeVariableName: includeVariableName,
				}
			}
		}
	}
}

func (v *Visitor) LeaveInlineFragment(_ int) {

}

func (v *Visitor) EnterSelectionSet(_ int) {

}

func (v *Visitor) LeaveSelectionSet(_ int) {

}

func (v *Visitor) EnterField(ref int) {
	if v.skipField(ref) {
		return
	}

	skip, skipVariableName := v.resolveSkipForField(ref)
	include, includeVariableName := v.resolveIncludeForField(ref)

	fieldName := v.Operation.FieldNameBytes(ref)
	fieldAliasOrName := v.Operation.FieldAliasOrNameBytes(ref)
	if bytes.Equal(fieldName, literal.TYPENAME) {
		v.currentField = &resolve.Field{
			Name: fieldAliasOrName,
			Value: &resolve.String{
				Nullable:   false,
				Path:       []string{"__typename"},
				IsTypeName: true,
			},
			OnTypeNames:             v.resolveOnTypeNames(),
			Position:                v.resolveFieldPosition(ref),
			SkipDirectiveDefined:    skip,
			SkipVariableName:        skipVariableName,
			IncludeDirectiveDefined: include,
			IncludeVariableName:     includeVariableName,
		}
		*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, v.currentField)
		return
	}

	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}

	var (
		hasFetchConfig bool
		i              int
	)
	for i = range v.fetchConfigurations {
		if ref == v.fetchConfigurations[i].fieldRef {
			hasFetchConfig = true
			break
		}
	}
	if hasFetchConfig {
		if v.fetchConfigurations[i].isSubscription {
			plan, ok := v.plan.(*SubscriptionResponsePlan)
			if ok {
				v.fetchConfigurations[i].trigger = &plan.Response.Trigger
			}
		} else {
			v.fetchConfigurations[i].object = v.objects[len(v.objects)-1]
		}
	}

	path := v.resolveFieldPath(ref)
	fieldDefinitionType := v.Definition.FieldDefinitionType(fieldDefinition)
	bufferID, hasBuffer := v.fieldBuffers[ref]

	v.currentField = &resolve.Field{
		Name:                    fieldAliasOrName,
		Value:                   v.resolveFieldValue(ref, fieldDefinitionType, true, path),
		HasBuffer:               hasBuffer,
		BufferID:                bufferID,
		OnTypeNames:             v.resolveOnTypeNames(),
		Position:                v.resolveFieldPosition(ref),
		SkipDirectiveDefined:    skip,
		SkipVariableName:        skipVariableName,
		IncludeDirectiveDefined: include,
		IncludeVariableName:     includeVariableName,
	}

	*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, v.currentField)

	typeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldNameStr := v.Operation.FieldNameString(ref)
	fieldConfig := v.Config.Fields.ForTypeField(typeName, fieldNameStr)
	if fieldConfig == nil {
		return
	}
	v.fieldConfigs[ref] = fieldConfig
}

func (v *Visitor) resolveFieldPosition(ref int) resolve.Position {
	if v.disableResolveFieldPositions {
		return resolve.Position{}
	}
	return resolve.Position{
		Line:   v.Operation.Fields[ref].Position.LineStart,
		Column: v.Operation.Fields[ref].Position.CharStart,
	}
}

func (v *Visitor) resolveSkipForField(ref int) (bool, string) {
	skipInclude, ok := v.skipIncludeFields[ref]
	if ok {
		return skipInclude.skip, skipInclude.skipVariableName
	}
	return v.resolveSkip(v.Operation.Fields[ref].Directives.Refs)
}

func (v *Visitor) resolveIncludeForField(ref int) (bool, string) {
	skipInclude, ok := v.skipIncludeFields[ref]
	if ok {
		return skipInclude.include, skipInclude.includeVariableName
	}
	return v.resolveInclude(v.Operation.Fields[ref].Directives.Refs)
}

func (v *Visitor) resolveSkip(directiveRefs []int) (bool, string) {
	for _, i := range directiveRefs {
		if v.Operation.DirectiveNameString(i) != "skip" {
			continue
		}
		if value, ok := v.Operation.DirectiveArgumentValueByName(i, literal.IF); ok {
			if value.Kind == ast.ValueKindVariable {
				return true, v.Operation.VariableValueNameString(value.Ref)
			}
		}
	}
	return false, ""
}

func (v *Visitor) resolveInclude(directiveRefs []int) (bool, string) {
	for _, i := range directiveRefs {
		if v.Operation.DirectiveNameString(i) != "include" {
			continue
		}
		if value, ok := v.Operation.DirectiveArgumentValueByName(i, literal.IF); ok {
			if value.Kind == ast.ValueKindVariable {
				return true, v.Operation.VariableValueNameString(value.Ref)
			}
		}
	}
	return false, ""
}

func (v *Visitor) resolveOnTypeNames() [][]byte {
	if len(v.Walker.Ancestors) < 2 {
		return nil
	}
	inlineFragment := v.Walker.Ancestors[len(v.Walker.Ancestors)-2]
	if inlineFragment.Kind != ast.NodeKindInlineFragment {
		return nil
	}
	typeName := v.Operation.InlineFragmentTypeConditionName(inlineFragment.Ref)
	if typeName == nil {
		typeName = v.Walker.EnclosingTypeDefinition.NameBytes(v.Definition)
	}
	node, exists := v.Definition.NodeByName(typeName)
	// If not an interface, return the concrete type
	if !exists || !node.Kind.IsAbstractType() {
		return [][]byte{v.Config.Types.RenameTypeNameOnMatchBytes(typeName)}
	}
	if node.Kind == ast.NodeKindUnionTypeDefinition {
		// This should never be true. If it is, it's an error
		panic("resolveOnTypeNames called with a union type")
	}
	// We're dealing with an interface, so add all objects that implement this interface to the slice
	onTypeNames := make([][]byte, 0, 2)
	for objectTypeDefinitionRef := range v.Definition.ObjectTypeDefinitions {
		if v.Definition.ObjectTypeDefinitionImplementsInterface(objectTypeDefinitionRef, typeName) {
			onTypeNames = append(onTypeNames, v.Definition.ObjectTypeDefinitionNameBytes(objectTypeDefinitionRef))
		}
	}
	if len(onTypeNames) < 1 {
		return nil
	}
	return onTypeNames
}

func (v *Visitor) LeaveField(ref int) {
	if v.currentFields[len(v.currentFields)-1].popOnField == ref {
		v.currentFields = v.currentFields[:len(v.currentFields)-1]
	}
	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}
	fieldDefinitionTypeNode := v.Definition.FieldDefinitionTypeNode(fieldDefinition)
	switch fieldDefinitionTypeNode.Kind {
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
		v.objects = v.objects[:len(v.objects)-1]
	}
}

func (v *Visitor) skipField(ref int) bool {
	fullPath := v.Walker.Path.DotDelimitedString() + "." + v.Operation.FieldAliasOrNameString(ref)
	for i := range v.skipFieldPaths {
		if v.skipFieldPaths[i] == fullPath {
			return true
		}
	}
	return false
}

func (v *Visitor) resolveFieldValue(fieldRef, typeRef int, nullable bool, path []string) resolve.Node {
	ofType := v.Definition.Types[typeRef].OfType

	fieldName := v.Operation.FieldNameString(fieldRef)
	enclosingTypeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldConfig := v.Config.Fields.ForTypeField(enclosingTypeName, fieldName)
	unescapeResponseJson := false
	if fieldConfig != nil {
		unescapeResponseJson = fieldConfig.UnescapeResponseJson
	}

	switch v.Definition.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		return v.resolveFieldValue(fieldRef, ofType, false, path)
	case ast.TypeKindList:
		listItem := v.resolveFieldValue(fieldRef, ofType, true, nil)
		return &resolve.Array{
			Nullable: nullable,
			Path:     path,
			Item:     listItem,
		}
	case ast.TypeKindNamed:
		typeName := v.Definition.ResolveTypeNameString(typeRef)
		customResolve, ok := v.Config.CustomResolveMap[typeName]
		if ok {
			return &resolve.CustomNode{
				CustomResolve: customResolve,
				Path:          path,
				Nullable:      nullable,
			}
		}
		typeDefinitionNode, ok := v.Definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			return &resolve.Null{}
		}
		switch typeDefinitionNode.Kind {
		case ast.NodeKindScalarTypeDefinition:
			fieldExport := v.resolveFieldExport(fieldRef)
			switch typeName {
			case "String":
				return &resolve.String{
					Path:                 path,
					Nullable:             nullable,
					Export:               fieldExport,
					UnescapeResponseJson: unescapeResponseJson,
				}
			case "Boolean":
				return &resolve.Boolean{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			case "Int":
				return &resolve.Integer{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			case "Float":
				return &resolve.Float{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			case "BigInt":
				return &resolve.BigInt{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			default:
				return &resolve.String{
					Path:                 path,
					Nullable:             nullable,
					Export:               fieldExport,
					UnescapeResponseJson: unescapeResponseJson,
				}
			}
		case ast.NodeKindEnumTypeDefinition:
			return &resolve.String{
				Path:                 path,
				Nullable:             nullable,
				UnescapeResponseJson: unescapeResponseJson,
			}
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
			object := &resolve.Object{
				Nullable:             nullable,
				Path:                 path,
				Fields:               []*resolve.Field{},
				UnescapeResponseJson: unescapeResponseJson,
			}
			v.objects = append(v.objects, object)
			v.Walker.Defer(func() {
				v.currentFields = append(v.currentFields, objectFields{
					popOnField: fieldRef,
					fields:     &object.Fields,
				})
			})
			return object
		default:
			return &resolve.Null{}
		}
	default:
		return &resolve.Null{}
	}
}

func (v *Visitor) resolveFieldExport(fieldRef int) *resolve.FieldExport {
	if !v.Operation.Fields[fieldRef].HasDirectives {
		return nil
	}
	exportAs := ""
	for _, ref := range v.Operation.Fields[fieldRef].Directives.Refs {
		if v.Operation.Input.ByteSliceString(v.Operation.Directives[ref].Name) == "export" {
			value, ok := v.Operation.DirectiveArgumentValueByName(ref, []byte("as"))
			if !ok {
				continue
			}
			if value.Kind != ast.ValueKindString {
				continue
			}
			exportAs = v.Operation.StringValueContentString(value.Ref)
		}
	}
	if exportAs == "" {
		return nil
	}
	variableDefinition, ok := v.Operation.VariableDefinitionByNameAndOperation(v.Walker.Ancestors[0].Ref, []byte(exportAs))
	if !ok {
		return nil
	}
	v.exportedVariables[exportAs] = struct{}{}

	typeName := v.Operation.ResolveTypeNameString(v.Operation.VariableDefinitions[variableDefinition].Type)
	switch typeName {
	case "Int", "Float", "Boolean":
		return &resolve.FieldExport{
			Path: []string{exportAs},
		}
	default:
		return &resolve.FieldExport{
			Path:     []string{exportAs},
			AsString: true,
		}
	}
}

func (v *Visitor) fieldRequiresExportedVariable(fieldRef int) bool {
	for _, arg := range v.Operation.Fields[fieldRef].Arguments.Refs {
		if v.valueRequiresExportedVariable(v.Operation.Arguments[arg].Value) {
			return true
		}
	}
	return false
}

func (v *Visitor) valueRequiresExportedVariable(value ast.Value) bool {
	switch value.Kind {
	case ast.ValueKindVariable:
		name := v.Operation.VariableValueNameString(value.Ref)
		if _, ok := v.exportedVariables[name]; ok {
			return true
		}
		return false
	case ast.ValueKindList:
		for _, ref := range v.Operation.ListValues[value.Ref].Refs {
			if v.valueRequiresExportedVariable(v.Operation.Values[ref]) {
				return true
			}
		}
		return false
	case ast.ValueKindObject:
		for _, ref := range v.Operation.ObjectValues[value.Ref].Refs {
			if v.valueRequiresExportedVariable(v.Operation.ObjectFieldValue(ref)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (v *Visitor) EnterOperationDefinition(ref int) {
	operationName := v.Operation.OperationDefinitionNameString(ref)
	if v.OperationName != operationName {
		v.Walker.SkipNode()
		return
	}

	v.operationDefinition = ref

	rootObject := &resolve.Object{
		Fields: []*resolve.Field{},
	}

	v.objects = append(v.objects, rootObject)
	v.currentFields = append(v.currentFields, objectFields{
		fields:     &rootObject.Fields,
		popOnField: -1,
	})

	isSubscription, _, err := AnalyzePlanKind(v.Operation, v.Definition, v.OperationName)
	if err != nil {
		v.Walker.StopWithInternalErr(err)
		return
	}

	graphQLResponse := &resolve.GraphQLResponse{
		Data: rootObject,
	}

	if isSubscription {
		v.plan = &SubscriptionResponsePlan{
			FlushInterval: v.Config.DefaultFlushIntervalMillis,
			Response: &resolve.GraphQLSubscription{
				Response: graphQLResponse,
			},
		}
		return
	}

	/*if isStreaming {

	}*/

	v.plan = &SynchronousResponsePlan{
		Response: graphQLResponse,
	}
}

func (v *Visitor) resolveFieldPath(ref int) []string {
	typeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldName := v.Operation.FieldNameUnsafeString(ref)
	config := v.currentOrParentPlannerConfiguration()
	aliasOverride := false
	if config.planner != nil {
		aliasOverride = config.planner.DataSourcePlanningBehavior().OverrideFieldPathFromAlias
	}

	for i := range v.Config.Fields {
		if v.Config.Fields[i].TypeName == typeName && v.Config.Fields[i].FieldName == fieldName {
			if aliasOverride {
				override, exists := config.planner.DownstreamResponseFieldAlias(ref)
				if exists {
					return []string{override}
				}
			}
			if aliasOverride && v.Operation.FieldAliasIsDefined(ref) {
				return []string{v.Operation.FieldAliasString(ref)}
			}
			if v.Config.Fields[i].DisableDefaultMapping {
				return nil
			}
			if len(v.Config.Fields[i].Path) != 0 {
				return v.Config.Fields[i].Path
			}
			return []string{fieldName}
		}
	}

	if aliasOverride {
		return []string{v.Operation.FieldAliasOrNameString(ref)}
	}

	return []string{fieldName}
}

func (v *Visitor) EnterDocument(operation, definition *ast.Document) {
	v.Operation, v.Definition = operation, definition
	v.fieldConfigs = map[int]*FieldConfiguration{}
	v.exportedVariables = map[string]struct{}{}
	v.skipIncludeFields = map[int]skipIncludeField{}
}

func (v *Visitor) LeaveDocument(_, _ *ast.Document) {
	for _, config := range v.fetchConfigurations {
		if config.isSubscription {
			v.configureSubscription(config)
		} else {
			v.configureObjectFetch(config)
		}
	}
}

var (
	templateRegex = regexp.MustCompile(`{{.*?}}`)
	selectorRegex = regexp.MustCompile(`{{\s*\.(.*?)\s*}}`)
)

func (v *Visitor) currentOrParentPlannerConfiguration() plannerConfiguration {
	const none = -1
	currentPath := v.currentFullPath()
	plannerIndex := none
	plannerPathDeepness := none

	for i := range v.planners {
		for _, plannerPath := range v.planners[i].paths {
			if v.isCurrentOrParentPath(currentPath, plannerPath.path) {
				currentPlannerPathDeepness := v.pathDeepness(plannerPath.path)
				if currentPlannerPathDeepness > plannerPathDeepness {
					plannerPathDeepness = currentPlannerPathDeepness
					plannerIndex = i
					break
				}
			}
		}
	}

	if plannerIndex != none {
		return v.planners[plannerIndex]
	}

	return plannerConfiguration{}
}

func (v *Visitor) isCurrentOrParentPath(currentPath string, parentPath string) bool {
	return strings.HasPrefix(currentPath, parentPath)
}

func (v *Visitor) pathDeepness(path string) int {
	return strings.Count(path, ".")
}

func (v *Visitor) resolveInputTemplates(config objectFetchConfiguration, input *string, variables *resolve.Variables) {
	*input = templateRegex.ReplaceAllStringFunc(*input, func(s string) string {
		selectors := selectorRegex.FindStringSubmatch(s)
		if len(selectors) != 2 {
			return s
		}
		selector := strings.TrimPrefix(selectors[1], ".")
		parts := strings.Split(selector, ".")
		if len(parts) < 2 {
			return s
		}
		path := parts[1:]
		var (
			variableName string
		)
		switch parts[0] {
		case "object":
			variable := &resolve.ObjectVariable{
				Path:     path,
				Renderer: resolve.NewPlainVariableRenderer(),
			}
			variableName, _ = variables.AddVariable(variable)
		case "arguments":
			argumentName := path[0]
			arg, ok := v.Operation.FieldArgument(config.fieldRef, []byte(argumentName))
			if !ok {
				break
			}
			value := v.Operation.ArgumentValue(arg)
			if value.Kind != ast.ValueKindVariable {
				inputValueDefinition := -1
				for _, ref := range v.Definition.FieldDefinitions[config.fieldDefinitionRef].ArgumentsDefinition.Refs {
					inputFieldName := v.Definition.Input.ByteSliceString(v.Definition.InputValueDefinitions[ref].Name)
					if inputFieldName == argumentName {
						inputValueDefinition = ref
						break
					}
				}
				if inputValueDefinition == -1 {
					return "null"
				}
				return v.renderJSONValueTemplate(value, variables, inputValueDefinition)
			}
			variableValue := v.Operation.VariableValueNameString(value.Ref)
			if !v.Operation.OperationDefinitionHasVariableDefinition(v.operationDefinition, variableValue) {
				break // omit optional argument when variable is not defined
			}
			variableDefinition, exists := v.Operation.VariableDefinitionByNameAndOperation(v.operationDefinition, v.Operation.VariableValueNameBytes(value.Ref))
			if !exists {
				break
			}
			variableTypeRef := v.Operation.VariableDefinitions[variableDefinition].Type
			typeName := v.Operation.ResolveTypeNameBytes(v.Operation.VariableDefinitions[variableDefinition].Type)
			node, exists := v.Definition.Index.FirstNodeByNameBytes(typeName)
			if !exists {
				break
			}

			var variablePath []string
			if len(parts) > 2 && node.Kind == ast.NodeKindInputObjectTypeDefinition {
				variablePath = append(variablePath, path...)
			} else {
				variablePath = append(variablePath, variableValue)
			}

			variable := &resolve.ContextVariable{
				Path: variablePath,
			}

			if fieldConfig, ok := v.fieldConfigs[config.fieldRef]; ok {
				if argumentConfig := fieldConfig.Arguments.ForName(argumentName); argumentConfig != nil {
					switch argumentConfig.RenderConfig {
					case RenderArgumentAsArrayCSV:
						variable.Renderer = resolve.NewCSVVariableRendererFromTypeRef(v.Operation, v.Definition, variableTypeRef)
					case RenderArgumentDefault:
						renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(v.Operation, v.Definition, variableTypeRef, variablePath...)
						if err != nil {
							break
						}
						variable.Renderer = renderer
					case RenderArgumentAsGraphQLValue:
						renderer, err := resolve.NewGraphQLVariableRendererFromTypeRef(v.Operation, v.Definition, variableTypeRef)
						if err != nil {
							break
						}
						variable.Renderer = renderer
					case RenderArgumentAsJSONValue:
						renderer, err := resolve.NewJSONVariableRendererWithValidationFromTypeRef(v.Operation, v.Definition, variableTypeRef)
						if err != nil {
							break
						}
						variable.Renderer = renderer
					}
				}
			}

			if variable.Renderer == nil {
				renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(v.Operation, v.Definition, variableTypeRef, variablePath...)
				if err != nil {
					break
				}
				variable.Renderer = renderer
			}

			variableName, _ = variables.AddVariable(variable)
		case "request":
			if len(path) != 2 {
				break
			}
			switch path[0] {
			case "headers":
				key := path[1]
				variableName, _ = variables.AddVariable(&resolve.HeaderVariable{
					Path: []string{key},
				})
			}
		}
		return variableName
	})
}

func (v *Visitor) renderJSONValueTemplate(value ast.Value, variables *resolve.Variables, inputValueDefinition int) (out string) {
	switch value.Kind {
	case ast.ValueKindList:
		out += "["
		addComma := false
		for _, ref := range v.Operation.ListValues[value.Ref].Refs {
			if addComma {
				out += ","
			} else {
				addComma = true
			}
			listValue := v.Operation.Values[ref]
			out += v.renderJSONValueTemplate(listValue, variables, inputValueDefinition)
		}
		out += "]"
	case ast.ValueKindObject:
		out += "{"
		addComma := false
		for _, ref := range v.Operation.ObjectValues[value.Ref].Refs {
			fieldName := v.Operation.Input.ByteSlice(v.Operation.ObjectFields[ref].Name)
			fieldValue := v.Operation.ObjectFields[ref].Value
			typeName := v.Definition.ResolveTypeNameString(v.Definition.InputValueDefinitions[inputValueDefinition].Type)
			typeDefinitionNode, ok := v.Definition.Index.FirstNodeByNameStr(typeName)
			if !ok {
				continue
			}
			objectFieldDefinition, ok := v.Definition.NodeInputFieldDefinitionByName(typeDefinitionNode, fieldName)
			if !ok {
				continue
			}
			if addComma {
				out += ","
			} else {
				addComma = true
			}
			out += fmt.Sprintf("\"%s\":", string(fieldName))
			out += v.renderJSONValueTemplate(fieldValue, variables, objectFieldDefinition)
		}
		out += "}"
	case ast.ValueKindVariable:
		variablePath := v.Operation.VariableValueNameString(value.Ref)
		inputType := v.Definition.InputValueDefinitions[inputValueDefinition].Type
		renderer, err := resolve.NewJSONVariableRendererWithValidationFromTypeRef(v.Definition, v.Definition, inputType)
		if err != nil {
			renderer = resolve.NewJSONVariableRenderer()
		}
		variableName, _ := variables.AddVariable(&resolve.ContextVariable{
			Path:     []string{variablePath},
			Renderer: renderer,
		})
		out += variableName
	}
	return
}

func (v *Visitor) configureSubscription(config objectFetchConfiguration) {
	subscription := config.planner.ConfigureSubscription()
	config.trigger.Variables = subscription.Variables
	config.trigger.Source = subscription.DataSource
	config.trigger.ProcessResponseConfig = subscription.ProcessResponseConfig
	v.resolveInputTemplates(config, &subscription.Input, &config.trigger.Variables)
	config.trigger.Input = []byte(subscription.Input)
}

func (v *Visitor) configureObjectFetch(config objectFetchConfiguration) {
	if config.object == nil {
		return
	}
	fetchConfig := config.planner.ConfigureFetch()
	fetch := v.configureFetch(config, fetchConfig)

	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		v.resolveInputTemplates(config, &f.Input, &f.Variables)
	case *resolve.BatchFetch:
		v.resolveInputTemplates(config, &f.Fetch.Input, &f.Fetch.Variables)
	}
	if config.object.Fetch == nil {
		config.object.Fetch = fetch
		return
	}
	switch existing := config.object.Fetch.(type) {
	case *resolve.SingleFetch:
		copyOfExisting := *existing
		parallel := &resolve.ParallelFetch{
			Fetches: []resolve.Fetch{&copyOfExisting, fetch},
		}
		config.object.Fetch = parallel
	case *resolve.BatchFetch:
		copyOfExisting := *existing
		parallel := &resolve.ParallelFetch{
			Fetches: []resolve.Fetch{&copyOfExisting, fetch},
		}
		config.object.Fetch = parallel
	case *resolve.ParallelFetch:
		existing.Fetches = append(existing.Fetches, fetch)
	}
}

func (v *Visitor) configureFetch(internal objectFetchConfiguration, external FetchConfiguration) resolve.Fetch {
	dataSourceType := reflect.TypeOf(external.DataSource).String()
	dataSourceType = strings.TrimPrefix(dataSourceType, "*")

	singleFetch := &resolve.SingleFetch{
		BufferId:                              internal.bufferID,
		Input:                                 external.Input,
		DataSource:                            external.DataSource,
		Variables:                             external.Variables,
		DisallowSingleFlight:                  external.DisallowSingleFlight,
		DataSourceIdentifier:                  []byte(dataSourceType),
		ProcessResponseConfig:                 external.ProcessResponseConfig,
		DisableDataLoader:                     external.DisableDataLoader,
		SetTemplateOutputToNullOnVariableNull: external.SetTemplateOutputToNullOnVariableNull,
	}

	// if a field depends on an exported variable, data loader needs to be disabled
	// this is because the data loader will render all input templates before all fields are evaluated
	// exporting field values into a variable depends on the field being evaluated first
	// for that reason, if a field depends on an exported variable, data loader needs to be disabled
	disableDataLoader := v.fieldRequiresExportedVariable(internal.fieldRef)
	if disableDataLoader {
		singleFetch.DisableDataLoader = true
	}

	if !external.BatchConfig.AllowBatch {
		return singleFetch
	}

	return &resolve.BatchFetch{
		Fetch:        singleFetch,
		BatchFactory: external.BatchConfig.BatchFactory,
	}
}

type Kind int

const (
	SynchronousResponseKind Kind = iota + 1
	StreamingResponseKind
	SubscriptionResponseKind
)

type Plan interface {
	PlanKind() Kind
	SetFlushInterval(interval int64)
}

type SynchronousResponsePlan struct {
	Response      *resolve.GraphQLResponse
	FlushInterval int64
}

func (s *SynchronousResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *SynchronousResponsePlan) PlanKind() Kind {
	return SynchronousResponseKind
}

type StreamingResponsePlan struct {
	Response      *resolve.GraphQLStreamingResponse
	FlushInterval int64
}

func (s *StreamingResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *StreamingResponsePlan) PlanKind() Kind {
	return StreamingResponseKind
}

type SubscriptionResponsePlan struct {
	Response      *resolve.GraphQLSubscription
	FlushInterval int64
}

func (s *SubscriptionResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}

type DataSourcePlanningBehavior struct {
	// MergeAliasedRootNodes will reuse a data source for multiple root fields with aliases if true.
	// Example:
	//  {
	//    rootField
	//    alias: rootField
	//  }
	// On dynamic data sources (e.g. GraphQL, SQL, ...) this should return true and for
	// static data sources (e.g. REST, static, GRPC...) it should be false.
	MergeAliasedRootNodes bool
	// OverrideFieldPathFromAlias will let the planner know if the response path should also be aliased (= true)
	// or not (= false)
	// Example:
	//  {
	//    rootField
	//    alias: original
	//  }
	// When true expected response will be { "rootField": ..., "alias": ... }
	// When false expected response will be { "rootField": ..., "original": ... }
	OverrideFieldPathFromAlias bool
	// IncludeTypeNameFields should be set to true if the planner wants to get EnterField & LeaveField events
	// for __typename fields
	IncludeTypeNameFields bool
}

type DataSourcePlanner interface {
	Register(visitor *Visitor, configuration DataSourceConfiguration, isNested bool) error
	ConfigureFetch() FetchConfiguration
	ConfigureSubscription() SubscriptionConfiguration
	DataSourcePlanningBehavior() DataSourcePlanningBehavior
	// DownstreamResponseFieldAlias allows the DataSourcePlanner to overwrite the response path with an alias
	// It's required to set OverrideFieldPathFromAlias to true
	// This function is useful in the following scenario
	// 1. The downstream Query doesn't contain an alias
	// 2. The path configuration rewrites the field to an existing field
	// 3. The DataSourcePlanner is using an alias to the upstream
	// Example:
	//
	// type Query {
	//		country: Country
	//		countryAlias: Country
	// }
	//
	// Both, country and countryAlias have a path in the FieldConfiguration of "country"
	// In theory, they would be treated as the same field
	// However, by using DownstreamResponseFieldAlias, it's possible for the DataSourcePlanner to use an alias for countryAlias.
	// In this case, the response would contain both, country and countryAlias fields in the response.
	// At the same time, the downstream Query would only expect the response on the path "country",
	// as both country and countryAlias have a mapping to the path "country".
	// The DataSourcePlanner could keep track that it rewrites the upstream query and use DownstreamResponseFieldAlias
	// to indicate to the Planner to expect the response for countryAlias on the path "countryAlias" instead of "country".
	DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool)
}

type SubscriptionConfiguration struct {
	Input                 string
	Variables             resolve.Variables
	DataSource            resolve.SubscriptionDataSource
	ProcessResponseConfig resolve.ProcessResponseConfig
}

type FetchConfiguration struct {
	Input                string
	Variables            resolve.Variables
	DataSource           resolve.DataSource
	DisallowSingleFlight bool
	// DisableDataLoader will configure the Resolver to not use DataLoader
	// If this is set to false, the planner might still decide to override it,
	// e.g. if a field depends on an exported variable which doesn't work with DataLoader
	DisableDataLoader     bool
	ProcessResponseConfig resolve.ProcessResponseConfig
	BatchConfig           BatchConfig
	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool
}

type BatchConfig struct {
	AllowBatch   bool
	BatchFactory resolve.DataSourceBatchFactory
}

type configurationVisitor struct {
	operationName         string
	operation, definition *ast.Document
	walker                *astvisitor.Walker
	config                Configuration
	planners              []plannerConfiguration
	fetches               []objectFetchConfiguration
	currentBufferId       int
	fieldBuffers          map[int]int

	parentTypeNodes []ast.Node

	ctx context.Context
}

func (c *configurationVisitor) EnterSelectionSet(_ int) {
	c.parentTypeNodes = append(c.parentTypeNodes, c.walker.EnclosingTypeDefinition)
}

func (c *configurationVisitor) LeaveSelectionSet(_ int) {
	c.parentTypeNodes = c.parentTypeNodes[:len(c.parentTypeNodes)-1]
}

type plannerConfiguration struct {
	parentPath              string
	planner                 DataSourcePlanner
	paths                   []pathConfiguration
	dataSourceConfiguration DataSourceConfiguration
	bufferID                int
}

// isNestedPlanner returns true in case the planner is not directly attached to the Operation root
// a nested planner should always build a Query
func (p *plannerConfiguration) isNestedPlanner() bool {
	for i := range p.paths {
		pathElements := strings.Count(p.paths[i].path, ".") + 1
		if pathElements == 2 {
			return false
		}
	}
	return true
}

func (c *configurationVisitor) nextBufferID() int {
	c.currentBufferId++
	return c.currentBufferId
}

func (p *plannerConfiguration) hasPath(path string) bool {
	for i := range p.paths {
		if p.paths[i].path == path {
			return true
		}
	}
	return false
}

func (p *plannerConfiguration) isExitPath(path string) bool {
	for i := range p.paths {
		if p.paths[i].path == path {
			return p.paths[i].exitPlannerOnNode
		}
	}
	return false
}

func (p *plannerConfiguration) shouldWalkFieldsOnPath(path string) bool {
	for i := range p.paths {
		if p.paths[i].path == path {
			return p.paths[i].shouldWalkFields
		}
	}
	return false
}

func (p *plannerConfiguration) setPathExit(path string) {
	for i := range p.paths {
		if p.paths[i].path == path {
			p.paths[i].exitPlannerOnNode = true
			return
		}
	}
}

func (p *plannerConfiguration) hasPathPrefix(prefix string) bool {
	for i := range p.paths {
		if p.paths[i].path == prefix {
			continue
		}
		if strings.HasPrefix(p.paths[i].path, prefix) {
			return true
		}
	}
	return false
}

func (p *plannerConfiguration) hasParent(parent string) bool {
	return p.parentPath == parent
}

func (p *plannerConfiguration) hasChildNode(typeName, fieldName string) bool {
	for i := range p.dataSourceConfiguration.ChildNodes {
		if typeName != p.dataSourceConfiguration.ChildNodes[i].TypeName {
			continue
		}
		for j := range p.dataSourceConfiguration.ChildNodes[i].FieldNames {
			if fieldName == p.dataSourceConfiguration.ChildNodes[i].FieldNames[j] {
				return true
			}
		}
	}
	return false
}

func (p *plannerConfiguration) hasRootNode(typeName, fieldName string) bool {
	for i := range p.dataSourceConfiguration.RootNodes {
		if typeName != p.dataSourceConfiguration.RootNodes[i].TypeName {
			continue
		}
		for j := range p.dataSourceConfiguration.RootNodes[i].FieldNames {
			if fieldName == p.dataSourceConfiguration.RootNodes[i].FieldNames[j] {
				return true
			}
		}
	}
	return false
}

type pathConfiguration struct {
	path              string
	exitPlannerOnNode bool
	// shouldWalkFields indicates whether the planner is allowed to walk into fields
	// this is needed in case we're dealing with a nested federated abstract query
	// we need to be able to walk into the inline fragments and selection sets in the root
	// however, we want to skip the fields at this level
	// so, by setting shouldWalkFields to false, we can walk into non fields only
	shouldWalkFields bool
}

func (c *configurationVisitor) EnterOperationDefinition(ref int) {
	operationName := c.operation.OperationDefinitionNameString(ref)
	if c.operationName != operationName {
		c.walker.SkipNode()
		return
	}
}

func (c *configurationVisitor) EnterField(ref int) {
	fieldName := c.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	parentPath := c.walker.Path.DotDelimitedString()
	currentPath := parentPath + "." + fieldAliasOrName
	root := c.walker.Ancestors[0]
	if root.Kind != ast.NodeKindOperationDefinition {
		return
	}
	isSubscription := c.isSubscription(root.Ref, currentPath)
	for i, plannerConfig := range c.planners {
		planningBehaviour := plannerConfig.planner.DataSourcePlanningBehavior()
		if fieldAliasOrName == "__typename" && planningBehaviour.IncludeTypeNameFields {
			c.planners[i].paths = append(c.planners[i].paths, pathConfiguration{path: currentPath, shouldWalkFields: true})
			return
		}
		if plannerConfig.hasParent(parentPath) && plannerConfig.hasRootNode(typeName, fieldName) && planningBehaviour.MergeAliasedRootNodes {
			// same parent + root node = root sibling
			c.planners[i].paths = append(c.planners[i].paths, pathConfiguration{path: currentPath, shouldWalkFields: true})
			c.fieldBuffers[ref] = plannerConfig.bufferID
			return
		}
		if plannerConfig.hasPath(parentPath) {
			if plannerConfig.hasChildNode(typeName, fieldName) {
				// has parent path + has child node = child
				c.planners[i].paths = append(c.planners[i].paths, pathConfiguration{path: currentPath, shouldWalkFields: true})
				return
			}

			if pathAdded := c.addPlannerPathForUnionChildOfObjectParent(ref, i, currentPath); pathAdded {
				return
			}

			if pathAdded := c.addPlannerPathForChildOfAbstractParent(i, typeName, fieldName, currentPath); pathAdded {
				return
			}
		}
	}
	for i, config := range c.config.DataSources {
		if !config.HasRootNode(typeName, fieldName) {
			continue
		}
		var (
			bufferID int
		)
		if !isSubscription {
			bufferID = c.nextBufferID()
			c.fieldBuffers[ref] = bufferID
		}
		planner := c.config.DataSources[i].Factory.Planner(c.ctx)
		isParentAbstract := c.isParentTypeNodeAbstractType()
		paths := []pathConfiguration{
			{
				path:             currentPath,
				shouldWalkFields: true,
			},
		}
		if isParentAbstract {
			// if the parent is abstract, we add the parent path as well
			// this will ensure that we're walking into and out of the root inline fragments
			// otherwise, we'd only walk into the fields inside the inline fragments in the root,
			// so we'd miss the selection sets and inline fragments in the root
			paths = append([]pathConfiguration{
				{
					path:             parentPath,
					shouldWalkFields: false,
				},
			}, paths...)
		}
		c.planners = append(c.planners, plannerConfiguration{
			bufferID:                bufferID,
			parentPath:              parentPath,
			planner:                 planner,
			paths:                   paths,
			dataSourceConfiguration: config,
		})
		fieldDefinition, ok := c.walker.FieldDefinition(ref)
		if !ok {
			continue
		}
		c.fetches = append(c.fetches, objectFetchConfiguration{
			bufferID:           bufferID,
			planner:            planner,
			isSubscription:     isSubscription,
			fieldRef:           ref,
			fieldDefinitionRef: fieldDefinition,
		})
		return
	}
}

func (c *configurationVisitor) addPlannerPathForUnionChildOfObjectParent(fieldRef int, plannerIndex int, currentPath string) (pathAdded bool) {
	if c.walker.EnclosingTypeDefinition.Kind != ast.NodeKindObjectTypeDefinition {
		return false
	}
	fieldDefRef, exists := c.definition.NodeFieldDefinitionByName(c.walker.EnclosingTypeDefinition, c.operation.FieldNameBytes(fieldRef))
	if !exists {
		return false
	}

	fieldDefTypeRef := c.definition.FieldDefinitionType(fieldDefRef)
	fieldDefTypeName := c.definition.TypeNameBytes(fieldDefTypeRef)
	node, ok := c.definition.NodeByName(fieldDefTypeName)
	if !ok {
		return false
	}

	if node.Kind == ast.NodeKindUnionTypeDefinition {
		c.planners[plannerIndex].paths = append(c.planners[plannerIndex].paths, pathConfiguration{path: currentPath, shouldWalkFields: true})
		return true
	}
	return false
}

func (c *configurationVisitor) addPlannerPathForChildOfAbstractParent(
	plannerIndex int, typeName, fieldName, currentPath string,
) (pathAdded bool) {
	if !c.isParentTypeNodeAbstractType() {
		return false
	}
	// If the field is a root node in any of the data sources, the path shouldn't be handled here
	for _, d := range c.config.DataSources {
		if d.HasRootNode(typeName, fieldName) {
			return false
		}
	}
	// The path for this field should only be added if the parent path also exists on this planner
	c.planners[plannerIndex].paths = append(c.planners[plannerIndex].paths, pathConfiguration{path: currentPath, shouldWalkFields: true})
	return true
}

func (c *configurationVisitor) isParentTypeNodeAbstractType() bool {
	if len(c.parentTypeNodes) < 2 {
		return false
	}
	parentTypeNode := c.parentTypeNodes[len(c.parentTypeNodes)-2]
	return parentTypeNode.Kind.IsAbstractType()
}

func (c *configurationVisitor) LeaveField(ref int) {
	fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	parent := c.walker.Path.DotDelimitedString()
	current := parent + "." + fieldAliasOrName
	for i, planner := range c.planners {
		if planner.hasPath(current) && !planner.hasPathPrefix(current) {
			c.planners[i].setPathExit(current)
			return
		}
	}
}

func (c *configurationVisitor) EnterDocument(operation, definition *ast.Document) {
	c.operation, c.definition = operation, definition
	c.currentBufferId = -1
	c.parentTypeNodes = c.parentTypeNodes[:0]
	if c.planners == nil {
		c.planners = make([]plannerConfiguration, 0, 8)
	} else {
		c.planners = c.planners[:0]
	}
	if c.fetches == nil {
		c.fetches = []objectFetchConfiguration{}
	} else {
		c.fetches = c.fetches[:0]
	}
	if c.fieldBuffers == nil {
		c.fieldBuffers = map[int]int{}
	} else {
		for i := range c.fieldBuffers {
			delete(c.fieldBuffers, i)
		}
	}
}

func (c *configurationVisitor) isSubscription(root int, path string) bool {
	rootOperationType := c.operation.OperationDefinitions[root].OperationType
	if rootOperationType != ast.OperationTypeSubscription {
		return false
	}
	return strings.Count(path, ".") == 1
}

type skipFieldData struct {
	selectionSetRef int
	fieldConfig     *FieldConfiguration
	requiredField   string
	fieldPath       string
}

type requiredFieldsVisitor struct {
	operation, definition *ast.Document
	walker                *astvisitor.Walker
	config                *Configuration
	operationName         string
	skipFieldPaths        []string
	// selectedFieldPaths is a set of all explicitly selected field paths
	selectedFieldPaths map[string]struct{}
	// skipFieldDataPaths is to prevent appending duplicate skipFieldData to potentialSkipFieldDatas
	skipFieldDataPaths map[string]struct{}
	// potentialSkipFieldDatas is used in LeaveDocument to determine whether a required field should be skipped
	// Must be a slice to preserve field order, which is why duplicates are handled with a set
	potentialSkipFieldDatas []*skipFieldData
}

func (r *requiredFieldsVisitor) EnterDocument(_, _ *ast.Document) {
	r.skipFieldPaths = r.skipFieldPaths[:0]
	r.selectedFieldPaths = make(map[string]struct{})
	r.potentialSkipFieldDatas = make([]*skipFieldData, 0)
}

func (r *requiredFieldsVisitor) EnterField(ref int) {
	typeName := r.walker.EnclosingTypeDefinition.NameString(r.definition)
	fieldName := r.operation.FieldNameUnsafeString(ref)
	fieldConfig := r.config.Fields.ForTypeField(typeName, fieldName)
	path := r.walker.Path.DotDelimitedString()
	if fieldConfig == nil {
		// Record all explicitly selected fields
		// A field selected on an interface will have the same field path as a fragment on an object
		// LeaveDocument uses this record to ensure only required fields that were not explicitly selected are skipped
		r.selectedFieldPaths[fmt.Sprintf("%s.%s", path, fieldName)] = struct{}{}
		return
	}
	if len(fieldConfig.RequiresFields) == 0 {
		return
	}
	selectionSet := r.walker.Ancestors[len(r.walker.Ancestors)-1]
	if selectionSet.Kind != ast.NodeKindSelectionSet {
		return
	}
	for _, requiredField := range fieldConfig.RequiresFields {
		requiredFieldPath := fmt.Sprintf("%s.%s", path, requiredField)
		// Prevent adding duplicates to the slice (order is necessary; hence, a separate map)
		if _, ok := r.skipFieldDataPaths[requiredFieldPath]; ok {
			continue
		}
		// For each required field, collect the data required to handle (in LeaveDocument) whether we should skip it
		data := &skipFieldData{
			selectionSetRef: selectionSet.Ref,
			fieldConfig:     fieldConfig,
			requiredField:   requiredField,
			fieldPath:       requiredFieldPath,
		}
		r.potentialSkipFieldDatas = append(r.potentialSkipFieldDatas, data)
	}
}

func (r *requiredFieldsVisitor) handleRequiredField(selectionSet int, requiredFieldName, fullFieldPath string) {
	for _, ref := range r.operation.SelectionSets[selectionSet].SelectionRefs {
		selection := r.operation.Selections[ref]
		if selection.Kind != ast.SelectionKindField {
			continue
		}
		name := r.operation.FieldAliasOrNameString(selection.Ref)
		if name == requiredFieldName {
			// already exists
			return
		}
	}
	r.addRequiredField(requiredFieldName, selectionSet, fullFieldPath)
}

func (r *requiredFieldsVisitor) addRequiredField(fieldName string, selectionSet int, fullFieldPath string) {
	field := ast.Field{
		Name: r.operation.Input.AppendInputString(fieldName),
	}
	addedField := r.operation.AddField(field)
	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  addedField.Ref,
	}
	r.operation.AddSelection(selectionSet, selection)
	r.skipFieldPaths = append(r.skipFieldPaths, fullFieldPath)
}

func (r *requiredFieldsVisitor) EnterOperationDefinition(ref int) {
	operationName := r.operation.OperationDefinitionNameString(ref)
	if r.operationName != operationName {
		r.walker.SkipNode()
		return
	}
}

func (r *requiredFieldsVisitor) LeaveDocument(_, _ *ast.Document) {
	for _, data := range r.potentialSkipFieldDatas {
		path := data.fieldPath
		// If a field was not explicitly selected, skip it
		if _, ok := r.selectedFieldPaths[path]; !ok {
			r.handleRequiredField(data.selectionSetRef, data.requiredField, path)
		}
	}
}
