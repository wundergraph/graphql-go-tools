package planv2

import (
	"encoding/json"
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
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
	DefaultFlushInterval int64
	DataSources          []DataSourceConfiguration
	Fields               FieldConfigurations
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
	TypeName                          string
	FieldName                         string
	DisableDefaultMapping             bool
	Path                              []string
	RespectOverrideFieldPathFromAlias bool
	Arguments                         ArgumentsConfigurations
	RequiresFields                    []string
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

const (
	ObjectFieldSource   SourceType = "object_field"
	FieldArgumentSource SourceType = "field_argument"
)

type ArgumentConfiguration struct {
	Name       string
	SourceType SourceType
	SourcePath []string
}

type DataSourceConfiguration struct {
	RootNodes                  []TypeField
	ChildNodes                 []TypeField
	Factory                    PlannerFactory
	OverrideFieldPathFromAlias bool
	Custom                     json.RawMessage
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
	Planner() DataSourcePlanner
}

type TypeField struct {
	TypeName   string
	FieldNames []string
}

type FieldMapping struct {
	TypeName                          string
	FieldName                         string
	DisableDefaultMapping             bool
	Path                              []string
	RespectOverrideFieldPathFromAlias bool
}

func NewPlanner(config Configuration) *Planner {

	// required fields pre-processing

	requiredFieldsWalker := astvisitor.NewWalker(48)
	requiredFieldsV := &requiredFieldsVisitor{
		walker: &requiredFieldsWalker,
	}

	requiredFieldsWalker.RegisterEnterDocumentVisitor(requiredFieldsV)
	requiredFieldsWalker.RegisterEnterOperationVisitor(requiredFieldsV)
	requiredFieldsWalker.RegisterEnterFieldVisitor(requiredFieldsV)

	// configuration

	configurationWalker := astvisitor.NewWalker(48)
	configVisitor := &configurationVisitor{
		walker: &configurationWalker,
	}

	configurationWalker.RegisterEnterDocumentVisitor(configVisitor)
	configurationWalker.RegisterFieldVisitor(configVisitor)
	configurationWalker.RegisterEnterOperationVisitor(configVisitor)

	// planning

	planningWalker := astvisitor.NewWalker(48)
	planningVisitor := &Visitor{
		Walker: &planningWalker,
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

func (p *Planner) Plan(operation, definition *ast.Document, operationName string, report *operationreport.Report) (plan Plan) {

	// make a copy of the config as the pre-processor modifies it

	config := p.config

	// pre-process required fields

	p.preProcessRequiredFields(&config, operation, definition, operationName, report)

	// find planning paths

	p.configurationVisitor.operationName = operationName
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

	for key := range p.planningVisitor.planners {
		err := p.planningVisitor.planners[key].planner.Register(p.planningVisitor, p.planningVisitor.planners[key].dataSourceConfiguration.Custom)
		if err != nil {
			p.planningWalker.StopWithInternalErr(err)
		}
	}

	// process the plan

	p.planningVisitor.OperationName = operationName
	p.planningWalker.Walk(operation, definition, report)

	return p.planningVisitor.plan
}

func (p *Planner) preProcessRequiredFields(config *Configuration, operation, definition *ast.Document, operationName string, report *operationreport.Report) {
	if !p.hasRequiredFields(config) {
		return
	}
	p.requiredFieldsVisitor.config = config
	p.requiredFieldsVisitor.operation = operation
	p.requiredFieldsVisitor.definition = definition
	p.requiredFieldsVisitor.operationName = operationName
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
	Operation, Definition *ast.Document
	Walker                *astvisitor.Walker
	Importer              astimport.Importer
	Config                Configuration
	plan                  Plan
	OperationName         string
	objects               []*resolve.Object
	currentFields         []objectFields
	planners              []plannerConfiguration
	fetchConfigurations   map[int]objectFetchConfiguration
	fieldBuffers          map[int]int
	skipFieldPaths        []string
}

type objectFields struct {
	popOnField int
	fields     *[]resolve.Field
}

type objectFetchConfiguration struct {
	object   *resolve.Object
	planner  DataSourcePlanner
	bufferID int
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

func (v *Visitor) currentPlannerConfiguration() plannerConfiguration {
	path := v.currentFullPath()
	for i := range v.planners {
		if v.planners[i].hasPath(path) {
			return v.planners[i]
		}
	}
	return plannerConfiguration{}
}

func (v *Visitor) EnterDirective(ref int) {
	directiveName := v.Operation.DirectiveNameString(ref)
	switch v.Walker.Ancestors[len(v.Walker.Ancestors)-1].Kind {
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
			/*initialBatchSize := 0
			if value, ok := v.Operation.DirectiveArgumentValueByName(ref, literal.INITIAL_BATCH_SIZE); ok {
				if value.Kind == ast.ValueKindInteger {
					initialBatchSize = int(v.Operation.IntValueAsInt(value.Ref))
				}
			}*/
		case "defer":

		}
	}
}

func (v *Visitor) LeaveSelectionSet(ref int) {

}

func (v *Visitor) EnterSelectionSet(ref int) {

}

func (v *Visitor) EnterField(ref int) {

	if v.skipField(ref) {
		return
	}

	fieldName := v.Operation.FieldAliasOrNameBytes(ref)
	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}

	if fetchConfig, exists := v.fetchConfigurations[ref]; exists {
		fetchConfig.object = v.objects[len(v.objects)-1]
		v.fetchConfigurations[ref] = fetchConfig
	}

	path := v.resolveFieldPath(ref)
	fieldDefinitionType := v.Definition.FieldDefinitionType(fieldDefinition)
	bufferID, hasBuffer := v.fieldBuffers[ref]
	field := resolve.Field{
		Name:      fieldName,
		Value:     v.resolveFieldValue(ref, fieldDefinitionType, true, path),
		HasBuffer: hasBuffer,
		BufferID:  bufferID,
	}

	*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, field)
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
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition:
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
		typeDefinitionNode, ok := v.Definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			return &resolve.Null{}
		}
		switch typeDefinitionNode.Kind {
		case ast.NodeKindScalarTypeDefinition:
			switch typeName {
			case "String":
				return &resolve.String{
					Path:     path,
					Nullable: nullable,
				}
			case "Boolean":
				return &resolve.Boolean{
					Path:     path,
					Nullable: nullable,
				}
			case "Int":
				return &resolve.Integer{
					Path:     path,
					Nullable: nullable,
				}
			case "Float":
				return &resolve.Float{
					Path:     path,
					Nullable: nullable,
				}
			default:
				return &resolve.String{
					Path:     path,
					Nullable: nullable,
				}
			}
		case ast.NodeKindEnumTypeDefinition:
			return &resolve.String{
				Path:     path,
				Nullable: nullable,
			}
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition:
			object := &resolve.Object{
				Nullable: nullable,
				Path:     path,
				Fields:   []resolve.Field{},
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

func (v *Visitor) EnterOperationDefinition(ref int) {
	operationName := v.Operation.OperationDefinitionNameString(ref)
	if v.OperationName != operationName {
		v.Walker.SkipNode()
		return
	}

	rootObject := &resolve.Object{
		Fields: []resolve.Field{},
	}

	v.objects = append(v.objects, rootObject)
	v.currentFields = append(v.currentFields, objectFields{
		fields:     &rootObject.Fields,
		popOnField: -1,
	})

	isSubscription, isStreaming, err := AnalyzePlanKind(v.Operation, v.Definition, v.OperationName)
	if err != nil {
		v.Walker.StopWithInternalErr(err)
		return
	}

	graphQLResponse := &resolve.GraphQLResponse{
		Data: rootObject,
	}

	if isSubscription {
		v.plan = &SubscriptionResponsePlan{
			Response: resolve.GraphQLSubscription{
				Response: graphQLResponse,
			},
		}
		return
	}

	if isStreaming {
		v.plan = &StreamingResponsePlan{
			Response: resolve.GraphQLStreamingResponse{
				InitialResponse: graphQLResponse,
			},
		}
		return
	}

	v.plan = &SynchronousResponsePlan{
		Response: graphQLResponse,
	}
}

func (v *Visitor) resolveFieldPath(ref int) []string {
	typeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldName := v.Operation.FieldNameString(ref)

	config := v.currentPlannerConfiguration()
	aliasOverride := config.dataSourceConfiguration.OverrideFieldPathFromAlias

	for i := range v.Config.Fields {
		if v.Config.Fields[i].TypeName == typeName && v.Config.Fields[i].FieldName == fieldName {
			if aliasOverride && v.Config.Fields[i].RespectOverrideFieldPathFromAlias {
				return []string{v.Operation.FieldAliasOrNameString(ref)}
			}
			if v.Config.Fields[i].DisableDefaultMapping {
				return nil
			}
			if v.Config.Fields[i].Path != nil {
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
}

func (v *Visitor) LeaveDocument(operation, definition *ast.Document) {
	for i := range v.fetchConfigurations {
		v.configureObjectFetch(v.fetchConfigurations[i])
	}
}

func (v *Visitor) configureObjectFetch(config objectFetchConfiguration) {
	if config.object == nil {
		return
	}
	fetchConfig := config.planner.ConfigureFetch()
	fetch := v.configureSingleFetch(config, fetchConfig)
	if config.object.Fetch == nil {
		config.object.Fetch = fetch
		return
	}
	switch existing := config.object.Fetch.(type) {
	case *resolve.SingleFetch:
		copyOfExisting := *existing
		parallel := &resolve.ParallelFetch{
			Fetches: []*resolve.SingleFetch{&copyOfExisting, fetch},
		}
		config.object.Fetch = parallel
	case *resolve.ParallelFetch:
		existing.Fetches = append(existing.Fetches, fetch)
	}
}

func (v *Visitor) configureSingleFetch(internal objectFetchConfiguration, external FetchConfiguration) *resolve.SingleFetch {
	return &resolve.SingleFetch{
		BufferId:             internal.bufferID,
		Input:                external.Input,
		DataSource:           external.DataSource,
		Variables:            external.Variables,
		DisallowSingleFlight: external.DisallowSingleFlight,
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
	Response      resolve.GraphQLStreamingResponse
	FlushInterval int64
}

func (s *StreamingResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *StreamingResponsePlan) PlanKind() Kind {
	return StreamingResponseKind
}

type SubscriptionResponsePlan struct {
	Response      resolve.GraphQLSubscription
	FlushInterval int64
}

func (s *SubscriptionResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}

type DataSourcePlanner interface {
	Register(visitor *Visitor, customConfiguration json.RawMessage) error
	ConfigureFetch() FetchConfiguration
}

type FetchConfiguration struct {
	Input                string
	Variables            resolve.Variables
	DataSource           resolve.DataSource
	DisallowSingleFlight bool
}

type configurationVisitor struct {
	operationName         string
	operation, definition *ast.Document
	walker                *astvisitor.Walker
	config                Configuration
	planners              []plannerConfiguration
	fetches               map[int]objectFetchConfiguration
	currentBufferId       int
	fieldBuffers          map[int]int
}

type plannerConfiguration struct {
	parentPath              string
	planner                 DataSourcePlanner
	paths                   []pathConfiguration
	dataSourceConfiguration DataSourceConfiguration
	bufferID                int
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

func (p *plannerConfiguration) setPathExit(path string) {
	for i := range p.paths {
		if p.paths[i].path == path {
			p.paths[i].exitPlannerOnNode = true
			return
		}
	}
	return
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
}

func (c *configurationVisitor) EnterOperationDefinition(ref int) {
	operationName := c.operation.OperationDefinitionNameString(ref)
	if c.operationName != operationName {
		c.walker.SkipNode()
		return
	}
}

func (c *configurationVisitor) EnterField(ref int) {
	fieldName := c.operation.FieldNameString(ref)
	fieldAliasOrName := c.operation.FieldAliasOrNameString(ref)
	typeName := c.walker.EnclosingTypeDefinition.NameString(c.definition)
	parent := c.walker.Path.DotDelimitedString()
	current := parent + "." + fieldAliasOrName
	for i, planner := range c.planners {
		if planner.hasParent(parent) && planner.hasRootNode(typeName, fieldName) {
			// same parent + root node = root sibling
			c.planners[i].paths = append(c.planners[i].paths, pathConfiguration{path: current})
			c.fieldBuffers[ref] = planner.bufferID
			return
		}
		if planner.hasPath(parent) && planner.hasChildNode(typeName, fieldName) {
			// has parent path + has child node = child
			c.planners[i].paths = append(c.planners[i].paths, pathConfiguration{path: current})
			return
		}
	}
	for i, config := range c.config.DataSources {
		if config.HasRootNode(typeName, fieldName) {
			bufferID := c.nextBufferID()
			c.fieldBuffers[ref] = bufferID
			planner := c.config.DataSources[i].Factory.Planner()
			c.planners = append(c.planners, plannerConfiguration{
				bufferID:   bufferID,
				parentPath: parent,
				planner:    planner,
				paths: []pathConfiguration{
					{
						path: current,
					},
				},
				dataSourceConfiguration: config,
			})
			c.fetches[ref] = objectFetchConfiguration{
				bufferID: bufferID,
				planner:  planner,
			}
			return
		}
	}
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
	if c.planners == nil {
		c.planners = make([]plannerConfiguration, 0, 8)
	} else {
		c.planners = c.planners[:0]
	}
	if c.fetches == nil {
		c.fetches = map[int]objectFetchConfiguration{}
	} else {
		for i := range c.fetches {
			delete(c.fetches, i)
		}
	}
	if c.fieldBuffers == nil {
		c.fieldBuffers = map[int]int{}
	} else {
		for i := range c.fieldBuffers {
			delete(c.fieldBuffers, i)
		}
	}
}

type requiredFieldsVisitor struct {
	operation, definition *ast.Document
	walker                *astvisitor.Walker
	config                *Configuration
	operationName         string
	skipFieldPaths        []string
}

func (r *requiredFieldsVisitor) EnterDocument(operation, definition *ast.Document) {
	r.skipFieldPaths = r.skipFieldPaths[:0]
}

func (r *requiredFieldsVisitor) EnterField(ref int) {
	typeName := r.walker.EnclosingTypeDefinition.NameString(r.definition)
	fieldName := r.operation.FieldNameString(ref)
	fieldConfig := r.config.Fields.ForTypeField(typeName, fieldName)
	if fieldConfig == nil {
		return
	}
	if len(fieldConfig.RequiresFields) == 0 {
		return
	}
	selectionSet := r.walker.Ancestors[len(r.walker.Ancestors)-1]
	if selectionSet.Kind != ast.NodeKindSelectionSet {
		return
	}
	for i := range fieldConfig.RequiresFields {
		r.handleRequiredField(selectionSet.Ref, fieldConfig.RequiresFields[i])
	}
}

func (r *requiredFieldsVisitor) handleRequiredField(selectionSet int, requiredFieldName string) {
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
	r.addRequiredField(requiredFieldName, selectionSet)
}

func (r *requiredFieldsVisitor) addRequiredField(fieldName string, selectionSet int) {
	field := ast.Field{
		Name: r.operation.Input.AppendInputString(fieldName),
	}
	addedField := r.operation.AddField(field)
	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  addedField.Ref,
	}
	r.operation.AddSelection(selectionSet, selection)
	addedFieldPath := r.walker.Path.DotDelimitedString() + "." + fieldName
	r.skipFieldPaths = append(r.skipFieldPaths, addedFieldPath)
}

func (r *requiredFieldsVisitor) EnterOperationDefinition(ref int) {
	operationName := r.operation.OperationDefinitionNameString(ref)
	if r.operationName != operationName {
		r.walker.SkipNode()
		return
	}
}
