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
	configurationWalker  *astvisitor.Walker
	configurationVisitor *configurationVisitor
	planningWalker       *astvisitor.Walker
	planningVisitor      *Visitor
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

	// configuration

	configurationWalker := astvisitor.NewWalker(48)
	configVisitor := &configurationVisitor{
		walker: &configurationWalker,
		config: config,
	}

	configurationWalker.RegisterEnterDocumentVisitor(configVisitor)
	configurationWalker.RegisterFieldVisitor(configVisitor)
	configurationWalker.RegisterEnterOperationVisitor(configVisitor)

	// planning

	planningWalker := astvisitor.NewWalker(48)
	planningVisitor := &Visitor{
		Walker: &planningWalker,
		Config: config,
	}

	p := &Planner{
		configurationWalker:  &configurationWalker,
		configurationVisitor: configVisitor,
		planningWalker:       &planningWalker,
		planningVisitor:      planningVisitor,
	}

	return p
}

func (p *Planner) Plan(operation, definition *ast.Document, operationName string, report *operationreport.Report) (plan Plan) {

	p.configurationVisitor.operationName = operationName
	p.configurationWalker.Walk(operation, definition, report)

	p.planningVisitor.planners = p.configurationVisitor.planners
	p.planningVisitor.fetchConfigurations = p.configurationVisitor.fetches
	p.planningVisitor.fieldBuffers = p.configurationVisitor.fieldBuffers

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

	p.planningVisitor.OperationName = operationName
	p.planningWalker.Walk(operation, definition, report)

	return p.planningVisitor.plan
}

type Visitor struct {
	Operation, Definition *ast.Document
	Walker                *astvisitor.Walker
	Importer              astimport.Importer
	Config                Configuration
	plan                  Plan
	OperationName         string
	objects               []*resolve.Object
	fields                []*[]resolve.Field
	planners              []plannerConfiguration
	fetchConfigurations   map[int]objectFetchConfiguration
	fieldBuffers          map[int]int
}

type objectFetchConfiguration struct {
	planners []plannerWithBufferID
	object   *resolve.Object
}

type plannerWithBufferID struct {
	planner  DataSourcePlanner
	bufferID int
}

func (v *Visitor) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor interface{}) bool {
	if visitor == v {
		return true
	}
	path := v.Walker.Path.DotDelimitedString()
	if kind == astvisitor.EnterField {
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
			initialBatchSize := 0
			if value, ok := v.Operation.DirectiveArgumentValueByName(ref, literal.INITIAL_BATCH_SIZE); ok {
				if value.Kind == ast.ValueKindInteger {
					initialBatchSize = int(v.Operation.IntValueAsInt(value.Ref))
				}
			}
			(*v.fields[len(v.fields)-1])[len(*v.fields[len(v.fields)-1])-1].Stream = &resolve.StreamField{
				InitialBatchSize: initialBatchSize,
			}
		case "defer":
			(*v.fields[len(v.fields)-1])[len(*v.fields[len(v.fields)-1])-1].Defer = &resolve.DeferField{}
		}
	}
}

func (v *Visitor) LeaveSelectionSet(ref int) {
	v.fields = v.fields[:len(v.fields)-1]
}

func (v *Visitor) EnterSelectionSet(ref int) {
	currentObject := v.objects[len(v.objects)-1]
	fieldSet := resolve.FieldSet{
		Fields: []resolve.Field{},
	}
	currentObject.FieldSets = append(currentObject.FieldSets, fieldSet)
	v.fields = append(v.fields, &currentObject.FieldSets[len(currentObject.FieldSets)-1].Fields)
}

func (v *Visitor) EnterField(ref int) {
	fieldName := v.Operation.FieldAliasOrNameBytes(ref)
	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}

	if fetchConfig, exists := v.fetchConfigurations[ref]; exists {
		v.fetchConfigurations[ref] = objectFetchConfiguration{
			planners: fetchConfig.planners,
			object:   v.objects[len(v.objects)-1],
		}
	}

	if bufferID, ok := v.fieldBuffers[ref]; ok {
		v.objects[len(v.objects)-1].FieldSets[len(v.objects[len(v.objects)-1].FieldSets)-1].HasBuffer = true
		v.objects[len(v.objects)-1].FieldSets[len(v.objects[len(v.objects)-1].FieldSets)-1].BufferID = bufferID
	}

	path := v.resolveFieldPath(ref)
	fieldDefinitionType := v.Definition.FieldDefinitionType(fieldDefinition)
	field := resolve.Field{
		Name:  fieldName,
		Value: v.resolveFieldValue(ref, fieldDefinitionType, true, path),
	}
	*v.fields[len(v.fields)-1] = append(*v.fields[len(v.fields)-1], field)
}

func (v *Visitor) LeaveField(ref int) {
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
			}
			v.objects = append(v.objects, object)
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

	rootObject := &resolve.Object{}

	v.objects = append(v.objects, rootObject)

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
	if len(config.planners) == 0 {
		return
	}
	if len(config.planners) == 1 {
		fetchConfig := config.planners[0].planner.ConfigureFetch()
		config.object.Fetch = v.configureSingleFetch(config.planners[0].bufferID, config, fetchConfig)
		return
	}
	fetches := make([]*resolve.SingleFetch, len(config.planners))
	for i := range fetches {
		fetchConfig := config.planners[i].planner.ConfigureFetch()
		bufferID := config.planners[i].bufferID
		fetches[i] = v.configureSingleFetch(bufferID, config, fetchConfig)
	}
	config.object.Fetch = &resolve.ParallelFetch{
		Fetches: fetches,
	}
}

func (v *Visitor) configureSingleFetch(bufferID int, internal objectFetchConfiguration, external FetchConfiguration) *resolve.SingleFetch {
	return &resolve.SingleFetch{
		BufferId:   bufferID,
		Input:      external.Input,
		DataSource: external.DataSource,
		Variables:  external.Variables,
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
	Input      string
	Variables  resolve.Variables
	DataSource resolve.DataSource
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
			plannerWithBuffer := plannerWithBufferID{
				planner:  planner,
				bufferID: bufferID,
			}
			if existing, ok := c.fetches[ref]; ok {
				c.fetches[ref] = objectFetchConfiguration{
					planners: append(existing.planners, plannerWithBuffer),
				}
			} else {
				c.fetches[ref] = objectFetchConfiguration{
					planners: []plannerWithBufferID{plannerWithBuffer},
				}
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
