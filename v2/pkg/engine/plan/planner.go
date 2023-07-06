package plan

import (
	"context"
	"strings"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
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

		if plannerWithId, ok := p.planningVisitor.planners[key].planner.(astvisitor.VisitorIdentifier); ok {
			plannerWithId.SetID(key + 1)
		}
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
