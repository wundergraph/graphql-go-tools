package graphql

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/jensneuse/abstractlogger"

	"github.com/jensneuse/graphql-go-tools/pkg/execution"
	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type DataSourceHttpJsonOptions struct {
	HttpClient *http.Client
}

type DataSourceGraphqlOptions struct {
	HttpClient *http.Client
}

type ExecutionOptions struct {
	ExtraArguments  json.RawMessage
	HttpJsonOptions DataSourceHttpJsonOptions
	GraphqlOptions  DataSourceGraphqlOptions
}

type ExecutionEngine struct {
	logger      abstractlogger.Logger
	basePlanner *datasource.BasePlanner
	executor    *execution.Executor
}

func NewExecutionEngine(logger abstractlogger.Logger, schema *Schema, plannerConfig datasource.PlannerConfiguration) (*ExecutionEngine, error) {
	executor := execution.NewExecutor(nil)
	basePlanner, err := datasource.NewBaseDataSourcePlanner(schema.rawInput, plannerConfig, logger)
	if err != nil {
		return nil, err
	}

	return &ExecutionEngine{
		logger:      logger,
		basePlanner: basePlanner,
		executor:    executor,
	}, nil
}

func (e *ExecutionEngine) AddHttpJsonDataSource(name string) error {
	return e.AddDataSource(name, &datasource.HttpJsonDataSourcePlannerFactoryFactory{})
}

func (e *ExecutionEngine) AddGraphqlDataSource(name string) error {
	return e.AddDataSource(name, &datasource.GraphQLDataSourcePlannerFactoryFactory{})
}

func (e *ExecutionEngine) AddDataSource(name string, plannerFactoryFactory datasource.PlannerFactoryFactory) error {
	return e.basePlanner.RegisterDataSourcePlannerFactory(name, plannerFactoryFactory)
}

func (e *ExecutionEngine) Execute(ctx context.Context, operation *Request, writer io.Writer) error {
	return e.ExecuteWithOptions(ctx, operation, writer, ExecutionOptions{})
}

func (e *ExecutionEngine) ExecuteWithOptions(ctx context.Context, operation *Request, writer io.Writer, options ExecutionOptions) error {
	var report operationreport.Report
	planner := execution.NewPlanner(e.basePlanner)
	plan := planner.Plan(&operation.document, e.basePlanner.Definition, &report)
	if report.HasErrors() {
		return report
	}

	variables, extraArguments := execution.VariablesFromJson(operation.Variables, options.ExtraArguments)
	executionContext := execution.Context{
		Context:        ctx,
		Variables:      variables,
		ExtraArguments: extraArguments,
	}

	return e.executor.Execute(executionContext, plan, writer)
}
