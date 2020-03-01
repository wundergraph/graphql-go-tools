package execution

import (
	"bytes"
	"github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"io"
)

type DataSource interface {
	Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction
}

type DataSourcePlanner interface {
	CoreDataSourcePlanner
	DataSourcePlannerVisitors
}

type CoreDataSourcePlanner interface {
	// Plan plan returns the pre configured DataSource as well as the Arguments
	// During runtime the arguments get resolved and passed to the DataSource
	Plan(args []Argument) (DataSource, []Argument)
	// Configure is the function to initialize all important values for the DataSourcePlanner to function correctly
	// You probably need access to the walker, operation and definition to use the DataSourcePlanner to its full power
	// walker gives you useful information from within all visitor Callbacks, e.g. the Path & Ancestors
	// operation is the AST of the GraphQL operation
	// definition is the AST of the GraphQL schema definition
	// args are the pre-calculated Arguments from the planner
	// resolverParameters are the parameters from the @directive params field
	Configure(config DataSourcePlannerConfiguration)
}

type DataSourcePlannerVisitors interface {
	astvisitor.EnterInlineFragmentVisitor
	astvisitor.LeaveInlineFragmentVisitor
	astvisitor.EnterSelectionSetVisitor
	astvisitor.LeaveSelectionSetVisitor
	astvisitor.EnterFieldVisitor
	astvisitor.LeaveFieldVisitor
}

type DataSourcePlannerFactory interface {
	DataSourcePlanner() DataSourcePlanner
}

type DataSourcePlannerFactoryFactory interface {
	Initialize(base BaseDataSourcePlanner, configReader io.Reader) (DataSourcePlannerFactory, error)
}

type DataSourcePlannerConfiguration struct {
	walker                *astvisitor.Walker
	operation, definition *ast.Document
}

type BaseDataSourcePlanner struct {
	log                   abstractlogger.Logger
	walker                *astvisitor.Walker   // nolint
	definition, operation *ast.Document        // nolint
	args                  []Argument           // nolint
	rootField             rootField            // nolint
	config                PlannerConfiguration // nolint
}

func NewBaseDataSourcePlanner(schema []byte, config PlannerConfiguration, logger abstractlogger.Logger) (*BaseDataSourcePlanner, error) {

	schema = append(schema, graphqlDefinitionBoilerplate...)

	definition, report := astparser.ParseGraphqlDocumentBytes(schema)
	if report.HasErrors() {
		return nil, report
	}

	return &BaseDataSourcePlanner{
		config:     config,
		log:        logger,
		definition: &definition,
	}, nil
}

func (b *BaseDataSourcePlanner) Configure(config DataSourcePlannerConfiguration) {
	b.operation = config.operation
	b.definition = config.definition
	b.walker = config.walker
}

func (b *BaseDataSourcePlanner) RegisterDataSourcePlannerFactory(dataSourceName string, factory DataSourcePlannerFactoryFactory) (err error) {
	for i := range b.config.TypeFieldConfigurations {
		if dataSourceName != b.config.TypeFieldConfigurations[i].DataSource.Name {
			continue
		}
		configReader := bytes.NewReader(b.config.TypeFieldConfigurations[i].DataSource.Config)
		b.config.TypeFieldConfigurations[i].DataSourcePlannerFactory, err = factory.Initialize(*b, configReader)
		if err != nil {
			return err
		}
	}
	return nil
}

type rootField struct {
	isDefined bool
	ref       int
}

func (r *rootField) setIfNotDefined(ref int) {
	if r.isDefined {
		return
	}
	r.isDefined = true
	r.ref = ref
}

func (r *rootField) isDefinedAndEquals(ref int) bool {
	return r.isDefined && r.ref == ref
}

type visitingDataSourcePlanner struct {
	CoreDataSourcePlanner
}

func (_ visitingDataSourcePlanner) EnterInlineFragment(ref int) {}
func (_ visitingDataSourcePlanner) LeaveInlineFragment(ref int) {}
func (_ visitingDataSourcePlanner) EnterSelectionSet(ref int)   {}
func (_ visitingDataSourcePlanner) LeaveSelectionSet(ref int)   {}
func (_ visitingDataSourcePlanner) EnterField(ref int)          {}
func (_ visitingDataSourcePlanner) LeaveField(ref int)          {}

func SimpleDataSourcePlanner(core CoreDataSourcePlanner) DataSourcePlanner {
	return &visitingDataSourcePlanner{
		CoreDataSourcePlanner: core,
	}
}
